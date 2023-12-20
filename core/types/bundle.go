package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

type Bundle struct {
	// TODO not export
	Txs               Transactions
	MaxBlockNumber    rpc.BlockNumber
	MinTimestamp      uint64
	MaxTimestamp      uint64
	RevertingTxHashes []common.Hash

	Hash common.Hash `rlp:"-"`
}

type SimulatedBundle struct {
	// TODO not export

	MevGasPrice       *big.Int
	TotalEth          *big.Int
	EthSentToCoinbase *big.Int
	TotalGasUsed      uint64
	OriginalBundle    *Bundle
}
