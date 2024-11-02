package task_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
		p, _ := task.Start(&task.Task{
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
		p, _ := task.Start(&task.Task{
			Info: task.Info{
				Label:     "root",
				IdleClose: time.Nanosecond,
			},
		})

		child, _ := p.StartChild(&task.Task{
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
		p, _ := task.Start(&task.Task{
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
		p, _ := task.Start(&task.Task{
			Info: task.Info{
				Label: "close tester",
			},
		})

		child, _ := p.StartChild(&task.Task{
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

		require.Eventually(t, func() bool { return isDone(t, p.Done()) }, 5*time.Second, 100*time.Millisecond)
		require.Eventually(t, func() bool { return isDone(t, child.Done()) }, 5*time.Second, 100*time.Millisecond)
		require.Eventually(t, func() bool { return isDone(t, foo1.Done()) }, 5*time.Second, 100*time.Millisecond)
		require.Eventually(t, func() bool { return isDone(t, foo2.Done()) }, 5*time.Second, 100*time.Millisecond)
	})
}

func requireDone(t *testing.T, chDone <-chan struct{}, done bool) {
	t.Helper()
	require.Equal(t, done, isDone(t, chDone))
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
