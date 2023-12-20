package ethapi

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
)

// IssueAPI offers an API for accepting bundled transactions
type IssueAPI struct {
	b Backend
}

// NewIssueAPI creates a new Tx Bundle API instance.
func NewIssueAPI(b Backend) *IssueAPI {
	return &IssueAPI{b}
}

// SendIssueArgs represents the arguments for a call.
type SendIssueArgs struct {
	BidHash common.Hash `json:"bidHash"`
	Message string      `json:"message"`
}

func (s *IssueAPI) SendIssue(ctx context.Context, args SendIssueArgs) error {
	// TODO
	return nil
}
