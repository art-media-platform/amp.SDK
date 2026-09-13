// Package tests holds the symbol.Table conformance and stress harness shared
// by Table implementations.
package tests

import (
	"bytes"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/symbol"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

const kTotalEntries = 1001447

// DoTableTest fills a table under concurrent writers, reopens it, and checks
// every entry under concurrent readers.  The run is one task tree: a root per
// call, the writers and readers its Go children; the root's Done() is the
// join.  The first failure is recorded once and stops every worker (fail);
// no worker ever sends on a channel nobody drains.
func DoTableTest(t *testing.T, totalEntries int, opener func() (symbol.Table, error)) {
	if totalEntries == 0 {
		totalEntries = kTotalEntries
	}

	tt := tableTester{
		stop: make(chan struct{}),
	}
	tt.setupTestData(totalEntries)

	root, err := task.Start(task.Task{
		Info: task.Info{
			Label:     "symbol-table-test",
			IdleClose: 1,
		},
		OnRun: func(ctx task.Context) {
			// 1) fill and write a table
			table, err := opener()
			if err != nil {
				tt.fail(err)
				return
			}
			tt.fillTable(ctx, table)
			table.Close()
			if tt.failed() {
				return
			}

			// 2) read and check the table
			table, err = opener()
			if err != nil {
				tt.fail(err)
				return
			}
			tt.checkTable(ctx, table)
			table.Close()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	<-root.Done()
	if err := tt.firstErr.Load(); err != nil {
		t.Fatal(*err)
	}
}

type tableTester struct {
	firstErr atomic.Pointer[error]
	stopOnce sync.Once
	stop     chan struct{} // closed on the first failure; every worker loop leaves on it
	vals     [][]byte
	IDs      []symbol.ID
}

// fail records the first error and releases every worker.
func (tt *tableTester) fail(err error) {
	if err == nil {
		return
	}
	tt.firstErr.CompareAndSwap(nil, &err)
	tt.stopOnce.Do(func() { close(tt.stop) })
}

func (tt *tableTester) failed() bool {
	select {
	case <-tt.stop:
		return true
	default:
		return false
	}
}

func (tt *tableTester) setupTestData(totalEntries int) {
	if len(tt.vals) != totalEntries {
		tt.vals = make([][]byte, totalEntries)
		for i := range tt.vals {
			tt.vals[i] = []byte(strconv.Itoa(i))
		}
	}
	if cap(tt.IDs) < totalEntries {
		tt.IDs = make([]symbol.ID, totalEntries)
	} else {
		tt.IDs = tt.IDs[:totalEntries]
		for i := range tt.IDs {
			tt.IDs[i] = 0
		}
	}
}

var (
	hardwireStart     = symbol.DefaultIssuerMin - hardwireTestCount
	hardwireTestCount = 101
)

// runWorkers starts numWorkers Go children of parent, each running body over
// its own start offset, and joins them through a supervision child's Done().
func (tt *tableTester) runWorkers(parent task.Context, label string, numWorkers int, body func(startAt int)) {
	group, err := task.NewChild(parent, label)
	if err != nil {
		tt.fail(err)
		return
	}
	for i := 0; i < numWorkers; i++ {
		startAt := len(tt.vals) * i / numWorkers
		if _, err := task.Go(group, label+"-worker", func(task.Context) { body(startAt) }); err != nil {
			tt.fail(err)
		}
	}
	group.Close()
	<-group.Done()
}

func (tt *tableTester) fillTable(ctx task.Context, table symbol.Table) {
	vals := tt.vals
	totalEntries := len(vals)

	// Test reserved symbol ID space -- set symbol IDs less than symbol.DefaultIssuerMin
	// Do multiple write passes to check overwrites don't cause issues.
	for k := 0; k < 3; k++ {
		for j := 0; j < hardwireTestCount; j++ {
			idx := hardwireStart + j
			symID := symbol.ID(idx)
			symIDGot, _ := table.SetSymbolID(vals[idx], symID)
			if symIDGot != symID {
				tt.fail(errors.New("SetSymbolID failed setup check"))
				return
			}
		}
	}

	hardwireCount := int32(0)
	const numWorkers = 5

	// Populate the table with multiple workers all setting values at once
	tt.runWorkers(ctx, "fill", numWorkers, func(startAt int) {
		var symBuf [128]byte
		for j := 0; j < totalEntries && !tt.failed(); j++ {
			idx := (startAt + j) % totalEntries
			symID, _ := table.GetSymbolID(vals[idx], true)
			if symID < symbol.DefaultIssuerMin {
				atomic.AddInt32(&hardwireCount, 1)
			}
			stored := table.GetSymbol(symID, symBuf[:0])
			if !bytes.Equal(stored, vals[idx]) {
				tt.fail(errors.New("LookupID failed setup check"))
				return
			}
			symIDGot, _ := table.SetSymbolID(vals[idx], symID)
			if symIDGot != symID {
				tt.fail(errors.New("SetSymbolID failed setup check"))
				return
			}
		}
	})
	if tt.failed() {
		return
	}
	if int(atomic.LoadInt32(&hardwireCount)) != numWorkers*hardwireTestCount {
		tt.fail(errors.New("hardwire test count failed"))
		return
	}

	var symBuf [128]byte

	// Verify all the tokens are valid
	IDs := tt.IDs
	for i, k := range vals {
		IDs[i], _ = table.GetSymbolID(k, false)
		if IDs[i] == 0 {
			tt.fail(errors.New("GetSymbolID failed final verification"))
			return
		}
		stored := table.GetSymbol(IDs[i], symBuf[:0])
		if !bytes.Equal(stored, vals[i]) {
			tt.fail(errors.New("LookupID failed final verification"))
			return
		}
	}

	if table.GetSymbol(123456789, nil) != nil {
		tt.fail(errors.New("bad ID returns value"))
	}
	if ID, _ := table.GetSymbolID([]byte{4, 5, 6, 7, 8, 9, 10, 11}, false); ID != 0 {
		tt.fail(errors.New("bad value returns ID"))
	}
}

func (tt *tableTester) checkTable(ctx task.Context, table symbol.Table) {
	vals := tt.vals
	totalEntries := len(vals)
	IDs := tt.IDs
	const numWorkers = 5

	// Check that all the tokens are present
	tt.runWorkers(ctx, "check", numWorkers, func(startAt int) {
		var symBuf [128]byte
		for j := 0; j < totalEntries && !tt.failed(); j++ {
			idx := (startAt + j) % totalEntries
			if (j % numWorkers) == 0 {
				symID, _ := table.GetSymbolID(vals[idx], false)
				if symID != IDs[idx] {
					tt.fail(errors.New("GetSymbolID failed readback check"))
					return
				}
			} else {
				stored := table.GetSymbol(IDs[idx], symBuf[:0])
				if !bytes.Equal(stored, vals[idx]) {
					tt.fail(errors.New("LookupID failed readback check"))
					return
				}
			}
		}
	})
}
