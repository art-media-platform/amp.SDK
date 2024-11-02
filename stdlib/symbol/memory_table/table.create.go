package memory_table

import "github.com/art-media-platform/amp.SDK/stdlib/symbol"

// CreateTable creates a new memory-based symbol.Table intended to handle extreme loading.
//
// Value allocations are pooled, so TableOpts.PoolSz of 16k means thousands of small value entries could be stored within a single allocation.
func (opts TableOpts) CreateTable() (symbol.Table, error) {
	return createTable(opts)
}

type TableOpts struct {
	symbol.Issuer             // How this table will issue new IDs.  If nil, this table's db will be used as the Issuer
	IssuerInitsAt   symbol.ID // The floor ID to start issuing from if initializing a new Issuer.
	WorkingSizeHint int       // anticipated number of entries in working set
	PoolSz          int32     // Value backing buffer allocation pool sz
}

// DefaultOpts is a suggested set of options.
func DefaultOpts() TableOpts {
	return TableOpts{
		IssuerInitsAt:   symbol.DefaultIssuerMin,
		WorkingSizeHint: 600,
		PoolSz:          16 * 1024,
	}
}
