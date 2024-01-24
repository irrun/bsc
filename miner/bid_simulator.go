package miner

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// maxBidPerBuilder is the max bid number per builder
	maxBidPerBuilder = 3

	commitInterruptBetterBid = 1
)

var (
	diffInTurn = big.NewInt(2) // the difficulty of a block that proposed by an in-turn validator

	systemTxsGas = params.SystemTxsGas
)

type SimulationWorkPreparer interface {
	prepareWork(params *generateParams) (*environment, error)
}

// simBidReq is the request for simulating a bid
type simBidReq struct {
	bid         *BidRuntime
	timestamp   int64
	interruptCh chan int32
}

// bidSimulator is in charge of receiving bid from builders, reporting issue to builders.
// And take care of bid simulation, rewards computing, best bid maintaining.
type bidSimulator struct {
	config        *MevConfig
	delayLeftOver time.Duration
	chain         *core.BlockChain
	chainConfig   *params.ChainConfig
	workPreparer  SimulationWorkPreparer

	running atomic.Bool // controlled by miner
	exitCh  chan struct{}

	bidReceiving atomic.Bool // controlled by config and eth.AdminAPI

	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription

	sentryCli *ethclient.Client

	// builder info (warning: only keep status in memory!)
	buildersMu sync.RWMutex
	builders   map[common.Address]*ethclient.Client

	// channels
	simBidCh chan *simBidReq
	newBidCh chan *types.Bid

	pendingMu sync.RWMutex
	pending   map[common.Hash]map[common.Address]map[common.Hash]struct{} // prevBlockHash -> builder -> bidHash -> struct{}

	bestBidMu sync.RWMutex
	bestBid   map[common.Hash]*BidRuntime // prevBlockHash -> bidRuntime

	simBidMu      sync.RWMutex
	simulatingBid map[common.Hash]*BidRuntime // prevBlockHash -> bidRuntime, in the process of simulation
}

func newBidSimulator(
	config *MevConfig,
	delayLeftOver time.Duration,
	chainConfig *params.ChainConfig,
	chain *core.BlockChain,
	workPreparer SimulationWorkPreparer,
) *bidSimulator {
	b := &bidSimulator{
		config:        config,
		delayLeftOver: delayLeftOver,
		chainConfig:   chainConfig,
		chain:         chain,
		workPreparer:  workPreparer,
		exitCh:        make(chan struct{}),
		chainHeadCh:   make(chan core.ChainHeadEvent, chainHeadChanSize),
		builders:      make(map[common.Address]*ethclient.Client),
		simBidCh:      make(chan *simBidReq),
		newBidCh:      make(chan *types.Bid),
		pending:       make(map[common.Hash]map[common.Address]map[common.Hash]struct{}),
		bestBid:       make(map[common.Hash]*BidRuntime),
		simulatingBid: make(map[common.Hash]*BidRuntime),
	}

	b.chainHeadSub = chain.SubscribeChainHeadEvent(b.chainHeadCh)

	if config.Enabled {
		b.bidReceiving.Store(true)
	}

	var sentryCli *ethclient.Client
	var err error

	if config.SentryURL != "" {
		sentryCli, err = ethclient.Dial(config.SentryURL)
		if err != nil {
			log.Error("BidSimulator: failed to dial sentry", "url", config.SentryURL, "err", err)
		}
	}

	b.sentryCli = sentryCli

	for _, v := range config.Builders {
		var builderCli *ethclient.Client

		if b.sentryCli != nil {
			builderCli = b.sentryCli
		} else {
			builderCli, err = ethclient.Dial(v.URL)
			if err != nil {
				log.Error("BidSimulator: failed to dial builder", "url", v.URL, "err", err)
				continue
			}
		}

		b.builders[v.Address] = builderCli
	}

	if len(b.builders) == 0 {
		log.Warn("BidSimulator: No valid builders")
	}

	go b.clearLoop()
	go b.mainLoop()
	go b.newBidLoop()

	return b
}

func (b *bidSimulator) start() {
	b.running.Store(true)
}

func (b *bidSimulator) stop() {
	b.running.Store(false)
}

func (b *bidSimulator) close() {
	b.running.Store(false)
	close(b.exitCh)
}

func (b *bidSimulator) isRunning() bool {
	return b.running.Load()
}

func (b *bidSimulator) receivingBid() bool {
	return b.bidReceiving.Load()
}

func (b *bidSimulator) startReceivingBid() {
	b.bidReceiving.Store(true)
}

func (b *bidSimulator) stopReceivingBid() {
	b.bidReceiving.Store(false)
}

func (b *bidSimulator) AddBuilder(builder common.Address, url string) error {
	b.buildersMu.Lock()
	defer b.buildersMu.Unlock()

	if b.sentryCli != nil {
		b.builders[builder] = b.sentryCli
	} else {
		var builderCli *ethclient.Client

		if url != "" {
			var err error

			builderCli, err = ethclient.Dial(url)
			if err != nil {
				log.Error("BidSimulator: failed to dial builder", "url", url, "err", err)
				return err
			}
		}

		b.builders[builder] = builderCli
	}

	return nil
}

func (b *bidSimulator) RemoveBuilder(builder common.Address) error {
	b.buildersMu.Lock()
	defer b.buildersMu.Unlock()

	delete(b.builders, builder)

	return nil
}

func (b *bidSimulator) ExistBuilder(builder common.Address) bool {
	b.buildersMu.RLock()
	defer b.buildersMu.RUnlock()

	_, ok := b.builders[builder]

	return ok
}

func (b *bidSimulator) SetBestBid(prevBlockHash common.Hash, bid *BidRuntime) {
	b.bestBidMu.Lock()
	defer b.bestBidMu.Unlock()

	b.bestBid[prevBlockHash] = bid
}

func (b *bidSimulator) GetBestBid(prevBlockHash common.Hash) *BidRuntime {
	b.bestBidMu.RLock()
	defer b.bestBidMu.RUnlock()

	return b.bestBid[prevBlockHash]
}

func (b *bidSimulator) GetSimulatingBid(prevBlockHash common.Hash) *BidRuntime {
	b.simBidMu.RLock()
	defer b.simBidMu.RUnlock()

	return b.simulatingBid[prevBlockHash]
}

func (b *bidSimulator) mainLoop() {
	defer b.chainHeadSub.Unsubscribe()

	for {
		select {
		case req := <-b.simBidCh:
			if !b.isRunning() {
				continue
			}

			b.simBid(req.interruptCh, req.timestamp, req.bid)

		// System stopped
		case <-b.exitCh:
			return

		case <-b.chainHeadSub.Err():
			return
		}
	}
}

func (b *bidSimulator) newBidLoop() {
	var (
		interruptCh chan int32
		timestamp   int64
	)

	// commit aborts in-flight bid execution with given signal and resubmits a new one.
	commit := func(reason int32, bidRuntime *BidRuntime) {
		// if the left time is not enough to do simulation, return
		var simDuration time.Duration
		if lastBid := b.GetBestBid(bidRuntime.bid.ParentHash); lastBid != nil && lastBid.duration != 0 {
			simDuration = lastBid.duration
		}
		// simulatingBid's duration is longer than bestBid's duration in most case
		if lastBid := b.GetSimulatingBid(bidRuntime.bid.ParentHash); lastBid != nil && lastBid.duration != 0 {
			simDuration = lastBid.duration
		}
		if time.Until(b.bidMustBefore()) <= simDuration {
			return
		}

		if interruptCh != nil {
			// each commit work will have its own interruptCh to stop work with a reason
			interruptCh <- reason
			close(interruptCh)
		}
		interruptCh = make(chan int32, 1)
		select {
		case b.simBidCh <- &simBidReq{interruptCh: interruptCh, bid: bidRuntime, timestamp: timestamp}:
		case <-b.exitCh:
			return
		}
	}

	for {
		select {
		case newBid := <-b.newBidCh:
			if !b.isRunning() {
				continue
			}

			// check the block reward and validator reward of the newBid
			expectedBlockReward := newBid.GasFee
			expectedValidatorReward := new(big.Int).Mul(expectedBlockReward, big.NewInt(b.config.ValidatorCommission))
			expectedValidatorReward.Div(expectedValidatorReward, big.NewInt(10000))
			expectedValidatorReward.Sub(expectedValidatorReward, newBid.BuilderFee)

			if expectedValidatorReward.Cmp(big.NewInt(0)) < 0 {
				// damage self profit, ignore
				continue
			}

			bidRuntime := &BidRuntime{
				bid:                     newBid,
				expectedBlockReward:     expectedBlockReward,
				expectedValidatorReward: expectedValidatorReward,
				packedBlockReward:       big.NewInt(0),
				packedValidatorReward:   big.NewInt(0),
			}

			// TODO(renee-) opt bid comparation

			simulatingBid := b.GetSimulatingBid(newBid.ParentHash)
			// simulatingBid is nil means there is no bid in simulation
			if simulatingBid == nil {
				// bestBid is nil means bid is the first bid
				bestBid := b.GetBestBid(newBid.ParentHash)
				if bestBid == nil {
					timestamp = time.Now().Unix()
					commit(commitInterruptBetterBid, bidRuntime)
					continue
				}

				// if bestBid is not nil, check if newBid is better than bestBid
				if bidRuntime.expectedBlockReward.Cmp(bestBid.expectedBlockReward) > 0 &&
					bidRuntime.expectedValidatorReward.Cmp(bestBid.expectedValidatorReward) > 0 {
					// if both reward are better than last simulating newBid, commit for simulation
					timestamp = time.Now().Unix()
					commit(commitInterruptBetterBid, bidRuntime)
					continue
				}

				continue
			}

			// simulatingBid must be better than bestBid, if newBid is better than simulatingBid, commit for simulation
			if bidRuntime.expectedBlockReward.Cmp(simulatingBid.expectedBlockReward) > 0 &&
				bidRuntime.expectedValidatorReward.Cmp(simulatingBid.expectedValidatorReward) > 0 {
				// if both reward are better than last simulating newBid, commit for simulation
				timestamp = time.Now().Unix()
				commit(commitInterruptBetterBid, bidRuntime)
				continue
			}

		case <-b.exitCh:
			return
		}
	}
}

func (b *bidSimulator) bidMustBefore() time.Time {
	currentHeader := b.chain.CurrentHeader()
	nextHeaderTimestamp := currentHeader.Time + b.chainConfig.Parlia.Period
	return time.Unix(int64(nextHeaderTimestamp), 0).Add(-b.delayLeftOver)
}

func (b *bidSimulator) clearLoop() {
	clearFn := func(parentHash common.Hash) {
		b.pendingMu.Lock()
		delete(b.pending, parentHash)
		b.pendingMu.Unlock()

		b.bestBidMu.Lock()
		if bid, ok := b.bestBid[parentHash]; ok {
			bid.env.discard()
		}
		delete(b.bestBid, parentHash)
		b.bestBidMu.Unlock()

		b.simBidMu.Lock()
		if bid, ok := b.simulatingBid[parentHash]; ok {
			bid.env.discard()
		}
		delete(b.simulatingBid, parentHash)
		b.simBidMu.Unlock()
	}

	for {
		select {
		case head := <-b.chainHeadCh:
			if !b.isRunning() {
				continue
			}

			clearFn(head.Block.ParentHash())
		}
	}
}

// sendBid checks if the bid is already exists or if the builder sends too many bids,
// if yes, return error, if not, add bid into newBid chan waiting for judge profit.
func (b *bidSimulator) sendBid(ctx context.Context, bid *types.Bid) error {
	if !b.ExistBuilder(bid.Builder) {
		return errors.New("builder is not registered")
	}

	err := b.pendingCheck(bid)
	if err != nil {
		return err
	}

	// pass checking, add bid into alternative chan waiting for judge profit
	b.newBidCh <- bid

	return nil
}

func (b *bidSimulator) pendingCheck(bid *types.Bid) error {
	var (
		builder    = bid.Builder
		parentHash = bid.ParentHash
	)

	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()

	// check if bid exists or if builder sends too many bids
	if _, ok := b.pending[parentHash]; !ok {
		b.pending[parentHash] = make(map[common.Address]map[common.Hash]struct{})
	}

	if _, ok := b.pending[parentHash][builder]; !ok {
		b.pending[parentHash][builder] = make(map[common.Hash]struct{})
	}

	if _, ok := b.pending[parentHash][builder][bid.Hash()]; ok {
		return errors.New("bid already exists")
	}

	if len(b.pending[parentHash][builder]) >= maxBidPerBuilder {
		return errors.New("too many bids")
	}

	b.pending[parentHash][builder][bid.Hash()] = struct{}{}

	return nil
}

// simBid simulates a newBid with txs.
// simBid does not enable state prefetching when commit transaction.
func (b *bidSimulator) simBid(interruptCh chan int32, timestamp int64, bidRuntime *BidRuntime) {
	// prevent from stopping happen in time interval from sendBid to simBid
	if !b.isRunning() || !b.receivingBid() {
		return
	}

	var (
		blockNumber = bidRuntime.bid.BlockNumber
		parentHash  = bidRuntime.bid.ParentHash
		builder     = bidRuntime.bid.Builder
		err         error
		success     bool
	)

	// ensure simulation exited then start next simulation
	b.simBidMu.Lock()
	b.simulatingBid[parentHash] = bidRuntime

	defer func(simStart time.Time) {
		logCtx := []any{
			"blockNumber", blockNumber,
			"parentHash", parentHash,
			"builder", builder,
			"gasUsed", bidRuntime.bid.GasUsed,
		}

		if bidRuntime.env != nil {
			logCtx = append(logCtx, "gasLimit", bidRuntime.env.header.GasLimit)

			if err != nil || !success {
				bidRuntime.env.discard()
			}
		}

		if err != nil {
			logCtx = append(logCtx, "err", err)
			log.Debug("bid simulation failed", logCtx...)

			go b.reportIssue(bidRuntime, err)
		}

		if success {
			bidRuntime.duration = time.Since(simStart)
		}

		b.simulatingBid[parentHash] = nil
		b.simBidMu.Unlock()
	}(time.Now())

	// prepareWork will configure header with a suitable time according to consensus
	// prepareWork will start trie prefetching
	if bidRuntime.env, err = b.workPreparer.prepareWork(&generateParams{
		timestamp:  uint64(timestamp),
		parentHash: bidRuntime.bid.ParentHash,
	}); err != nil {
		return
	}

	if bidRuntime.bid.GasUsed > (bidRuntime.env.header.GasLimit - systemTxsGas) {
		err = errors.New("gas used exceeds gas limit")
		return
	}

	for _, tx := range bidRuntime.bid.Txs {
		select {
		case _ = <-interruptCh:
			err = errors.New("simulation abort due to better bid arrived")
			return

		case <-b.exitCh:
			err = errors.New("miner exit")
			return

		default:
		}

		_, err = bidRuntime.commitTransaction(b.chain, b.chainConfig, tx)
		if err != nil {
			err = fmt.Errorf("invalid tx in bid, %v", err)
			return
		}
	}

	bidRuntime.packedReward(b.config.ValidatorCommission)

	// return if bid is invalid, reportIssue issue to mev-sentry/builder if simulation is fully done
	if !bidRuntime.valid() {
		err = errors.New("reward does not achieve the expectation")
		return
	}

	bestBid := b.GetBestBid(parentHash)

	if bestBid == nil {
		b.SetBestBid(bidRuntime.bid.ParentHash, bidRuntime)
		success = true
		return
	}

	// TODO(renee-) opt bid comparation
	if bidRuntime.packedBlockReward.Cmp(bestBid.packedBlockReward) > 0 {
		b.SetBestBid(bidRuntime.bid.ParentHash, bidRuntime)
		success = true
		return
	}
}

// reportIssue reports the issue to the mev-sentry
func (b *bidSimulator) reportIssue(bidRuntime *BidRuntime, err error) {
	cli := b.builders[bidRuntime.bid.Builder]
	if cli != nil {
		cli.ReportIssue(context.Background(), &types.BidIssue{
			Validator:   bidRuntime.env.header.Coinbase,
			Builder:     bidRuntime.bid.Builder,
			BlockNumber: bidRuntime.bid.BlockNumber,
			ParentHash:  bidRuntime.bid.ParentHash,
			Message:     err.Error(),
		})
	}
}

type BidRuntime struct {
	bid *types.Bid

	env *environment

	expectedBlockReward     *big.Int
	expectedValidatorReward *big.Int

	packedBlockReward     *big.Int
	packedValidatorReward *big.Int

	duration time.Duration
}

func (r *BidRuntime) valid() bool {
	return r.packedBlockReward.Cmp(r.expectedBlockReward) >= 0 &&
		r.packedValidatorReward.Cmp(r.expectedValidatorReward) >= 0
}

// packedReward calculates packedBlockReward and packedValidatorReward
func (r *BidRuntime) packedReward(validatorCommission int64) {
	r.packedBlockReward = r.env.state.GetBalance(consensus.SystemAddress)
	r.packedValidatorReward = new(big.Int).Mul(r.packedBlockReward, big.NewInt(validatorCommission))
	r.packedValidatorReward.Div(r.packedValidatorReward, big.NewInt(10000))
	r.packedValidatorReward.Sub(r.packedBlockReward, r.bid.BuilderFee)
}

func (r *BidRuntime) commitTransaction(chain *core.BlockChain, chainConfig *params.ChainConfig, tx *types.Transaction) (
	*types.Receipt, error,
) {
	var (
		env  = r.env
		snap = env.state.Snapshot()
		gp   = env.gasPool.Gas()
	)

	receipt, err := core.ApplyTransaction(chainConfig, chain, &env.coinbase, env.gasPool, env.state, env.header, tx,
		&env.header.GasUsed, *chain.GetVMConfig(), core.NewReceiptBloomGenerator())
	if err != nil {
		env.state.RevertToSnapshot(snap)
		env.gasPool.SetGas(gp)
		return nil, err
	}

	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)

	return receipt, nil
}
