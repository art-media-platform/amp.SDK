package memory_table_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/symbol"
	"github.com/art-media-platform/amp.SDK/stdlib/symbol/memory_table"
	"github.com/art-media-platform/amp.SDK/stdlib/symbol/tests"
)

var gMemTable symbol.Table

func Test_memory_table(t *testing.T) {
	open_table := func() (symbol.Table, error) {
		if gMemTable == nil {
			opts := memory_table.DefaultOpts()
			gMemTable, _ = opts.CreateTable()
			gMemTable.AddRef() // add ref to get past first close in DoTableTest
		}
		return gMemTable, nil
	}

	tests.DoTableTest(t, 0, open_table)
}
