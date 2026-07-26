package alog

// logger_flags_test.go pins InitFlags' re-entry contract: an embedding host can
// build many Environments in one process (each with its own fresh FlagSet —
// e.g. an embedded-lib Startup beside an in-process host, or a Shutdown→Startup
// cycle), so flag registration must neither race a live logger's emit nor
// clobber a configured -log_file path.

import (
	"flag"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestInitFlagsConcurrentWithEmit registers alog's flags on fresh FlagSets while
// another goroutine's logger emits.  Registration must not write any package
// global, or the emit path's read races it.  Meaningful under -race.
func TestInitFlagsConcurrentWithEmit(t *testing.T) {
	SetColor(false)
	_ = captureStderr(t, func() {
		probe := NewLogger("race-probe")
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for emitCount := 0; ; emitCount++ {
				select {
				case <-stop:
					return
				default:
					probe.Infof(0, "emit %d during re-registration", emitCount)
				}
			}
		}()
		for envCount := 0; envCount < 200; envCount++ {
			fs := flag.NewFlagSet(fmt.Sprintf("env-%d", envCount), flag.ContinueOnError)
			InitFlags(fs)
		}
		close(stop)
		wg.Wait()
	})
}

// TestInitFlagsKeepsConfiguredLogFile: a later InitFlags (a second Environment
// in the same process) must not reset a previously configured -log_file — that
// would silently turn off an active file tee.
func TestInitFlagsKeepsConfiguredLogFile(t *testing.T) {
	fsOne := flag.NewFlagSet("env-one", flag.ContinueOnError)
	InitFlags(fsOne)
	logPath := filepath.Join(t.TempDir(), "probe.log")
	if err := fsOne.Set("log_file", logPath); err != nil {
		t.Fatal(err)
	}
	defer fsOne.Set("log_file", "") // restore process-global state for other tests

	fsTwo := flag.NewFlagSet("env-two", flag.ContinueOnError)
	InitFlags(fsTwo)
	if got := fsTwo.Lookup("log_file").Value.String(); got != logPath {
		t.Fatalf("second InitFlags clobbered the configured log file: %q (want %q)", got, logPath)
	}
}
