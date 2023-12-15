package ethapi

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/google/uuid"
	"math/big"
)

// ---------------------------------------------------------------- FlashBots ----------------------------------------------------------------

// PrivateTxBundleAPI offers an API for accepting bundled transactions
type PrivateTxBundleAPI struct {
	b Backend
}

// NewPrivateTxBundleAPI creates a new Tx Bundle API instance.
func NewPrivateTxBundleAPI(b Backend) *PrivateTxBundleAPI {
	return &PrivateTxBundleAPI{b}
}

// SendBundleArgs represents the arguments for a SendBundle call.
type SendBundleArgs struct {
	Txs               []hexutil.Bytes `json:"txs"`
	BlockNumber       rpc.BlockNumber `json:"blockNumber"`
	ReplacementUuid   *uuid.UUID      `json:"replacementUuid"`
	SigningAddress    *common.Address `json:"signingAddress"`
	MinTimestamp      *uint64         `json:"minTimestamp"`
	MaxTimestamp      *uint64         `json:"maxTimestamp"`
	RevertingTxHashes []common.Hash   `json:"revertingTxHashes"`
}

// SendBundle will add the signed transaction to the transaction pool.
// The sender is responsible for signing the transaction and using the correct nonce and ensuring validity
func (s *PrivateTxBundleAPI) SendBundle(ctx context.Context, args SendBundleArgs) error {
	var txs types.Transactions
	if len(args.Txs) == 0 {
		return errors.New("bundle missing txs")
	}
	if args.BlockNumber == 0 {
		return errors.New("bundle missing blockNumber")
	}

	for _, encodedTx := range args.Txs {
		tx := new(types.Transaction)
		if err := tx.UnmarshalBinary(encodedTx); err != nil {
			return err
		}
		txs = append(txs, tx)
	}

	var replacementUuid uuid.UUID
	if args.ReplacementUuid != nil {
		replacementUuid = *args.ReplacementUuid
	}

	var signingAddress common.Address
	if args.SigningAddress != nil {
		signingAddress = *args.SigningAddress
	}

	var minTimestamp, maxTimestamp uint64
	if args.MinTimestamp != nil {
		minTimestamp = *args.MinTimestamp
	}
	if args.MaxTimestamp != nil {
		maxTimestamp = *args.MaxTimestamp
	}

	go s.b.SendBundle(ctx, txs, args.BlockNumber, replacementUuid, signingAddress, minTimestamp, maxTimestamp, args.RevertingTxHashes)

	return nil
}

// BundleAPI offers an API for accepting bundled transactions
type BundleAPI struct {
	b     Backend
	chain *core.BlockChain
}

// NewBundleAPI creates a new Tx Bundle API instance.
func NewBundleAPI(b Backend, chain *core.BlockChain) *BundleAPI {
	return &BundleAPI{b, chain}
}

// CallBundleArgs represents the arguments for a call.
type CallBundleArgs struct {
	Txs                    []hexutil.Bytes       `json:"txs"`
	BlockNumber            rpc.BlockNumber       `json:"blockNumber"`
	StateBlockNumberOrHash rpc.BlockNumberOrHash `json:"stateBlockNumber"`
	Coinbase               *string               `json:"coinbase"`
	Timestamp              *uint64               `json:"timestamp"`
	Timeout                *int64                `json:"timeout"`
	GasLimit               *uint64               `json:"gasLimit"`
	Difficulty             *big.Int              `json:"difficulty"`
	BaseFee                *big.Int              `json:"baseFee"`
}

// CallBundle will simulate a bundle of transactions at the top of a given block
// number with the state of another (or the same) block. This can be used to
// simulate future blocks with the current state, or it can be used to simulate
// a past block.
// The sender is responsible for signing the transactions and using the correct
// nonce and ensuring validity
func (s *BundleAPI) CallBundle(ctx context.Context, args CallBundleArgs) (map[string]interface{}, error) {
	return nil, nil
}

// EstimateGasBundleArgs represents the arguments for a call
type EstimateGasBundleArgs struct {
	Txs                    []TransactionArgs     `json:"txs"`
	BlockNumber            rpc.BlockNumber       `json:"blockNumber"`
	StateBlockNumberOrHash rpc.BlockNumberOrHash `json:"stateBlockNumber"`
	Coinbase               *string               `json:"coinbase"`
	Timestamp              *uint64               `json:"timestamp"`
	Timeout                *int64                `json:"timeout"`
}

func (s *BundleAPI) EstimateGasBundle(ctx context.Context, args EstimateGasBundleArgs) (map[string]interface{}, error) {
	return nil, nil
}
