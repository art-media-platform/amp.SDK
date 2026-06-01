// Features task.Context, a wrapper for goroutines inspired by a conventional parent-child process model
package task

import (
	"context"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/alog"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// Starts a new Context with no parent Context.
func Start(task Task) (Context, error) {
	return Context((*ctx)(nil)).StartChild(task)
}

// Go starts fn as a new Context that runs to completion and then idle-closes --
// the Context equivalent of launching a goroutine.
//
// If parent is nil, the new Context has no parent (a root, as with Start).
func Go(parent Context, label string, fn func(ctx Context)) (Context, error) {
	if parent == nil {
		parent = Context((*ctx)(nil))
	}
	return parent.Go(label, fn)
}

type Info struct {
	TaskID     tag.UID  // universally unique instance ID -- assigned automatically when unset
	Headers    []string // cookies, auth, or task references
	Label      string   // logging and debugging label
	Attachment any      // optional user-defined value
	DebugMode  bool     // when set, a context logs more verbosely and can perform (or log) expensive diagnostics

	// If > 0, Context.CloseWhenIdle() is automatically called when the last remaining child is closed or when OnRun() completes, whichever occurs later.
	//
	// This does not take effect unless OnRun is given or a child is started.
	IdleClose time.Duration
}

// Task is a parameter block used to start a new Context and contains hooks for each stage of the Context's lifecycle.
type Task struct {
	Info

	OnStart        func(ctx Context) error // Blocking fn called in StartChild(). If it errors, the child is closed, StartChild() returns the err, and OnRun is never called.
	OnRun          func(ctx Context)       // Async work body run in its own goroutine. When it returns, idle-close is armed if Info.IdleClose > 0.
	OnClosing      func()                  // Called immediately after Close() is first called, while self & children are closing.
	OnChildClosing func(child Context)     // Called immediately after the child's OnClosing() is called.
	OnClosed       func()                  // Called after Close() and all children have completed Close() (but immediately before Done() is released).
}

// Context is an expanded form of a context.Context offering, featuring:
//   - integrated logging, removing guesswork of which Context logged what
//   - "child" Contexts such that Close() will cause a Context's children to close
//   - automatic idle-close of Contexts after a period of inactivity
//   - the OnClosing() hook, allowing cleanup to occur when a Context is closed but before its parent is closed.
//   - PrintTreePeriodically() which visualizes a Context's child tree and is helpful for debugging in large projects.
type Context interface {
	Log() alog.Logger

	// Includes functionality and behavior of a context.Context, with these
	// refinements: Err() returns context.Canceled as soon as Close() is called
	// (i.e. once Closing() fires), ahead of Done(); Value() always returns nil;
	// and there is no Deadline.
	context.Context

	// Returns a snapshot of this Context's Info.
	Info() Info

	// Creates a new child Context for the given Task.
	// If OnStart() returns an error, then child.Close() is immediately called and the error is returned.
	StartChild(task Task) (Context, error)

	// Convenience wrapper for StartChild() equivalent to:
	//
	//      parent.StartChild(Task{
	//          Info:  Info{Label: label, IdleClose: time.Nanosecond},
	//          OnRun: fn,
	//      })
	Go(label string, fn func(ctx Context)) (Context, error)

	// Atomically appends all child Contexts to the given slice and returns the new slice.
	// The total blocking time is minimal as only a slice is populated.
	GetChildren(in []Context) []Context

	// Atomically iterates over all child Contexts and calls the given function.
	// The total blocking time is proportional to the number of children and running time of the given function.
	ForEachChild(fn func(child Context))

	// Initiates task shutdown and causes all children's Close() to be called -- non-blocking.
	// Close can be called multiple times but calls after the first are in effect ignored.
	// Closing() fires and OnClosing() runs, while children are closed concurrently in breadth-first order.
	// Once all children and OnRun() have drained, OnClosed() runs and then Done() fires.
	Close() error

	// Schedules Close() to run once this Context has been idle for the given delay.
	// A Context is idle when OnRun() has completed and it has no children.
	// Subsequent calls update the delay, restarting the countdown; a delay <= 0 disables the pending idle-close.
	// A later PreventIdleClose() floor takes precedence over the delay.
	CloseWhenIdle(delay time.Duration)

	// Ensures that this Context will not automatically idle-close until the given delay has passed.
	// If previous PreventIdleClose calls were made, the more limiting (later) delay is retained.
	//
	// Returns false if this Context has already begun closing.
	PreventIdleClose(delay time.Duration) bool

	// Signals when Close() has been called -- ahead of Done().
	// OnClosing() then runs and children close concurrently; once they (and OnRun) drain, OnClosed() runs before Done() fires.
	Closing() <-chan struct{}

	// Signals when Close() has fully executed, no children remain, and OnClosed() has been completed.
	Done() <-chan struct{}
}
