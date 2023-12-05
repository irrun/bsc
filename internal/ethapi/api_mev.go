package ethapi

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
)

const (
	InvalidBidParamError = -38001
)

// BidArgs represents the arguments to submit a bid.
type BidArgs struct {
	// bid
	Bid *Bid
	// signed signature of the bid
	Signature string `json:"signature"`
}

// Bid represents a bid.
type Bid struct {
	BlockNumber uint64          `json:"blockNumber"`
	ParentHash  common.Hash     `json:"parentHash"`
	Txs         []hexutil.Bytes `json:"txs,omitempty"`
	GasUsed     uint64          `json:"gasUsed"`
	GasFee      uint64          `json:"gasFee"`
	Timestamp   int64           `json:"timestamp"`
	BuilderFee  *big.Int        `json:"builderFee"`
}

// MevAPI provides an API for PBS.
// It offers methods for the interaction between builders and validators.
type MevAPI struct {
	b Backend
}

// NewMevAPI creates a new Builder API.
func NewMevAPI(b Backend) *MevAPI {
	return &MevAPI{b}
}

// BidBlock sends a bid to the validator.
func (m *MevAPI) BidBlock(ctx context.Context, args BidArgs) error {
	if !m.b.MevRunning() {
		return errors.New("mev is not running")
	}

	var (
		bid           = args.Bid
		currentHeader = m.b.CurrentHeader()
		txs           types.Transactions
		builderFee    = big.NewInt(0)
	)

	if len(bid.Txs) == 0 {
		return newBidError(errors.New("block missing txs"))
	}

	if bid.BlockNumber <= currentHeader.Number.Uint64() {
		return newBidError(fmt.Errorf("block number is stale"))
	}

	if bid.ParentHash != currentHeader.Hash() {
		return newBidError(fmt.Errorf("non-aligned parent hash:%v", currentHeader.Hash()))
	}

	if bid.BuilderFee.Cmp(big.NewInt(0).SetUint64(bid.GasFee)) >= 0 {
		return newBidError(fmt.Errorf("builder fee must be less than gas fee"))
	}

	for _, encodedTx := range args.Bid.Txs {
		tx := new(types.Transaction)
		if err := tx.UnmarshalBinary(encodedTx); err != nil {
			return newBidError(fmt.Errorf("invalid tx, %v", err))
		}
		txs = append(txs, tx)
	}

	builder, err := parseSignature(args)
	if err != nil {
		return newBidError(err)
	}

	if args.Bid.BuilderFee != nil {
		builderFee = args.Bid.BuilderFee
	}

	innerBid := &types.Bid{
		Builder:     builder,
		BlockNumber: args.Bid.BlockNumber,
		ParentHash:  args.Bid.ParentHash,
		Txs:         txs,
		GasUsed:     args.Bid.GasUsed,
		GasFee:      args.Bid.GasFee,
		Timestamp:   args.Bid.Timestamp,
		BuilderFee:  builderFee,
	}

	err = m.b.BidBlock(ctx, innerBid)

	if err != nil {
		return newBidError(err)
	}

	return nil
}

// MevRunning returns true if the mev is enabled.
func (m *MevAPI) MevRunning() bool {
	return m.b.MevRunning()
}

func newBidError(err error) *bidError {
	return &bidError{
		error: err,
	}
}

// bidError is an API error that encompasses an invalid bid with JSON error
// code and a binary data blob.
type bidError struct {
	error
}

// ErrorCode returns the JSON error code for an invalid bid.
// See: https://github.com/ethereum/wiki/wiki/JSON-RPC-Error-Codes-Improvement-Proposal
func (e *bidError) ErrorCode() int {
	return InvalidBidParamError
}

func parseSignature(args BidArgs) (common.Address, error) {
	hash, err := rlp.EncodeToBytes(args.Bid)
	if err != nil {
		return common.Address{}, errors.New("fail to parse signature, err: " + err.Error())
	}

	sig := hexutil.MustDecode(args.Signature)
	sigPublicKey, err := crypto.Ecrecover(hash, sig)
	if err != nil {
		return common.Address{}, errors.New("fail to parse signature, err: " + err.Error())
	}

	pk, err := crypto.UnmarshalPubkey(sigPublicKey)
	if err != nil {
		return common.Address{}, errors.New("fail to parse signature, err: " + err.Error())
	}

	return crypto.PubkeyToAddress(*pk), nil
}
