package miner

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/panjf2000/ants/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

const (
	MevRoutineLimit              = 10000
	TxDecodeConcurrencyForPerBid = 5
	CheckTxDecodeInterval        = 3 * time.Millisecond
)

type BuilderConfig struct {
	Address common.Address
	URL     string
}

type MevConfig struct {
	Enabled   bool            // Whether to enable Mev or not
	SentryURL string          // The url of Mev sentry
	Builders  []BuilderConfig // The list of builders

	ValidatorCommission int64 // 100 means 1%
}

// MevRunning return true if mev is running.
func (miner *Miner) MevRunning() bool {
	return miner.bidSimulator.isRunning() && miner.bidSimulator.receivingBid()
}

// StartMev starts mev.
func (miner *Miner) StartMev() {
	miner.bidSimulator.startReceivingBid()
}

// StopMev stops mev.
func (miner *Miner) StopMev() {
	miner.bidSimulator.stopReceivingBid()
}

// AddBuilder adds a builder to the bid simulator.
func (miner *Miner) AddBuilder(builder common.Address, url string) error {
	return miner.bidSimulator.AddBuilder(builder, url)
}

// RemoveBuilder removes a builder from the bid simulator.
func (miner *Miner) RemoveBuilder(builderAddr common.Address) error {
	return miner.bidSimulator.RemoveBuilder(builderAddr)
}

func (miner *Miner) SendBid(ctx context.Context, bid *types.BidArgs) (common.Hash, error) {
	builder, err := types.EcrecoverBuilder(bid)
	if err != nil {
		return common.Hash{}, types.NewInvalidBidError(fmt.Sprintf("invalid signature:%v", err))
	}

	bidTxs := make([]*BidTx, len(bid.Bid.Txs))
	signer := types.MakeSigner(miner.worker.chainConfig, big.NewInt(int64(bid.Bid.BlockNumber)), uint64(time.Now().Unix()))

	txChan := make(chan int, TxDecodeConcurrencyForPerBid)
	for i := 0; i < TxDecodeConcurrencyForPerBid; i++ {
		go func() {
			for {
				select {
				case txIndex := <-txChan:
					err = miner.antsPool.Submit(func() {
						encodedTx := bid.Bid.Txs[txIndex]
						tx := new(types.Transaction)
						er := tx.UnmarshalBinary(encodedTx)
						if er != nil {
							bidTxs[txIndex] = &BidTx{tx: nil, err: er}
							return
						}

						_, er = types.Sender(signer, tx)
						if er != nil {
							bidTxs[txIndex] = &BidTx{tx: nil, err: er}
							return
						}

						bidTxs[txIndex] = &BidTx{tx: tx, err: nil}
					})

					if err != nil {
						bidTxs[txIndex] = &BidTx{tx: nil, err: err}
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	for i := 0; i < len(bid.Bid.Txs); i++ {
		select {
		case txChan <- i:
		}
	}

	for {
		if len(bidTxs) == len(bid.Bid.Txs) {
			break
		}
		time.Sleep(CheckTxDecodeInterval)
		log.Debug("waiting for txs to be decoded")
	}

	txs := make([]*types.Transaction, 0)
	for _, v := range bidTxs {
		if v == nil {
			return common.Hash{}, types.NewInvalidBidError("invalid tx in bid")
		}

		if v.err != nil {
			if v.err == ants.ErrPoolClosed || v.err == ants.ErrPoolOverload {
				return common.Hash{}, types.ErrMevBusy
			}

			return common.Hash{}, types.NewInvalidBidError("invalid tx in bid")
		}

		if v.tx != nil {
			txs = append(txs, v.tx)
		}
	}

	if len(txs) != len(bid.Bid.Txs) {
		return common.Hash{}, types.NewInvalidBidError("invalid tx in bid")
	}

	if len(bid.PayBidTx) != 0 {
		var payBidTx = new(types.Transaction)
		if err = payBidTx.UnmarshalBinary(bid.PayBidTx); err != nil {
			return common.Hash{}, types.NewInvalidBidError(fmt.Sprintf("unmarshal transfer tx err:%v", err))
		}
		txs = append(txs, payBidTx)
	}

	innerBid := types.FromRawBid(bid.Bid, builder, txs, bid.PayBidTxGasUsed)

	bidMustBefore := miner.bidSimulator.bidMustBefore(bid.Bid.ParentHash)
	timeout := time.Until(bidMustBefore)

	if timeout <= 0 {
		return common.Hash{}, fmt.Errorf("too late, expected befor %s, appeared %s later", bidMustBefore,
			common.PrettyDuration(timeout))
	}

	err = miner.bidSimulator.sendBid(ctx, innerBid)

	if err != nil {
		return common.Hash{}, err
	}

	return innerBid.Hash(), nil
}

type BidTx struct {
	tx  *types.Transaction
	err error
}
