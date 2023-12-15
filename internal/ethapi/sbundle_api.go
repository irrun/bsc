package ethapi

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

const maxDepth = 5
const maxBodySize = 50
const defaultSimTimeout = time.Second * 5
const maxSimTimeout = time.Second * 30

var (
	ErrMaxDepth         = errors.New("max depth reached")
	ErrUnmatchedBundle  = errors.New("unmatched bundle")
	ErrBundleTooLarge   = errors.New("bundle too large")
	ErrInvalidValidity  = errors.New("invalid validity")
	ErrInvalidInclusion = errors.New("invalid inclusion")
)

type MevAPI struct {
	b     Backend
	chain *core.BlockChain
}

func NewMevAPI(b Backend, chain *core.BlockChain) *MevAPI {
	return &MevAPI{b, chain}
}

type SendMevBundleArgs struct {
	Version   string               `json:"version"`
	Inclusion MevBundleInclusion   `json:"inclusion"`
	Body      []MevBundleBody      `json:"body"`
	Validity  types.BundleValidity `json:"validity"`
}

type MevBundleInclusion struct {
	BlockNumber hexutil.Uint64 `json:"block"`
	MaxBlock    hexutil.Uint64 `json:"maxBlock,omitempty"`
}

type MevBundleBody struct {
	Hash      *common.Hash       `json:"hash,omitempty"`
	Tx        *hexutil.Bytes     `json:"tx,omitempty"`
	Bundle    *SendMevBundleArgs `json:"bundle,omitempty"`
	CanRevert bool               `json:"canRevert,omitempty"`
}

func ParseSBundleArgs(args *SendMevBundleArgs) (bundle types.SBundle, err error) {
	return parseBundleInner(0, args)
}

func parseBundleInner(level int, args *SendMevBundleArgs) (bundle types.SBundle, err error) {
	return bundle, nil
}

func (api *MevAPI) SendBundle(ctx context.Context, args SendMevBundleArgs) error {
	bundle, err := parseBundleInner(0, &args)
	if err != nil {
		return err
	}
	go api.b.SendSBundle(ctx, &bundle)
	return nil
}

type SimMevBundleResponse struct {
	Success         bool                     `json:"success"`
	Error           string                   `json:"error,omitempty"`
	StateBlock      hexutil.Uint64           `json:"stateBlock"`
	MevGasPrice     hexutil.Big              `json:"mevGasPrice"`
	Profit          hexutil.Big              `json:"profit"`
	RefundableValue hexutil.Big              `json:"refundableValue"`
	GasUsed         hexutil.Uint64           `json:"gasUsed"`
	BodyLogs        []core.SimBundleBodyLogs `json:"logs,omitempty"`
}

type SimMevBundleAuxArgs struct {
	ParentBlock *rpc.BlockNumberOrHash `json:"parentBlock"`
	// override the default values for the block header
	BlockNumber *hexutil.Big    `json:"blockNumber"`
	Coinbase    *common.Address `json:"coinbase"`
	Timestamp   *hexutil.Uint64 `json:"timestamp"`
	GasLimit    *hexutil.Uint64 `json:"gasLimit"`
	BaseFee     *hexutil.Big    `json:"baseFee"`
	Timeout     *int64          `json:"timeout"`
}

func (api *MevAPI) SimBundle(ctx context.Context, args SendMevBundleArgs, aux SimMevBundleAuxArgs) (*SimMevBundleResponse, error) {

	return nil, nil
}

func (api *MevAPI) CancelBundleByHash(ctx context.Context, hash common.Hash) error {
	go api.b.CancelSBundles(ctx, []common.Hash{hash})
	return nil
}
