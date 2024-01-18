package ethapi

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	InvalidBidParamError = -38001
	MevNotRunningError   = -38002

	TransferTxGasLimit = 25000
)

// BidArgs represents the arguments to submit a bid.
type BidArgs struct {
	// bid
	Bid *Bid
	// signed signature of the bid
	Signature string `json:"signature"`
	// TransferTx is a flag to indicate whether to return txs in the bid
	TransferTx        hexutil.Bytes `json:"transferTx,omitempty"`
	TransferTxGasUsed uint64        `json:"transferTxGasUsed"`
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
		bidTxs        = make([]*types.Transaction, len(bid.Txs))
		builderFee    = big.NewInt(0)
		transferTx    = new(types.Transaction)
	)

	// only support bidding for the next block not for the future block
	if bid.BlockNumber != currentHeader.Number.Uint64()+1 {
		return common.Hash{}, newInvalidBidError("stale block number")
	}

	if bid.ParentHash.Cmp(currentHeader.Hash()) != 0 {
		return common.Hash{},
			newInvalidBidError("non-aligned parent hash:" + currentHeader.Hash().Hex())
	}

	if bid.BuilderFee != nil {
		builderFee = bid.BuilderFee
		if builderFee.Cmp(bid.GasFee) >= 0 {
			return common.Hash{}, newInvalidBidError("builder fee must be less than gas fee")
		}

		if builderFee.Cmp(big.NewInt(0)) > 0 {
			if args.TransferTxGasUsed >= TransferTxGasLimit {
				return common.Hash{}, newInvalidBidError("transfer tx gas used must be less than " +
					strconv.Itoa(TransferTxGasLimit))
			}

			if args.TransferTx != nil && len(args.TransferTx) != 0 {
				if err := transferTx.UnmarshalBinary(args.TransferTx); err != nil {
					return common.Hash{}, newInvalidBidError(fmt.Sprintf("unmarshal transfer tx err:%v", err))
				}
			}
		}
	}

	builder, err := parseSignature(args)
	if err != nil {
		return common.Hash{}, newBidError(err, InvalidBidParamError)
	}

	var wg sync.WaitGroup
	for i, encodedTx := range bid.Txs {
		wg.Add(1)

		go func(i int, encodedTx hexutil.Bytes) {
			defer wg.Done()
			tx := new(types.Transaction)
			if err := tx.UnmarshalBinary(encodedTx); err == nil {
				bidTxs[i] = tx
			}
		}(i, encodedTx)
	}

	wg.Wait()

	for i, _ := range bidTxs {
		if bidTxs[i] == nil {
			return common.Hash{}, newInvalidBidError("invalid tx in bid")
		}
	}

	txs := make([]*types.Transaction, 0)
	txs = append(txs, bidTxs...)
	gasFee := big.NewInt(0).Set(bid.GasFee)
	if len(transferTx.Data()) != 0 {
		txs = append(txs, transferTx)
		gasFee.Add(gasFee, big.NewInt(0).Mul(big.NewInt(int64(args.TransferTxGasUsed)), transferTx.GasPrice()))
	}

	innerBid := &types.Bid{
		Builder:     builder,
		BlockNumber: bid.BlockNumber,
		ParentHash:  bid.ParentHash,
		Txs:         txs,
		GasUsed:     bid.GasUsed + args.TransferTxGasUsed,
		GasFee:      gasFee,
		Timestamp:   bid.Timestamp,
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

func newInvalidBidError(message string) *bidError {
	return newBidError(errors.New(message), InvalidBidParamError)
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
	bid, err := rlp.EncodeToBytes(args.Bid)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to marshal bid, %v", err.Error())
	}

	signature, err := hexutil.Decode(args.Signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to decode signature, %v", err.Error())
	}

	sigPublicKey, err := crypto.Ecrecover(crypto.Keccak256(bid), signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to recover signature, %v ", err.Error())
	}

	pk, err := crypto.UnmarshalPubkey(sigPublicKey)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to unmarshal pubkey, %v", err.Error())
	}

	return crypto.PubkeyToAddress(*pk), nil
}
