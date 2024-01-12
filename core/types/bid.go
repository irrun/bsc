package types

import (
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
)

// Bid represents a bid.
type Bid struct {
	Builder     common.Address
	BlockNumber uint64
	ParentHash  common.Hash
	Txs         Transactions
	GasUsed     uint64
	GasFee      *big.Int
	Timestamp   int64
	BuilderFee  *big.Int

	// caches
	hash atomic.Value
}

// Hash returns the transaction hash.
func (b *Bid) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}

	var h common.Hash
	h = rlpHash(b)

	b.hash.Store(h)
	return h
}

// BidIssue
// TODO(renee-) predefine some code/message to help builder handle the error
type BidIssue struct {
	BlockNumber uint64
	ParentHash  common.Hash
	BidHash     common.Hash
	Message     string
}
