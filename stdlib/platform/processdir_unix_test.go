//go:build darwin || linux

package platform

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

// TestProcessDir_FailedAttemptRemoved: a create that fails for a reason other
// than a concurrent reap (here EMFILE: no descriptor is left) returns that
// error and leaves no directory behind.
func TestProcessDir_FailedAttemptRemoved(t *testing.T) {
	root := t.TempDir()

	limit := syscall.Rlimit{}
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	lowered := limit
	lowered.Cur = min(limit.Cur, 256)
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lowered); err != nil {
		t.Fatal(err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit)

	fillers := []*os.File{}
	closeFillers := func() {
		for _, filler := range fillers {
			filler.Close()
		}
		fillers = nil
	}
	defer closeFillers()
	for {
		filler, err := os.Open(os.DevNull)
		if errors.Is(err, syscall.EMFILE) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		fillers = append(fillers, filler)
	}
	dir, createErr := CreateProcessDir(root, "amp-test-")
	closeFillers()

	if !errors.Is(createErr, syscall.EMFILE) {
		t.Errorf("CreateProcessDir = %q, %v; want the EMFILE that stopped it", dir, createErr)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a failed create left %d directories under the root", len(entries))
	}
}
