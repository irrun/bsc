package miner

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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

// StartMev starts mev, return error if it is already running.
func (miner *Miner) StartMev() {
	miner.bidSimulator.startReceivingBid()
}

// StopMev stops mev, always return nil.
func (miner *Miner) StopMev() {
	miner.bidSimulator.stopReceivingBid()
}

// AddBuilder adds a builder to the bid simulator.
func (miner *Miner) AddBuilder(builder common.Address, builderUrl string) error {
	return miner.bidSimulator.AddBuilder(builder, builderUrl)
}

// RemoveBuilder removes a builder from the bid simulator.
func (miner *Miner) RemoveBuilder(builderAddr common.Address) error {
	return miner.bidSimulator.RemoveBuilder(builderAddr)
}

func (miner *Miner) SendBid(ctx context.Context, bid *types.Bid) error {
	currentHeader := miner.eth.BlockChain().CurrentHeader()
	nextHeaderTimestamp := currentHeader.Time + miner.worker.chainConfig.Parlia.Period
	endOfProposingWindow := time.Unix(int64(nextHeaderTimestamp), 0).Add(-miner.worker.config.DelayLeftOver)
	timeout := time.Until(endOfProposingWindow)

	if timeout <= 0 {
		return fmt.Errorf("too late, expected befor %s, appeared %s later", endOfProposingWindow,
			common.PrettyDuration(timeout))
	}

	return miner.bidSimulator.sendBid(ctx, bid)
}
