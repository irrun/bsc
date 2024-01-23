package miner

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/utils"
)

var batchRunner = utils.NewBatchRunner().WithConcurrencyLimit(1024)

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

func (miner *Miner) SendBid(ctx context.Context, bid *types.Bid) error {
	bidMustBefore := miner.bidSimulator.bidMustBefore()
	timeout := time.Until(bidMustBefore)

	if timeout <= 0 {
		return fmt.Errorf("too late, expected befor %s, appeared %s later", bidMustBefore,
			common.PrettyDuration(timeout))
	}

	signer := types.MakeSigner(miner.worker.chainConfig, big.NewInt(int64(bid.BlockNumber)), uint64(time.Now().Unix()))

	for _, tx := range bid.Txs {
		tx := tx

		batchRunner.AddTasks(func() error {
			_, err := types.Sender(signer, tx)
			if err != nil {
				return err
			}

			return nil
		})
	}

	if err := batchRunner.Exec(); err != nil {
		return fmt.Errorf("invalid tx in bid")
	}

	return miner.bidSimulator.sendBid(ctx, bid)
}
