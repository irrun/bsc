package miner

import (
	"github.com/ethereum/go-ethereum/core/types"
)

// fillTransactions retrieves the pending transactions from the txpool and fills them
// into the given sealing block. The transaction selection and ordering strategy can
// be customized with the plugin in the future.
// Returns error if any, otherwise the bundles that made it into the block and all bundles that passed simulation
func (w *worker) fillTransactionsMev(interrupt chan int32, env *environment) error {
	// localTxs

	// remoteTxs

	bundles, ccBundleCh := w.eth.TxPool().MevBundles(env.header.Number, env.header.Time)
	bundles = append(bundles, <-ccBundleCh...)

	// convert bundles to bundleTxs
	var bundleTxs types.Transactions

	if err := w.commitBundle(env, bundleTxs, interrupt); err != nil {
		return err
	}

	// commit local tx

	// commit remote tx

	return nil
}

func (w *worker) commitBundle(env *environment, txs types.Transactions, interrupt chan int32) error {
	for _, _ = range txs {
		//  commit tx
		// if tx failed, return error
	}

	return nil
}
