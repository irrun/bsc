package ethapi

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// BidArgs represents the arguments to submit a bid.
type BidArgs struct {
	// bid
	Bid *Bid
	// signed signature of the bid
	Signature string `json:"signature"`
}

// Bid represents a bid.
type Bid struct {
	Builder     common.Address  `json:"builder"` // TODO delete
	BlockNumber uint64          `json:"blockNumber"`
	ParentHash  common.Hash     `json:"parentHash"`
	GasLimit    uint64          `json:"gasLimit"`
	GasFee      uint64          `json:"gasFee"`
	BuilderFee  uint64          `json:"builder_fee"`
	Txs         []hexutil.Bytes `json:"txs,omitempty"`
	Timestamp   int64           `json:"timestamp"`
}

func (b BidArgs) checkParameters(currentHeader *types.Header) error {
	// basic check of args:
	// 1. count of txs must be greater than 0
	// 2. block number must be greater than current block
	// 3. parent hash must be equal to current block hash
	// 4. builder signature must be valid, address must be whitelisted
	// 5. builder fee must be less than gas fee
	// 6. txs must be valid

	return nil
}

func checkSignature(args BidArgs) error {
	hash, err := rlp.EncodeToBytes(args.Bid)
	if err != nil {
		return errors.New("fail to verify signature, err: " + err.Error())
	}

	sig := hexutil.MustDecode(args.Signature)
	sigPublicKey, err := crypto.Ecrecover(hash, sig)
	if err != nil {
		return errors.New("fail to verify signature, err: " + err.Error())
	}

	pk, err := crypto.UnmarshalPubkey(sigPublicKey)
	if err != nil {
		return errors.New("fail to verify signature, err: " + err.Error())
	}

	expected := crypto.PubkeyToAddress(*pk)
	actual := common.HexToAddress(args.Bid.Builder.String())
	if expected != actual {
		return fmt.Errorf("invalid signature: signature comes from %v not %v", expected, actual)
	}

	return nil
}
