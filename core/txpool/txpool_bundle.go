package txpool

import (
	"errors"
	"github.com/ethereum/go-ethereum/core/types"
)

// AddBundle adds a mev bundle to the bundle pool
func (p *TxPool) AddBundle(bundle *types.Bundle) error {
	// Try to find a subpool that accepts the
	for _, subpool := range p.subpools {
		if subpool.FilterBundle() {
			return subpool.AddBundle(bundle)
		}
	}

	return errors.New("no subpool accepts the bundle")
}
