package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Bid represents a bid.
type Bid struct {
	BlockNumber uint64          `json:"blockNumber"`
	ParentHash  common.Hash     `json:"parentHash"`
	GasUsed     uint64          `json:"gasUsed"`
	GasFee      uint64          `json:"gasFee"`
	BuilderFee  uint64          `json:"builder_fee"`
	Txs         []hexutil.Bytes `json:"txs,omitempty"`
	Timestamp   int64           `json:"timestamp"`
}

// BidArgs represents the arguments to submit a bid.
type BidArgs struct {
	// bid
	Bid *Bid
	// signed signature of the bid
	Signature string `json:"signature"`
}
