package memory_table_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/symbol"
	"github.com/art-media-platform/amp.SDK/stdlib/symbol/memory_table"
	"github.com/art-media-platform/amp.SDK/stdlib/symbol/tests"
)

func Test_memory_table(t *testing.T) {
	// A fresh table per test run (DoTableTest closes it at the end, so a shared
	// instance could not be reused -- e.g. under -count > 1).
	var memTable symbol.Table
	open_table := func() (symbol.Table, error) {
		if memTable == nil {
			opts := memory_table.DefaultOpts()
			memTable, _ = opts.CreateTable()
			memTable.PushOpen() // add a ref so the first Close() in DoTableTest is a no-op
		}
		return memTable, nil
	}

	tests.DoTableTest(t, 0, open_table)
}
