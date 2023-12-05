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
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	maxBidPerBuilder = 3

	commitInterruptNewBid int32 = iota
)

var (
	diffInTurn = big.NewInt(2) // Block difficulty for in-turn signatures

	// TODO(renee) get this value
	validatorCommission = big.NewInt(1)
)

type simBidReq struct {
	bid         *BidRuntime
	timestamp   int64
	interruptCh chan int32
}

type bidSimulator struct {
	*worker

	config *MevConfig

	running int32 // 0: not running, 1: running
	stopCh  chan struct{}

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

func newBidSimulator(config *MevConfig, worker *worker) *bidSimulator {
	w := &bidSimulator{
		worker:        worker,
		config:        config,
		running:       0,
		builders:      make(map[common.Address]*ethclient.Client),
		simBidCh:      make(chan *simBidReq),
		newBidCh:      make(chan *types.Bid),
		pending:       make(map[common.Hash]map[common.Address]map[common.Hash]struct{}),
		bestBid:       make(map[common.Hash]*BidRuntime),
		simulatingBid: make(map[common.Hash]*BidRuntime),
	}

	sentryCli, err := ethclient.Dial(config.SentryURL)
	if err != nil {
		log.Error("BidSimulator: failed to dial sentry", "url", config.SentryURL, "err", err)
	}

	w.sentryCli = sentryCli

	for _, v := range config.Builders {
		var builderCli *ethclient.Client
		if sentryCli != nil {
			builderCli = sentryCli
		} else {
			builderCli, err = ethclient.Dial(v.URL)
			if err != nil {
				log.Error("BidSimulator: failed to dial builder", "url", v.URL, "err", err)
				continue
			}
		}

		w.builders[v.Address] = builderCli
	}

	if len(w.builders) == 0 {
		log.Warn("BidSimulator: No valid builders")
	}

	return w
}

func (w *bidSimulator) Start() error {
	if atomic.LoadInt32(&w.running) == 1 {
		return errors.New("bid simulator is already running")
	}

	atomic.StoreInt32(&w.running, 1)
	w.stopCh = make(chan struct{})

	go w.mainLoop()
	go w.newBidLoop()
	go w.clearLoop()

	return nil
}

func (w *bidSimulator) Stop() error {
	atomic.StoreInt32(&w.running, 0)
	close(w.stopCh)

	return nil
}

func (w *bidSimulator) Running() bool {
	return atomic.LoadInt32(&w.running) == 1
}

func (w *bidSimulator) AddBuilder(builder common.Address, url string) error {
	w.buildersMu.Lock()
	defer w.buildersMu.Unlock()

	if w.sentryCli != nil {
		w.builders[builder] = w.sentryCli
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

		w.builders[builder] = builderCli
	}

	return nil
}

func (w *bidSimulator) RemoveBuilder(builder common.Address) error {
	w.buildersMu.Lock()
	defer w.buildersMu.Unlock()

	delete(w.builders, builder)

	return nil
}

func (w *bidSimulator) ExistBuilder(builder common.Address) bool {
	w.buildersMu.RLock()
	defer w.buildersMu.RUnlock()

	_, ok := w.builders[builder]

	return ok
}

func (w *bidSimulator) SetBestBid(prevBlockHash common.Hash, bid *BidRuntime) {
	w.bestBidMu.Lock()
	defer w.bestBidMu.Unlock()

	w.bestBid[prevBlockHash] = bid
}

func (w *bidSimulator) GetBestBid(prevBlockHash common.Hash) *BidRuntime {
	w.bestBidMu.RLock()
	defer w.bestBidMu.RUnlock()

	return w.bestBid[prevBlockHash]
}

func (w *bidSimulator) SetSimulatingBid(prevBlockHash common.Hash, bid *BidRuntime) {
	w.simBidMu.Lock()
	defer w.simBidMu.Unlock()

	w.simulatingBid[prevBlockHash] = bid
}

func (w *bidSimulator) GetSimulatingBid(prevBlockHash common.Hash) *BidRuntime {
	w.simBidMu.RLock()
	defer w.simBidMu.RUnlock()

	return w.simulatingBid[prevBlockHash]
}

func (w *bidSimulator) mainLoop() {
	for {
		select {
		case req := <-w.simBidCh:
			w.simBid(req.interruptCh, req.timestamp, req.bid)

		// System stopped
		case <-w.exitCh:
			return

		case <-w.stopCh:
			return
		}
	}
}

func (w *bidSimulator) newBidLoop() {
	if !w.Running() {
		return
	}

	var (
		interruptCh chan int32
		timestamp   int64
	)

	// commit aborts in-flight bid execution with given signal and resubmits a new one.
	commit := func(reason int32, bidRuntime *BidRuntime) {
		// TODO(renee) judge the left time is enough or not to do simulation, if not, return

		if interruptCh != nil {
			// each commit work will have its own interruptCh to stop work with a reason
			interruptCh <- reason
			close(interruptCh)
		}
		interruptCh = make(chan int32, 1)
		select {
		case w.simBidCh <- &simBidReq{interruptCh: interruptCh, bid: bidRuntime, timestamp: timestamp}:
		case <-w.exitCh:
			return
		case <-w.stopCh:
			return
		}
	}

	for {
		select {
		case bid := <-w.newBidCh:
			// check the block reward and validator reward of the bid
			prevBlockHash := bid.ParentHash
			bestBid := w.GetBestBid(prevBlockHash)
			simulatingBid := w.GetSimulatingBid(prevBlockHash)

			expectedBlockReward := big.NewInt(int64(bid.GasFee))
			expectedValidatorReward := new(big.Int).Mul(expectedBlockReward, validatorCommission)
			expectedValidatorReward.Sub(expectedValidatorReward, bid.BuilderFee)

			if expectedValidatorReward.Cmp(big.NewInt(0)) < 0 {
				// damage self profit, ignore
				continue
			}

			bidRuntime := &BidRuntime{
				bid:                     bid,
				env:                     nil,
				expectedBlockReward:     expectedBlockReward,
				expectedValidatorReward: expectedValidatorReward,
				packedBlockReward:       big.NewInt(0),
				packedValidatorReward:   big.NewInt(0),
			}

			firstBid := simulatingBid == nil && bestBid == nil
			betterThanSimulatingBid := simulatingBid != nil && bidRuntime.expectedBlockReward.Cmp(simulatingBid.expectedBlockReward) > 0
			betterThanBestBid := bestBid != nil && bidRuntime.expectedBlockReward.Cmp(bestBid.expectedBlockReward) > 0

			// if better than last best bid or simulating bid, commit for simulation
			if firstBid || betterThanSimulatingBid || betterThanBestBid {
				if len(bid.Txs) > 0 {
					timestamp = time.Now().Unix()
					commit(commitInterruptNewBid, bidRuntime)
				} else {
					// TODO two-round interaction
					// go fetchTxs() from builder, when return call BidBlock() again
				}
			}

		case <-w.exitCh:
			return

		case <-w.stopCh:
			return
		}
	}
}

func (w *bidSimulator) clearLoop() {
	chainHeadCh := make(chan core.ChainHeadEvent, chainHeadChanSize)
	chainHeadSub := w.eth.BlockChain().SubscribeChainHeadEvent(chainHeadCh)

	defer chainHeadSub.Unsubscribe()

	clearPending := func(parentHash common.Hash) {
		w.pendingMu.Lock()
		defer w.pendingMu.Unlock()

		for h, _ := range w.pending {
			delete(w.pending, h)
		}
	}

	clearBestBid := func(parentHash common.Hash) {
		w.bestBidMu.Lock()
		defer w.bestBidMu.Unlock()

		for h, _ := range w.bestBid {
			delete(w.bestBid, h)
		}
	}

	clearSimingBid := func(parentHash common.Hash) {
		w.simBidMu.Lock()
		defer w.simBidMu.Unlock()

		for h, _ := range w.simulatingBid {
			delete(w.simulatingBid, h)
		}
	}

	for {
		select {
		case head := <-chainHeadCh:
			if !w.isRunning() {
				continue
			}
			clearPending(head.Block.ParentHash())
			clearBestBid(head.Block.ParentHash())
			clearSimingBid(head.Block.ParentHash())

		case <-w.exitCh:
			return

		case <-w.stopCh:
			return

		case <-chainHeadSub.Err():
			return
		}
	}
}

// BidBlock adds bid into pending,
// if one-round bidding, goes into simulation routine,
// if two-round bidding, goes into wait-for-fetch routine.
func (w *bidSimulator) BidBlock(ctx context.Context, bid *types.Bid) error {
	if !w.ExistBuilder(bid.Builder) {
		return errors.New("builder is not registered")
	}

	var (
		builder    = bid.Builder
		parentHash = bid.ParentHash
	)

	// check if bid exists or if builder sends too many bids
	{
		w.pendingMu.Lock()

		if _, ok := w.pending[parentHash]; !ok {
			w.pending[parentHash] = make(map[common.Address]map[common.Hash]struct{})
		}

		if _, ok := w.pending[parentHash][builder]; !ok {
			w.pending[parentHash][builder] = make(map[common.Hash]struct{})
		}

		if _, ok := w.pending[parentHash][builder][bid.Hash()]; ok {
			return errors.New("bid is already exists")
		}

		if len(w.pending[parentHash][builder]) >= maxBidPerBuilder {
			return errors.New("too many bids")
		}

		w.pending[parentHash][builder][bid.Hash()] = struct{}{}

		w.pendingMu.Unlock()
	}

	// pass checking, add bid into alternative chan waiting for judge profit
	w.newBidCh <- bid

	return nil
}

// simBid simulates a BidBlock with txs
func (w *bidSimulator) simBid(interruptCh chan int32, timestamp int64, bidRuntime *BidRuntime) {
	// Abort committing if node is still syncing
	if w.syncing.Load() {
		return
	}

	// set the current simulating bid
	w.SetSimulatingBid(bidRuntime.bid.ParentHash, bidRuntime)

	var (
		bidBlockNumber = bidRuntime.bid.BlockNumber
		bidParentHash  = bidRuntime.bid.ParentHash
		bidFrom        = bidRuntime.bid.Builder
	)

	var coinbase common.Address
	// Set the coinbase if the worker is running, or it's required
	if w.isRunning() {
		coinbase = w.etherbase()
		if coinbase == (common.Address{}) {
			log.Error("refusing to mine without etherbase")
			return
		}
	}

	work, err := w.worker.prepareWork(&generateParams{
		timestamp:  uint64(timestamp),
		parentHash: bidRuntime.bid.ParentHash,
		coinbase:   coinbase,
		prevWork:   nil,
	})
	if err != nil {
		log.Error("failed to prepare work for bid", "number", bidBlockNumber,
			"prevHash", bidParentHash, "builder", bidFrom, "err", err)
		return
	}

	bidRuntime.env = work

	if bidRuntime.bid.GasUsed > work.header.GasLimit {
		log.Error("bid gas used exceeds gas limit, ignore", "number", bidBlockNumber,
			"prevHash", bidParentHash, "builder", bidFrom, "gasUsed", bidRuntime.bid.GasUsed,
			"gasLimit", work.header.GasLimit)
		go w.reportIssue(bidRuntime, "gas used exceeds gas limit")
		return
	}

	var ctx = context.Background() // TODO(renee): interrupt by timeout

LOOP:
	for {
		select {
		case signal := <-interruptCh:
			log.Debug("bid simulation abort due to interruption", "reason", signalToErr(signal))
			return

		case <-ctx.Done():
			log.Debug("bid simulation abort due to timeout")
			break LOOP

		default:
		}

		if bidRuntime.env.tcount >= len(bidRuntime.bid.Txs) {
			break LOOP
		}

		// if gas exceeds limit, exit directly
		if gasLeft := bidRuntime.env.gasPool.Gas(); gasLeft < params.TxGas {
			log.Warn("not enough gas for further transactions", "have", gasLeft, "want", params.TxGas)
			break LOOP
		}

		tx := bidRuntime.bid.Txs[bidRuntime.env.tcount]
		_, err = bidRuntime.commitTransaction(w.chain, w.chainConfig, tx)
		switch {
		case errors.Is(err, nil):
			// Everything ok, collect the logs and shift in the next transaction from the same account
			bidRuntime.env.tcount++

		default:
			log.Error("failed to commit transaction", "number", bidBlockNumber,
				"prevHash", bidParentHash, "builder", bidFrom, "err", err)
			go w.reportIssue(bidRuntime, fmt.Sprintf("invalid tx in bid, %v", err.Error()))
			break LOOP
		}
	}

	bidRuntime.Finalize()

	// return if bid is invalid, reportIssue issue to mev-sentry/builder if simulation is fully done
	if !bidRuntime.Valid() {
		if bidRuntime.FullyExecuted() {
			go w.reportIssue(bidRuntime, "reward is wrong")
		}
		return
	}

	bestBid := w.GetBestBid(bidRuntime.bid.ParentHash)
	firstBid := bestBid == nil
	betterThanBestBid := !firstBid && bidRuntime.PackedBlockReward().Cmp(bestBid.PackedBlockReward()) > 0

	if firstBid || betterThanBestBid {
		w.SetBestBid(bidRuntime.bid.ParentHash, bidRuntime)
		return
	}
}

// reportIssue reports the issue to the mev-sentry
func (w *bidSimulator) reportIssue(bidRuntime *BidRuntime, msg string) {
	cli := w.builders[bidRuntime.bid.Builder]
	if cli != nil {
		cli.ReportIssue(context.Background(), &types.BidIssue{
			BidHash: bidRuntime.bid.Hash(),
			Message: msg,
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
}

func (r *BidRuntime) Valid() bool {
	return r.FullyExecuted() &&
		r.packedBlockReward.Cmp(r.expectedBlockReward) >= 0 &&
		r.packedValidatorReward.Cmp(r.expectedValidatorReward) >= 0
}

func (r *BidRuntime) FullyExecuted() bool {
	return r.env.tcount == len(r.bid.Txs)
}

func (r *BidRuntime) PackedBlockReward() *big.Int {
	if r.packedValidatorReward != nil {
		return r.packedValidatorReward
	}
	return big.NewInt(0)
}

func (r *BidRuntime) PackedValidatorReward() *big.Int {
	if r.packedValidatorReward != nil {
		return r.packedValidatorReward
	}
	return big.NewInt(0)
}

// Finalize calculates packedBlockReward and packedValidatorReward
func (r *BidRuntime) Finalize() {
	r.packedBlockReward = r.env.state.GetBalance(consensus.SystemAddress)
	r.packedValidatorReward = new(big.Int).Mul(r.packedBlockReward, validatorCommission)
	r.packedValidatorReward.Sub(r.packedBlockReward, r.bid.BuilderFee)
}

func (r *BidRuntime) commitTransaction(chain *core.BlockChain, chainConfig *params.ChainConfig, tx *types.Transaction) (
	*types.Receipt, error) {
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
