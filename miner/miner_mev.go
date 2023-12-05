package miner

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type BuilderConfig struct {
	Address common.Address
	URL     string
}

type MevConfig struct {
	Enabled                    bool            // Whether to enable MEV or not
	SentryURL                  string          // The url of MEV sentry
	Builders                   []BuilderConfig // The list of builders
	CommissionRate             float64         // Commission rate of staking for the current validator
	MaxAllowDurationForBidding time.Duration   // Max allowed duration for accepting bids in two rounds interactions
	TimeoutForRetrieveTxs      time.Duration   // Timeout duration for retrieving txs from a builder in two rounds interactions
	SlashBlocksForTxsTimeout   int64           // The count of blocks for slashing a builder when retrieving txs timeout; restart node will cancel slashes.
	SlashBlocksForInvalidBlock int64           // The count of blocks for slashing a builder when the proposed block is invalid; restart node will cancel slashes.
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
	return miner.worker.BidBlock(ctx, bid)
}
