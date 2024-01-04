package miner

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"time"
)

type BuilderConfig struct {
	Address common.Address
	URL     string
}

type MevConfig struct {
	Enabled   bool            // Whether to enable MEV or not
	SentryURL string          // The url of MEV sentry
	Builders  []BuilderConfig // The list of builders
}

func (miner *Miner) MevRunning() bool {
	if miner.worker.config.Mev == nil {
		return false
	}

	return miner.worker.bidSimulator.Running()
}

func (miner *Miner) StartMEV() error {
	return miner.worker.bidSimulator.Start()
}

func (miner *Miner) StopMEV() error {
	return miner.worker.bidSimulator.Stop()
}

func (miner *Miner) AddBuilder(builder common.Address, builderUrl string) error {
	return miner.worker.bidSimulator.AddBuilder(builder, builderUrl)
}

func (miner *Miner) RemoveBuilder(builderAddr common.Address) error {
	return miner.worker.bidSimulator.RemoveBuilder(builderAddr)
}

func (miner *Miner) BidBlock(ctx context.Context, bid *types.Bid) error {
	currentHeader := miner.eth.BlockChain().CurrentHeader()
	nextHeaderTimestamp := currentHeader.Time + miner.worker.chainConfig.Parlia.Period
	endOfProposingWindow := time.Unix(int64(nextHeaderTimestamp), 0).Add(-miner.worker.config.DelayLeftOver)
	timeout := time.Until(endOfProposingWindow)

	if timeout <= 0 {
		return fmt.Errorf("too late, expected befor %s, appeared %s later", endOfProposingWindow,
			common.PrettyDuration(timeout))
	}

	return miner.worker.BidBlock(ctx, bid)
}
