package memorytable_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/symbol"
	"github.com/art-media-platform/amp.SDK/stdlib/symbol/memorytable"
	"github.com/art-media-platform/amp.SDK/stdlib/symbol/tests"
)

func TestMemoryTable(t *testing.T) {
	// A fresh table per test run (DoTableTest closes it at the end, so a shared
	// instance could not be reused -- e.g. under -count > 1).
	var memTable symbol.Table
	openTable := func() (symbol.Table, error) {
		if memTable == nil {
			opts := memorytable.DefaultOpts()
			memTable, _ = opts.CreateTable()
			memTable.PushOpen() // add a ref so the first Close() in DoTableTest is a no-op
		}
		return memTable, nil
	}

	tests.DoTableTest(t, 0, openTable)
}
