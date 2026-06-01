package task_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

func spawnN(p task.Context, numGoroutines int, delay time.Duration) {
	for i := 0; i < numGoroutines; i++ {
		name := fmt.Sprintf("#%d", i+1)
		p.Go(name, func(ctx task.Context) {
			time.Sleep(delay)
			yoyo := delay
			fmt.Print(yoyo)
		})
	}
}

func TestCore(t *testing.T) {
	t.Run("basic idle close", func(t *testing.T) {
		p, _ := task.Start(task.Task{
			Info: task.Info{
				Label:     "root",
				IdleClose: time.Nanosecond,
			},
		})

		spawnN(p, 1, 1*time.Second)

		select {
		case <-time.After(2000 * time.Second):
			t.Fatal("fail")
		case <-p.Done():
		}
	})
}

func TestNestedIdleClose(t *testing.T) {
	t.Run("nested idle close", func(t *testing.T) {
		p, _ := task.Start(task.Task{
			Info: task.Info{
				Label:     "root",
				IdleClose: time.Nanosecond,
			},
		})

		child, _ := p.StartChild(task.Task{
			Info: task.Info{
				Label:     "child",
				IdleClose: time.Nanosecond,
			},
		})
		spawnN(child, 10, 1*time.Second)

		select {
		case <-time.After(2 * time.Second):
			t.Fatal("fail")
		case <-p.Done():
		}
	})
}

func TestIdleCloseWithDelay(t *testing.T) {
	t.Run("idle close with delay", func(t *testing.T) {
		p, _ := task.Start(task.Task{
			Info: task.Info{
				Label:     "root with idle close delay",
				IdleClose: 2 * time.Second,
			},
		})

		select {
		case <-time.After(3 * time.Second):
		case <-p.Done():
			t.Fatal("ctx exited early")
		default:
		}

		spawnN(p, 10, 1*time.Second)

		select {
		case <-time.After(4 * time.Second):
			t.Fatal("fail")
		case <-p.Done():
		}

	})
}

func Test6(t *testing.T) {

	t.Run("close cancels children", func(t *testing.T) {
		p, _ := task.Start(task.Task{
			Info: task.Info{
				Label: "close tester",
			},
		})

		child, _ := p.StartChild(task.Task{
			Info: task.Info{
				Label: "child",
			},
		})

		canceled1 := NewAwaiter()
		canceled2 := NewAwaiter()

		foo1, _ := p.Go("foo1", func(ctx task.Context) {
			select {
			case <-ctx.Closing():
				canceled1.ItHappened()
			case <-time.After(5 * time.Second):
				t.Fatal("context wasn't canceled")
			}
		})

		foo2, _ := child.Go("foo2", func(ctx task.Context) {
			select {
			case <-ctx.Closing():
				canceled2.ItHappened()
			case <-time.After(5 * time.Second):
				t.Fatal("context wasn't canceled")
			}

		})

		requireDone(t, p.Done(), false)
		requireDone(t, child.Done(), false)
		requireDone(t, foo1.Done(), false)
		requireDone(t, foo2.Done(), false)

		go p.Close()

		canceled1.AwaitOrFail(t)
		canceled2.AwaitOrFail(t)

		status.RequireEventually(t, func() bool { return isDone(t, p.Done()) }, 5*time.Second, 100*time.Millisecond)
		status.RequireEventually(t, func() bool { return isDone(t, child.Done()) }, 5*time.Second, 100*time.Millisecond)
		status.RequireEventually(t, func() bool { return isDone(t, foo1.Done()) }, 5*time.Second, 100*time.Millisecond)
		status.RequireEventually(t, func() bool { return isDone(t, foo2.Done()) }, 5*time.Second, 100*time.Millisecond)
	})
}

func requireDone(t *testing.T, chDone <-chan struct{}, done bool) {
	t.Helper()
	status.Require(t, isDone(t, chDone), done)
}

func isDone(t *testing.T, chDone <-chan struct{}) bool {
	t.Helper()
	select {
	case <-chDone:
		return true
	default:
		return false
	}
}

type Awaiter chan struct{}

func NewAwaiter() Awaiter { return make(Awaiter, 10) }

func (a Awaiter) ItHappened() { a <- struct{}{} }

func (a Awaiter) AwaitOrFail(t testing.TB, params ...interface{}) {
	t.Helper()

	duration := 10 * time.Second
	msg := ""
	for _, p := range params {
		switch p := p.(type) {
		case time.Duration:
			duration = p
		case string:
			msg = p
		}
	}

	select {
	case <-a:
	case <-time.After(duration):
		t.Fatalf("Timed out waiting for Awaiter to get ItHappened: %v", msg)
	}
}

func (a Awaiter) NeverHappenedOrFail(t testing.TB, params ...interface{}) {
	t.Helper()

	duration := 10 * time.Second
	msg := ""
	for _, p := range params {
		switch p := p.(type) {
		case time.Duration:
			duration = p
		case string:
			msg = p
		}
	}

	select {
	case <-a:
		t.Fatalf("should not happen: %v", msg)
	case <-time.After(duration):
	}
}

// TestConcurrentStartChildVsIdleClose hammers StartChild() against a parent that
// has a live idle-close goroutine. The two must coordinate purely through the
// Context's internal lock; run under -race to guard against regressions in the
// idle-close bookkeeping (active count, idle floor, close state).
func TestConcurrentStartChildVsIdleClose(t *testing.T) {
	// A long delay keeps the root alive under the hammer while its idle-close
	// goroutine repeatedly re-evaluates idleness as children come and go.
	root, _ := task.Start(task.Task{
		Info: task.Info{
			Label:     "root",
			IdleClose: 30 * time.Second,
		},
	})

	const writers = 16
	const iters = 2000

	var started atomic.Int64
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range iters {
				child, err := root.StartChild(task.Task{
					Info:  task.Info{Label: "c"},
					OnRun: func(ctx task.Context) {},
				})
				if err != nil {
					return // root closed unexpectedly
				}
				started.Add(1)
				child.Close()
			}
		})
	}

	wg.Wait()
	if got := started.Load(); got != writers*iters {
		t.Fatalf("root closed early: started %d of %d children", got, writers*iters)
	}

	root.Close()
	select {
	case <-root.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("root did not close")
	}
}

// TestPreventIdleClose verifies that PreventIdleClose defers an otherwise
// imminent idle-close until its floor elapses.
func TestPreventIdleClose(t *testing.T) {
	root, _ := task.Start(task.Task{
		Info: task.Info{Label: "root"},
	})

	// Hold the floor well past the idle delay, then arm a near-instant idle-close.
	if !root.PreventIdleClose(750 * time.Millisecond) {
		t.Fatal("PreventIdleClose returned false on a running Context")
	}
	root.CloseWhenIdle(time.Nanosecond)

	select {
	case <-root.Done():
		t.Fatal("idle-closed before the PreventIdleClose floor elapsed")
	case <-time.After(250 * time.Millisecond):
		// still open, as required
	}

	select {
	case <-root.Done():
		// closed after the floor, as required
	case <-time.After(2 * time.Second):
		t.Fatal("did not idle-close after the floor elapsed")
	}

	// After close, PreventIdleClose reports the Context is no longer running.
	if root.PreventIdleClose(time.Second) {
		t.Fatal("PreventIdleClose returned true on a closed Context")
	}
}
