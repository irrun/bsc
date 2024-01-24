package ethapi

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"sync"

	"github.com/panjf2000/ants/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	InvalidBidParamError = -38001
	MevNotRunningError   = -38002
	MevBusyError         = -38003
)

const (
	MevRoutineLimit    = 10000
	TransferTxGasLimit = 25000
)

// MevAPI implements the interfaces that defined in the BEP-322.
// It offers methods for the interaction between builders and validators.
type MevAPI struct {
	b Backend

	antsPool *ants.Pool
}

// NewMevAPI creates a new MevAPI.
func NewMevAPI(b Backend) *MevAPI {
	antsPool, _ := ants.NewPool(MevRoutineLimit)

	return &MevAPI{b, antsPool}
}

// SendBid receives bid from the builders.
// If mev is not running or bid is invalid, return error.
// Otherwise, creates a builder bid for the given argument, submit it to the miner.
func (m *MevAPI) SendBid(ctx context.Context, args types.BidArgs) (common.Hash, error) {
	if !m.b.MevRunning() {
		return common.Hash{}, newBidError(errors.New("mev is not running"), MevNotRunningError)
	}

	var (
		bid           = args.Bid
		currentHeader = m.b.CurrentHeader()
		bidTxs        = make([]*types.Transaction, len(bid.Txs))
		builderFee    = big.NewInt(0)
		payBidTx      = new(types.Transaction)
	)

	// only support bidding for the next block not for the future block
	if bid.BlockNumber != currentHeader.Number.Uint64()+1 {
		return common.Hash{}, newInvalidBidError("stale block number")
	}

	if bid.ParentHash == currentHeader.Hash() {
		return common.Hash{}, newInvalidBidError(fmt.Sprintf("non-aligned parent hash: %v", currentHeader.Hash()))
	}

	if bid.BuilderFee != nil {
		builderFee = bid.BuilderFee
		if builderFee.Cmp(big.NewInt(0)) < 0 {
			return common.Hash{}, newInvalidBidError("builder fee must be greater than 0")
		}

		if builderFee.Cmp(bid.GasFee) >= 0 {
			return common.Hash{}, newInvalidBidError("builder fee must be less than gas fee")
		}

		if builderFee.Cmp(big.NewInt(0)) > 0 {
			if args.PayBidTxGasUsed >= TransferTxGasLimit {
				return common.Hash{}, newInvalidBidError("transfer tx gas used must be less than " +
					strconv.Itoa(TransferTxGasLimit))
			}

			if len(args.PayBidTx) != 0 {
				if err := payBidTx.UnmarshalBinary(args.PayBidTx); err != nil {
					return common.Hash{}, newInvalidBidError(fmt.Sprintf("unmarshal transfer tx err:%v", err))
				}
			}
		}
	}

	builder, err := types.ParseBidSignature(args)
	if err != nil {
		return common.Hash{}, newBidError(err, InvalidBidParamError)
	}

	if m.antsPool.Cap()-m.antsPool.Running() < len(bid.Txs) {
		return common.Hash{}, newBidError(errors.New("mev service busy"), MevBusyError)
	}

	signer := m.b.Signer(int64(bid.BlockNumber))

	var wg sync.WaitGroup
	for i, encodedTx := range bid.Txs {
		i := i
		encodedTx := encodedTx
		wg.Add(1)

		err = m.antsPool.Submit(func() {
			defer wg.Done()

			var er error
			tx := new(types.Transaction)
			er = tx.UnmarshalBinary(encodedTx)
			if er != nil {
				return
			}

			_, er = types.Sender(signer, tx)
			if er != nil {
				return
			}

			bidTxs[i] = tx
		})

		if err != nil {
			return common.Hash{}, newBidError(errors.New("mev service busy"), MevBusyError)
		}
	}

	wg.Wait()

	for _, v := range bidTxs {
		if v == nil {
			return common.Hash{}, newInvalidBidError("invalid tx in bid")
		}
	}

	txs := make([]*types.Transaction, 0)
	txs = append(txs, bidTxs...)
	if len(payBidTx.Data()) != 0 {
		txs = append(txs, payBidTx)
	}

	innerBid := &types.Bid{
		Builder:     builder,
		BlockNumber: bid.BlockNumber,
		ParentHash:  bid.ParentHash,
		Txs:         txs,
		GasUsed:     bid.GasUsed + args.PayBidTxGasUsed,
		GasFee:      bid.GasFee,
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
