package tests

import (
	"bytes"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/symbol"
)

const kTotalEntries = 1001447

func DoTableTest(t *testing.T, totalEntries int, opener func() (symbol.Table, error)) {
	if totalEntries == 0 {
		totalEntries = kTotalEntries
	}

	tt := tableTester{
		errs: make(chan error),
	}
	tt.setupTestData(totalEntries)

	go func() {

		// 1) tortuously fill and write a table
		table, err := opener()
		if err != nil {
			tt.errs <- err
		}
		tt.fillTable(table)
		table.Close()

		// 2) tortuously read and check the table
		table, err = opener()
		if err != nil {
			tt.errs <- err
		}
		tt.checkTable(table)
		table.Close()

		close(tt.errs)
	}()

	// wait for test to finish, fail the test on any errors
	err := <-tt.errs
	if err != nil {
		t.Fatal(err)
	}
}

type tableTester struct {
	errs chan error
	vals [][]byte
	IDs  []symbol.ID
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

func (tt *tableTester) fillTable(table symbol.Table) {
	vals := tt.vals
	totalEntries := len(vals)

	// Test reserved symbol ID space -- set symbol IDs less than symbol.MinIssuedID
	// Do multiple write passes to check overwrites don't cause issues.
	for k := 0; k < 3; k++ {
		for j := 0; j < hardwireTestCount; j++ {
			idx := hardwireStart + j
			symID := symbol.ID(idx)
			symID_got, _ := table.SetSymbolID(vals[idx], symID)
			if symID_got != symID {
				tt.errs <- errors.New("SetSymbolID failed setup check")
			}
		}
	}

	hardwireCount := int32(0)
	hardwireCountPtr := &hardwireCount

	// Populate the table with multiple workers all setting values at once
	{
		running := &sync.WaitGroup{}
		numWorkers := 5
		for i := 0; i < numWorkers; i++ {
			running.Add(1)
			startAt := len(vals) * i / numWorkers
			go func() {
				var symBuf [128]byte
				for j := 0; j < totalEntries; j++ {
					idx := (startAt + j) % totalEntries
					symID, _ := table.GetSymbolID(vals[idx], true)
					if symID < symbol.DefaultIssuerMin {
						atomic.AddInt32(hardwireCountPtr, 1)
					}
					stored := table.GetSymbol(symID, symBuf[:0])
					if !bytes.Equal(stored, vals[idx]) {
						tt.errs <- errors.New("LookupID failed setup check")
					}
					symID_got, _ := table.SetSymbolID(vals[idx], symID)
					if symID_got != symID {
						tt.errs <- errors.New("SetSymbolID failed setup check")
					}
				}
				running.Done()
			}()
		}

		running.Wait()

		if int(hardwireCount) != numWorkers*hardwireTestCount {
			tt.errs <- errors.New("hardwire test count failed")
		}

	}

	var symBuf [128]byte

	// Verify all the tokens are valid
	IDs := tt.IDs
	for i, k := range vals {
		IDs[i], _ = table.GetSymbolID(k, false)
		if IDs[i] == 0 {
			tt.errs <- errors.New("GetSymbolID failed final verification")
		}
		stored := table.GetSymbol(IDs[i], symBuf[:0])
		if !bytes.Equal(stored, vals[i]) {
			tt.errs <- errors.New("LookupID failed final verification")
		}
	}

	if table.GetSymbol(123456789, nil) != nil {
		tt.errs <- errors.New("bad ID returns value")
	}
	if ID, _ := table.GetSymbolID([]byte{4, 5, 6, 7, 8, 9, 10, 11}, false); ID != 0 {
		tt.errs <- errors.New("bad value returns ID")
	}
}

func (tt *tableTester) checkTable(table symbol.Table) {
	vals := tt.vals
	totalEntries := len(vals)

	// Check that all the tokens are present
	{
		IDs := tt.IDs
		running := &sync.WaitGroup{}
		numWorkers := 5
		for i := 0; i < numWorkers; i++ {
			running.Add(1)
			startAt := len(vals) * i / numWorkers
			go func() {
				var symBuf [128]byte
				for j := 0; j < totalEntries; j++ {
					idx := (startAt + j) % totalEntries

					if (j % numWorkers) == 0 {
						symID, _ := table.GetSymbolID(vals[idx], false)
						if symID != IDs[idx] {
							tt.errs <- errors.New("GetSymbolID failed readback check")
						}
					} else {
						stored := table.GetSymbol(IDs[idx], symBuf[:0])
						if !bytes.Equal(stored, vals[idx]) {
							tt.errs <- errors.New("LookupID failed readback check")
						}
					}
				}
				running.Done()
			}()
		}

		running.Wait()
	}
}
