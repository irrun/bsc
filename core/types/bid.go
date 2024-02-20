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
	Signature hexutil.Bytes `json:"signature"`

	// PayBidTx pays to builder
	PayBidTx        hexutil.Bytes `json:"payBidTx"`
	PayBidTxGasUsed uint64        `json:"payBidTxGasUsed"`
}

// RawBid represents a raw bid.
type RawBid struct {
	BlockNumber uint64          `json:"blockNumber"`
	ParentHash  common.Hash     `json:"parentHash"`
	Txs         []hexutil.Bytes `json:"txs"`
	GasUsed     uint64          `json:"gasUsed"`
	GasFee      *big.Int        `json:"gasFee"`
	BuilderFee  *big.Int        `json:"builderFee"`

	hash atomic.Value
}

// Hash returns the hash of the bid.
func (b *RawBid) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}

	h := rlpHash(b)
	b.hash.Store(h)

	return h
}

func EcrecoverBuilder(args *BidArgs) (common.Address, error) {
	bid, err := rlp.EncodeToBytes(args.Bid)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to encode bid, %v", err)
	}

	pk, err := crypto.SigToPub(crypto.Keccak256(bid), args.Signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("fail to extract pubkey, %v", err)
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

	rawBid RawBid
}

func FromRawBid(bid *RawBid, builder common.Address, txs Transactions, payBidTxGasUsed uint64) *Bid {
	b := &Bid{
		Builder:     builder,
		BlockNumber: bid.BlockNumber,
		ParentHash:  bid.ParentHash,
		Txs:         txs,
		GasUsed:     bid.GasUsed + payBidTxGasUsed,
		GasFee:      bid.GasFee,
		BuilderFee:  big.NewInt(0),
		rawBid:      *bid,
	}

	if bid.BuilderFee != nil {
		b.BuilderFee = bid.BuilderFee
	}

	return b
}

// Hash returns the bid hash.
func (b *Bid) Hash() common.Hash {
	return b.rawBid.Hash()
}

// BidIssue represents a bid issue.
type BidIssue struct {
	Validator common.Address
	Builder   common.Address
	BidHash   common.Hash
	Message   string
}
