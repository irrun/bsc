package ethapi

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	jsoniter "github.com/json-iterator/go"
)

const (
	InvalidBidParamError = -38001
	MevNotRunningError   = -38002
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
	GasFee      *big.Int        `json:"gasFee"`
	Timestamp   int64           `json:"timestamp"`
	BuilderFee  *big.Int        `json:"builderFee"`
}

// MevAPI implements the interfaces that defined in the BEP-322.
// It offers methods for the interaction between builders and validators.
type MevAPI struct {
	b Backend
}

// NewMevAPI creates a new MevAPI.
func NewMevAPI(b Backend) *MevAPI {
	return &MevAPI{b}
}

// SendBid receives bid from the builders.
// If mev is not running or bid is invalid, return error.
// Otherwise, creates a builder bid for the given argument, submit it to the miner.
func (m *MevAPI) SendBid(ctx context.Context, args BidArgs) (common.Hash, error) {
	if !m.b.MevRunning() {
		return common.Hash{}, newBidError(errors.New("mev is not running"), MevNotRunningError)
	}

	var (
		bid           = args.Bid
		currentHeader = m.b.CurrentHeader()
		txs           types.Transactions
		builderFee    = big.NewInt(0)
	)

	// only support bidding for the next block not for the future block
	if bid.BlockNumber != currentHeader.Number.Uint64()+1 {
		return common.Hash{}, newBidError(fmt.Errorf("block number is stale"), InvalidBidParamError)
	}

	if bid.ParentHash != currentHeader.Hash() {
		return common.Hash{}, newBidError(fmt.Errorf("non-aligned parent hash:%v", currentHeader.Hash()), InvalidBidParamError)
	}

	if bid.BuilderFee != nil {
		builderFee = bid.BuilderFee
		if builderFee.Cmp(bid.GasFee) >= 0 {
			return common.Hash{}, newBidError(fmt.Errorf("builder fee must be less than gas fee"), InvalidBidParamError)
		}
	}

	builder, err := parseSignature(args)
	if err != nil {
		return common.Hash{}, newBidError(err, InvalidBidParamError)
	}

	// TODO(renee) recover signature
	// TODO(renee) parallel here
	for _, encodedTx := range bid.Txs {
		tx := new(types.Transaction)
		if err := tx.UnmarshalBinary(encodedTx); err != nil {
			return common.Hash{}, newBidError(fmt.Errorf("invalid tx, %v", err), InvalidBidParamError)
		}
		txs = append(txs, tx)
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

	err = m.b.SendBid(ctx, innerBid)

	if err != nil {
		return common.Hash{}, newBidError(err, InvalidBidParamError)
	}

	return innerBid.Hash(), nil
}

// Running returns true if mev is running
func (m *MevAPI) Running() bool {
	return m.b.MevRunning()
}

func newBidError(err error, code int) *bidError {
	return &bidError{
		error: err,
		code:  code,
	}
}

// bidError is an API error that encompasses an invalid bid with JSON error
// code and a binary data blob.
type bidError struct {
	error
	code int
}

// ErrorCode returns the JSON error code for an invalid bid.
// See: https://github.com/ethereum/wiki/wiki/JSON-RPC-Error-Codes-Improvement-Proposal
func (e *bidError) ErrorCode() int {
	return e.code
}

func parseSignature(args BidArgs) (common.Address, error) {
	bid, err := jsoniter.Marshal(args.Bid)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to marshal bid, %v", err.Error())
	}

	sigature, err := hexutil.Decode(args.Signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to decode signature, %v", err.Error())
	}

	sigPublicKey, err := crypto.Ecrecover(crypto.Keccak256(bid), sigature)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to recover signature, %v ", err.Error())
	}

	pk, err := crypto.UnmarshalPubkey(sigPublicKey)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to unmarshal pubkey, %v", err.Error())
	}

	return crypto.PubkeyToAddress(*pk), nil
}
