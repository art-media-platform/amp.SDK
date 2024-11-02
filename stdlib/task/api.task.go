// Features task.Context, a wrapper for goroutines inspired by a conventional parent-child process model
package task

import (
	"context"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/log"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// Starts a new Context with no parent Context.
func Start(task *Task) (Context, error) {
	return Context((*ctx)(nil)).StartChild(task)
}

// Go is a convenience function that starts a new Context that runs the given function -- like starting a goroutine.
//
// If parent == null, then the new Context will have no parent.
func Go(parent Context, label string, fn func(ctx Context)) (Context, error) {
	return parent.StartChild(&Task{
		Info: Info{
			Label: label,
		},
		OnRun: fn,
	})
}

type Info struct {
	TID       int64    // globally unique atomically incremented instance ID -- assigned OnStart()
	TagID     tag.ID   // optional user-defined tag.ID
	Other     any      // optional user-defined value
	Headers   []string // cookies, auth, or task references
	Label     string   // logging and debugging label
	DebugMode bool     // when set, a context logs more verbosely and can perform (or log) expensive diagnostics

	// If > 0, Context.CloseWhenIdle() will automatically called when the last remaining child is closed or when OnRun() completes, whichever occurs later.
	//
	// This will not enter into effect unless OnRun is given or a child is started.
	IdleClose time.Duration
}

// Task is a parameter block used to start a new Context and contains hooks for each stage of the Context's lifecycle.
type Task struct {
	Info Info

	OnStart        func(ctx Context) error // Blocking fn called in StartChild(). If err, ctx.Close() is called and Go() returns the err and OnRun is never called.
	OnRun          func(ctx Context)       // Async work body. If non-nil, ctx.Close() will be automatically called after OnRun() completes
	OnClosing      func()                  // Called immediately after Close() is first called while self & children are still closing
	OnChildClosing func(child Context)     // Called immediately after the child's OnClosing() is called
	OnClosed       func()                  // Called after Close() and all children have completed Close() (but immediately before Done() is released)
}

// Context is an expanded form of a context.Context offering, featuring:
//   - integrated logging, removing guesswork of which Context logged what
//   - "child" Contexts such that Close() will cause a Context's children to close
//   - automatic idle-close of Contexts after a period of inactivity
//   - the OnClosing() hook, allowing cleanup to occur  when a Context is closed but before its parent is closed.
//   - PrintTreePeriodically() which visualizes a Context's child tree and is helpful for debugging in large projects.
type Context interface {
	Log() log.Logger

	// Includes functionality and behavior of a context.Context.
	context.Context

	// Returns a snapshot of this Context's Info.
	Info() Info

	// Creates a new child Context with for given Task.
	// If OnStart() returns an error error is encountered, then child.Close() is immediately called and the error is returned.
	StartChild(task *Task) (Context, error)

	// Convenience function for StartChild() and is equivalent to:
	//
	//      parent.StartChild(label, &Task{
	//  		IdleClose: time.Nanosecond,
	// 	        OnRun: fn,
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
	// First, children get Close() in breath-first order.
	// After all children are done closing, OnClosing(), then OnClosed() are executed.
	Close() error

	// Inserts a pending Close() on this Context once it is idle after the given delay.
	// Subsequent calls will update the delay but the previously pending delay must run out first.
	// If at the end of the period Task.OnRun() is complete, there are no children, PreventIdleClose() is not in effect, then Close() is called.
	CloseWhenIdle(delay time.Duration)

	// Ensures that that this Context will not automatically idle-close until the given delay has passed.
	// If previous PreventIdleClose calls were made, the more limiting delay is retained.
	//
	// Returns false if this Context has already been closed.
	PreventIdleClose(delay time.Duration) bool

	// Signals when Close() has been called.
	// First, Children get Close(),  then OnClosing, then OnClosed are executing
	Closing() <-chan struct{}

	// Signals when Close() has fully executed, no children remain, and OnClosed() has been completed.
	Done() <-chan struct{}
}
