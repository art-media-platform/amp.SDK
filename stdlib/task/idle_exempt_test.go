package task_test

// idle_exempt_test.go pins the Info.IdleExempt contract: an idle-exempt child
// never keeps an idle-closing parent alive, yet the parent drains it before
// finalizing (unlike Detach) — so a parent hook never races the child's.
// ClosingContext's Done() = Closing() is pinned beside it.

import (
	"context"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// TestIdleExempt_ChildDoesNotKeepParentAlive: a relay that leaves only on
// Closing() lets its idle-close parent close, is drained before the parent's
// Done(), and is never counted abandoned.
func TestIdleExempt_ChildDoesNotKeepParentAlive(t *testing.T) {
	var relay task.Context
	started := make(chan struct{})
	parent, err := task.Start(task.Task{
		Info: task.Info{
			Label:     "parent",
			IdleClose: time.Nanosecond,
		},
		OnRun: func(ctx task.Context) {
			child, err := ctx.StartChild(task.Task{
				Info: task.Info{
					Label:      "relay",
					IdleExempt: true,
					IdleClose:  time.Nanosecond,
				},
				OnRun: func(ctx task.Context) { <-ctx.Closing() },
			})
			if err != nil {
				t.Errorf("StartChild: %v", err)
			}
			relay = child
			close(started)
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	select {
	case <-parent.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("idle-close parent was held open by an idle-exempt child")
	}
	// Drained, not abandoned: the child finalized before (or with) the parent.
	requireDone(t, relay.Done(), true)
	if got := parent.AbandonedChildren(); got != 0 {
		t.Fatalf("AbandonedChildren = %d, want 0 (an idle-exempt child is drained, never abandoned)", got)
	}
}

// TestIdleExempt_OwnedChildStillHoldsIdle is the control: the same relay as
// an ordinary owned child holds the parent open.
func TestIdleExempt_OwnedChildStillHoldsIdle(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)
	parent, err := task.Start(task.Task{
		Info: task.Info{
			Label:     "parent",
			IdleClose: time.Nanosecond,
		},
		OnRun: func(ctx task.Context) {
			if _, err := task.Go(ctx, "owned", func(ctx task.Context) { <-gate }); err != nil {
				t.Errorf("Go: %v", err)
			}
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-parent.Done():
		t.Fatal("an owned child must hold its parent's idle-close")
	case <-time.After(300 * time.Millisecond):
	}
	parent.Close()
}

// TestClosingContext_DoneIsClosing: Done() fires at Closing(), before the
// body returns, and Err() reads context.Canceled — the shape a body hands to a
// context.Context-taking wait.
func TestClosingContext_DoneIsClosing(t *testing.T) {
	bodyDone := make(chan struct{})
	boundCh := make(chan context.Context, 1)
	child, err := task.Start(task.Task{
		Info: task.Info{Label: "body"},
		OnRun: func(ctx task.Context) {
			boundCh <- task.ClosingContext(ctx)
			<-bodyDone // the body has not returned: ctx.Done() cannot fire
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var bound context.Context
	select {
	case bound = <-boundCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("body did not hand over its ClosingContext")
	}
	if bound.Err() != nil {
		t.Fatalf("Err() before close = %v, want nil", bound.Err())
	}
	child.Close()
	select {
	case <-bound.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("ClosingContext.Done() did not fire at Closing()")
	}
	requireDone(t, child.Done(), false) // the body still runs: the Context's own Done() waits
	if bound.Err() != context.Canceled {
		t.Fatalf("Err() after close = %v, want context.Canceled", bound.Err())
	}
	close(bodyDone)
	requireDoneWithin(t, child.Done(), 2*time.Second)
}

func requireDoneWithin(t *testing.T, done <-chan struct{}, within time.Duration) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(within):
		t.Fatal("Done() did not fire within the bound")
	}
}
