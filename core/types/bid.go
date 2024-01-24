package types

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// BidArgs represents the arguments to submit a bid.
type BidArgs struct {
	// bid
	Bid *RawBid
	// signed signature of the bid
	Signature string `json:"signature"`

	// PayBidTx pays to builder
	PayBidTx        hexutil.Bytes `json:"payBidTx,omitempty"`
	PayBidTxGasUsed uint64        `json:"payBidTxGasUsed"`
}

// RawBid represents a raw bid.
type RawBid struct {
	BlockNumber uint64          `json:"blockNumber"`
	ParentHash  common.Hash     `json:"parentHash"`
	Txs         []hexutil.Bytes `json:"txs,omitempty"`
	GasUsed     uint64          `json:"gasUsed"`
	GasFee      *big.Int        `json:"gasFee"`
	BuilderFee  *big.Int        `json:"builderFee"`
}

func ParseBidSignature(args BidArgs) (common.Address, error) {
	bid, err := rlp.EncodeToBytes(args.Bid)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to encode bid, %v", err)
	}

	signature, err := hexutil.Decode(args.Signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to decode signature, %v", err)
	}

	sigPublicKey, err := crypto.Ecrecover(crypto.Keccak256(bid), signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to recover signature, %v ", err)
	}

	pk, err := crypto.UnmarshalPubkey(sigPublicKey)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to unmarshal pubkey, %v", err)
	}

	return crypto.PubkeyToAddress(*pk), nil
}

// Bid represents a bid.
type Bid struct {
	Builder     common.Address
	BlockNumber uint64
	ParentHash  common.Hash
	Txs         Transactions
	GasUsed     uint64
	GasFee      *big.Int
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

// BidIssue represents a bid issue.
type BidIssue struct {
	// TODO put validator and builder here or parsing by header?
	Validator   common.Address
	Builder     common.Address
	BlockNumber uint64
	ParentHash  common.Hash
	BidHash     common.Hash
	Message     string
}
