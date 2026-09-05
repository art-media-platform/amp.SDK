package task_test

// detach_test.go pins the task.Detach contract: a detached child is registered
// in the tree and closes with its parent, but never holds the parent's drain,
// and an abandoned one is counted exactly once. TestDetach_OwnedChildBlocksDrain
// pins the unchanged Go contract beside it.

import (
	"strings"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// TestDetach_ParkedChildDoesNotBlockParent: a detached child whose body cannot
// leave on Closing() is registered, is signalled at parent close, does not hold
// the parent's Done(), and is counted abandoned once.
func TestDetach_ParkedChildDoesNotBlockParent(t *testing.T) {
	root, _ := task.Start(task.Task{Info: task.Info{Label: "root"}})
	gate := make(chan struct{}) // the substrate park; released only by the test
	closing := NewAwaiter()

	parked, err := task.Detach(root, "parked", func(ctx task.Context) {
		<-ctx.Closing()
		closing.ItHappened()
		<-gate // the body cannot leave on Closing — the case Detach exists for
	})
	if err != nil {
		t.Fatalf("Detach: %v", err)
	}

	// Registered: present under the parent and marked in the tree render.
	if children := root.GetChildren(nil); len(children) != 1 || children[0] != parked {
		t.Fatalf("detached child not registered under root: %d children", len(children))
	}
	tree := strings.Builder{}
	task.PrintContextTree(root, &tree, 0)
	if !strings.Contains(tree.String(), "parked") || !strings.Contains(tree.String(), "[detached]") {
		t.Errorf("tree render lacks the detached node or its marker:\n%s", tree.String())
	}

	root.Close()

	// Signalled with the parent ...
	closing.AwaitOrFail(t, 2*time.Second, "detached child's Closing() did not fire at parent close")

	// ... yet the parent finalizes without it.
	select {
	case <-root.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("root.Done() blocked on a parked detached child")
	}
	if got := root.AbandonedChildren(); got != 1 {
		t.Fatalf("AbandonedChildren = %d, want 1", got)
	}
	requireDone(t, parked.Done(), false)

	// Releasing the park finalizes the child; the count does not move again.
	close(gate)
	select {
	case <-parked.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("detached child did not finalize once its body returned")
	}
	if got := root.AbandonedChildren(); got != 1 {
		t.Fatalf("AbandonedChildren = %d after the child finalized, want 1 (counted exactly once)", got)
	}
}

// TestDetach_OwnedChildBlocksDrain: the existing contract is unchanged — a Go
// child that has not returned holds the parent's Done(), and is never counted
// abandoned.
func TestDetach_OwnedChildBlocksDrain(t *testing.T) {
	root, _ := task.Start(task.Task{Info: task.Info{Label: "root"}})
	gate := make(chan struct{})

	if _, err := task.Go(root, "owned", func(ctx task.Context) {
		<-ctx.Closing()
		<-gate
	}); err != nil {
		t.Fatalf("Go: %v", err)
	}

	root.Close()
	select {
	case <-root.Done():
		t.Fatal("root finalized while an owned child was still running")
	case <-time.After(300 * time.Millisecond):
	}

	close(gate)
	select {
	case <-root.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("root did not finalize once its owned child returned")
	}
	if got := root.AbandonedChildren(); got != 0 {
		t.Fatalf("AbandonedChildren = %d, want 0", got)
	}
}

// TestDetach_CompletedChildNotAbandoned: a detached child that returned before
// the parent closed has left the tree and is not counted; Detach on a closed
// parent reports ErrNotRunning.
func TestDetach_CompletedChildNotAbandoned(t *testing.T) {
	root, _ := task.Start(task.Task{Info: task.Info{Label: "root"}})

	quick, err := task.Detach(root, "quick", func(ctx task.Context) {})
	if err != nil {
		t.Fatalf("Detach: %v", err)
	}
	select {
	case <-quick.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("detached child did not idle-close after its body returned")
	}
	if children := root.GetChildren(nil); len(children) != 0 {
		t.Fatalf("finished detached child still registered: %d children", len(children))
	}

	root.Close()
	select {
	case <-root.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("root did not finalize")
	}
	if got := root.AbandonedChildren(); got != 0 {
		t.Fatalf("AbandonedChildren = %d, want 0", got)
	}

	if _, err := task.Detach(root, "too-late", func(ctx task.Context) {}); err == nil {
		t.Fatal("Detach on a closed parent returned nil error")
	}
}

// TestDetach_DoesNotKeepIdleCloseParentAlive: a detached child is not parent
// work — an idle-closing parent whose body returned finalizes with the child
// still parked, and counts it.
func TestDetach_DoesNotKeepIdleCloseParentAlive(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)

	parent, err := task.Start(task.Task{
		Info: task.Info{
			Label:     "parent",
			IdleClose: time.Nanosecond,
		},
		OnRun: func(ctx task.Context) {
			if _, err := task.Detach(ctx, "parked", func(ctx task.Context) { <-gate }); err != nil {
				t.Errorf("Detach: %v", err)
			}
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-parent.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("idle-close parent was held open by a detached child")
	}
	if got := parent.AbandonedChildren(); got != 1 {
		t.Fatalf("AbandonedChildren = %d, want 1", got)
	}
}
