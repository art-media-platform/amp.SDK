package safe

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestLocalTomeStore_ConcurrentSaveLoad guards the atomic-write contract: a
// reader — a second session opening the same member tome — must never observe a
// torn file.  A non-atomic truncate-then-write lets a concurrent Load unmarshal
// a tome with a nil WrappedDEK; the temp-file + rename publish makes every Load
// observe a complete tome.
func TestLocalTomeStore_ConcurrentSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "member.tome")
	store := NewLocalTomeStore(path)
	ctx := context.Background()

	// Large enough to span multiple write chunks, so a non-atomic write has a
	// wide window for a reader to catch a partial file.
	want := bytes.Repeat([]byte{0xAB}, 64*1024)
	full := &SealedTome{
		Version:    1,
		WrappedDEK: &WrappedDEK{Provider: "fileGuard", Cipherblob: want},
		Cipherblob: want,
	}
	if err := store.Save(ctx, full); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	const writers, readers, iters = 4, 8, 200
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if err := store.Save(ctx, full); err != nil {
					fail(err)
					return
				}
			}
		}()
	}
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				sealed, err := store.Load(ctx)
				if err != nil {
					fail(err)
					return
				}
				if sealed == nil {
					continue // tome always present after seeding; tolerate regardless
				}
				if sealed.WrappedDEK == nil || !bytes.Equal(sealed.Cipherblob, want) {
					fail(fmt.Errorf("torn read: WrappedDEK=%v cipherblob_len=%d", sealed.WrappedDEK, len(sealed.Cipherblob)))
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
}
