package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"sort"

	"github.com/google/uuid"

	"github.com/ethereum/go-ethereum/common"
)

type MevBundle struct {
	Txs               Transactions
	BlockNumber       *big.Int
	Uuid              uuid.UUID
	SigningAddress    common.Address
	MinTimestamp      uint64
	MaxTimestamp      uint64
	RevertingTxHashes []common.Hash
	Hash              common.Hash
}

func (b *MevBundle) UniquePayload() []byte {
	var buf []byte
	buf = binary.AppendVarint(buf, b.BlockNumber.Int64())
	buf = append(buf, b.Hash[:]...)
	sort.Slice(b.RevertingTxHashes, func(i, j int) bool {
		return bytes.Compare(b.RevertingTxHashes[i][:], b.RevertingTxHashes[j][:]) <= 0
	})
	for _, txHash := range b.RevertingTxHashes {
		buf = append(buf, txHash[:]...)
	}
	return buf
}

func (b *MevBundle) ComputeUUID() uuid.UUID {
	return uuid.NewHash(sha256.New(), uuid.Nil, b.UniquePayload(), 5)
}

func (b *MevBundle) RevertingHash(hash common.Hash) bool {
	for _, revHash := range b.RevertingTxHashes {
		if revHash == hash {
			return true
		}
	}
	return false
}
