package std

// std.commit_test.go — the load pipeline's error channel must never park its
// sender (F-SDK-SIGNAL-CONTRACT, defect 2).  BlockingLoad and LoadItems both
// leave their select via appCtx.Closing(), so a PushTx reporting the context
// error can find no receiver at all; the send must complete or drop, never hold
// a pipeline goroutine for the process lifetime.

import (
	"context"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
)

// pushBound is the fixture's admissible ceiling for a PushTx that has no
// receiver: the send is meant to be non-blocking, so any real wait is the park.
const pushBound = 2 * time.Second

func TestLoadPushTxNeverParksWithoutReceiver(t *testing.T) {
	req := newLocalLoad()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the receiver has left via appCtx.Closing(); nothing reads outErr

	// Push twice: the first send must not park on an unbuffered channel, the
	// second must not park once the buffer holds the first error.
	for push := 1; push <= 2; push++ {
		pushed := make(chan error, 1)
		go func() { pushed <- req.PushTx(&amp.TxMsg{}, ctx) }()

		select {
		case err := <-pushed:
			if err == nil {
				t.Fatalf("push %d: want the context error, got nil", push)
			}
		case <-time.After(pushBound):
			t.Fatalf("push %d parked over %v with no receiver on outErr — pipeline goroutine leak", push, pushBound)
		}
	}
}
