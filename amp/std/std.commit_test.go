package std

// std.commit_test.go — the load pipeline's error channel must never park its
// sender (F-SDK-SIGNAL-CONTRACT, defect 2).  BlockingLoad and LoadItems both
// leave their select via appCtx.Closing(), so a PushTx reporting the context
// error can find no receiver at all; the send must complete or drop, never hold
// a pipeline goroutine for the process lifetime.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
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

// rejectingSession refuses every submit and keeps the context it was handed.
type rejectingSession struct {
	amp.Session
	submitted context.Context
	cause     error
}

func (sess *rejectingSession) SubmitTx(commit amp.TxCommit) error {
	sess.submitted = commit.Context
	return sess.cause
}

// rejectingApp lives as long as bound; its session refuses every submit.
type rejectingApp struct {
	amp.AppContext
	bound   context.Context
	session amp.Session
}

func (app *rejectingApp) Deadline() (time.Time, bool)        { return app.bound.Deadline() }
func (app *rejectingApp) Done() <-chan struct{}              { return app.bound.Done() }
func (app *rejectingApp) Err() error                         { return app.bound.Err() }
func (app *rejectingApp) Session() amp.Session               { return app.session }
func (app *rejectingApp) AppEnvironment() amp.AppEnvironment { return amp.AppEnvironment{} }
func (app *rejectingApp) NewTx(...amp.TxScope) *amp.TxMsg    { return &amp.TxMsg{} }

// A refused submit closes the wrapper BlockingLoad made for it, with the
// refusal; left open, its watcher goroutine lives until the app closes.
func TestLoadRefusedSubmitClosesWrapper(t *testing.T) {
	bound, cancel := context.WithCancel(context.Background())
	defer cancel()
	cause := errors.New("submit refused")
	sess := &rejectingSession{
		cause: cause,
	}
	app := &rejectingApp{
		bound:   bound,
		session: sess,
	}
	loadErr := BlockingLoad(app, tag.UID{1, 2}, &amp.Tags{})
	if sess.submitted == nil || !errors.Is(sess.submitted.Err(), cause) {
		t.Fatalf("BlockingLoad returned %v and left its wrapper open or closed without the refusal", loadErr)
	}
}

// closingApp has begun to close: Closing() has fired and Err reports it, while
// Done() stays open, as it does for a goroutine the app itself owns.
type closingApp struct {
	rejectingApp
	closing chan struct{}
}

func (app *closingApp) Closing() <-chan struct{} { return app.closing }
func (app *closingApp) Err() error               { return context.Canceled }

// A load whose app begins closing before the session answers reports the
// close; a nil return would read as a loaded dst.
func TestLoadClosingAppReportsCancel(t *testing.T) {
	bound, cancel := context.WithCancel(context.Background())
	defer cancel()
	closing := make(chan struct{})
	close(closing)
	sess := &rejectingSession{} // nil cause: the submit is accepted, never answered
	app := &closingApp{
		rejectingApp: rejectingApp{
			bound:   bound,
			session: sess,
		},
		closing: closing,
	}
	loadErr := BlockingLoad(app, tag.UID{1, 2}, &amp.Tags{})
	if !errors.Is(loadErr, context.Canceled) {
		t.Fatalf("BlockingLoad on a closing app returned %v, want context.Canceled", loadErr)
	}
}
