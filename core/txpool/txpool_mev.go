package txpool

import (
	"math/big"

	"github.com/google/uuid"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// MevBundles returns a list of bundles valid for the given blockNumber/blockTimestamp
// also prunes bundles that are outdated
// Returns regular bundles and a function resolving to current cancellable bundles
func (pool *TxPool) MevBundles(blockNumber *big.Int, blockTimestamp uint64) ([]types.MevBundle, chan []types.MevBundle) {
	// returned values
	var ret []types.MevBundle

	return ret, nil
}

// AddMevBundle adds a mev bundle to the pool
func (pool *TxPool) AddMevBundle(
	txs types.Transactions,
	blockNumber *big.Int,
	replacementUuid uuid.UUID,
	signingAddress common.Address,
	minTimestamp, maxTimestamp uint64,
	revertingTxHashes []common.Hash) error {
	return nil
}

func (pool *TxPool) AddSBundle(bundle *types.SBundle) error {
	return pool.sbundles.Add(bundle)
}

func (pool *TxPool) CancelSBundles(hashes []common.Hash) {
	pool.sbundles.Cancel(hashes)
}
