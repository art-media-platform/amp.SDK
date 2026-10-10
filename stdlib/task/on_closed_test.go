package task_test

// on_closed_test.go pins the Task.OnClosed order: an owned child's OnClosed
// returns and its Done() fires before its parent finalizes, whether the parent
// closes or idle-closes and whether the child is IdleExempt (ZO §1.2).

import (
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// TestOnClosed_OwnedChildPrecedesParentDone: a child blocked in OnClosed holds
// its parent's Done(), and the parent's OnClosed sees the child's Done().
func TestOnClosed_OwnedChildPrecedesParentDone(t *testing.T) {
	cases := []struct {
		name       string
		idleExempt bool
		autoClose  bool
	}{
		{name: "owned"},
		{
			name:       "idle-exempt",
			idleExempt: true,
		},
		{
			name:      "idle-owned",
			autoClose: true,
		},
		{
			name:       "idle-exempt-auto",
			idleExempt: true,
			autoClose:  true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			parentSawDone := make(chan bool, 1)
			childDone := (<-chan struct{})(nil)
			parent, err := task.Start(task.Task{
				Info: task.Info{Label: "parent"},
				OnClosed: func() {
					select {
					case <-childDone:
						parentSawDone <- true
					default:
						parentSawDone <- false
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			child, err := parent.StartChild(task.Task{
				Info: task.Info{
					Label:      "child",
					IdleExempt: testCase.idleExempt,
				},
				OnClosed: func() {
					close(entered)
					<-release
				},
			})
			if err != nil {
				parent.Close()
				t.Fatal(err)
			}
			childDone = child.Done()
			t.Cleanup(func() {
				close(release)
				parent.Close()
				select {
				case <-parent.Done():
				case <-time.After(2 * time.Second):
					t.Error("parent cleanup did not drain")
				}
				select {
				case <-child.Done():
				case <-time.After(2 * time.Second):
					t.Error("child cleanup did not drain")
				}
				select {
				case completed := <-parentSawDone:
					if !completed {
						t.Error("parent OnClosed preceded child Done")
					}
				default:
					t.Error("parent OnClosed did not run")
				}
			})
			if testCase.autoClose {
				parent.CloseWhenIdle(time.Nanosecond)
				if !testCase.idleExempt {
					child.Close()
				}
			} else {
				parent.Close()
			}
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("child OnClosed did not run")
			}
			select {
			case <-parent.Done():
				t.Error("parent finalized while child OnClosed was blocked")
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}
