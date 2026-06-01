package task

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/alog"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// ctx implements Context.
//
// All mutable fields are guarded by mu. state, chClosing, and chClosed give the
// two-phase close: Close() moves Running -> Closing and closes chClosing; once
// children and OnRun have drained, the monitor moves Closing -> Closed and
// closes chClosed.
type ctx struct {
	log  alog.Logger
	task Task

	mu        sync.Mutex
	state     int32         // Running | Closing | Closed
	subs      []Context     // live child Contexts
	active    int           // outstanding work: live children + a reservation held across setup/OnRun; 0 means idle
	changed   chan struct{} // lazily created; closed by signalChange to broadcast a state change, then cleared
	idleDelay time.Duration // CloseWhenIdle delay; <= 0 disables idle-close
	idleFloor time.Time     // PreventIdleClose floor; idle-close may not fire before this instant
	idleLoop  bool          // true while an idleCloseLoop goroutine is live

	chClosing chan struct{} // closed when Close() is first called
	chClosed  chan struct{} // closed once close-down is complete and OnClosed has run
}

// Errors
var (
	ErrNotRunning = errors.New("not running")
)

func (c *ctx) Close() error {
	c.mu.Lock()
	c.close()
	c.mu.Unlock()
	return nil
}

// close moves c from Running to Closing and signals it. It is a no-op if c is
// already closing or closed. The caller must hold c.mu.
func (c *ctx) close() {
	if c.state != Running {
		return
	}
	c.state = Closing
	close(c.chClosing)
	c.signalChange()
}

// signalChange wakes every goroutine blocked on the broadcast channel returned
// by changeChan. The caller must hold c.mu. It is a cheap no-op when no waiter
// is parked, so callers may signal liberally on any state change.
func (c *ctx) signalChange() {
	if c.changed != nil {
		close(c.changed)
		c.changed = nil
	}
}

// changeChan returns the broadcast channel that the next signalChange will
// close. The caller must hold c.mu.
func (c *ctx) changeChan() <-chan struct{} {
	if c.changed == nil {
		c.changed = make(chan struct{})
	}
	return c.changed
}

// release drops the work reservation held across setup and OnRun, marking c
// idle once no children remain.
func (c *ctx) release() {
	c.mu.Lock()
	c.active--
	c.signalChange()
	c.mu.Unlock()
}

// waitIdle blocks until c has no outstanding work (no children and no running
// OnRun). It is used by the monitor to drain a Context before finalizing it.
func (c *ctx) waitIdle() {
	for {
		c.mu.Lock()
		if c.active == 0 {
			c.mu.Unlock()
			return
		}
		wait := c.changeChan()
		c.mu.Unlock()
		<-wait
	}
}

func (c *ctx) PreventIdleClose(delay time.Duration) bool {
	c.mu.Lock()
	if floor := time.Now().Add(delay); floor.After(c.idleFloor) {
		c.idleFloor = floor
	}
	running := c.state == Running
	c.signalChange()
	c.mu.Unlock()
	return running
}

func (c *ctx) CloseWhenIdle(delay time.Duration) {
	c.mu.Lock()
	c.idleDelay = delay
	spawn := delay > 0 && !c.idleLoop && c.state == Running
	if spawn {
		c.idleLoop = true
	}
	c.signalChange()
	c.mu.Unlock()

	if spawn {
		go c.idleCloseLoop()
	}
}

// idleCloseLoop closes c once it has stayed idle for idleDelay, honoring any
// PreventIdleClose floor. Exactly one runs at a time, guarded by c.idleLoop;
// CloseWhenIdle either starts it or nudges the running one via signalChange.
func (c *ctx) idleCloseLoop() {
	for {
		c.mu.Lock()
		if c.state != Running || c.idleDelay <= 0 {
			c.idleLoop = false
			c.mu.Unlock()
			return
		}
		active := c.active
		delay := c.idleDelay
		floor := c.idleFloor
		wake := c.changeChan()
		c.mu.Unlock()

		if active > 0 {
			// Busy: re-evaluate when the work count or close-state changes.
			select {
			case <-wake:
			case <-c.Closing():
			}
			continue
		}

		// Idle: close after delay, but not before any PreventIdleClose floor.
		dur := delay
		if !floor.IsZero() {
			if until := time.Until(floor); until > dur {
				dur = until
			}
		}
		timer := time.NewTimer(dur)
		select {
		case <-timer.C:
			c.mu.Lock()
			ready := c.active == 0 && c.idleDelay > 0 &&
				(c.idleFloor.IsZero() || !time.Now().Before(c.idleFloor))
			if ready {
				c.idleLoop = false
				c.close()
			}
			c.mu.Unlock()
			if ready {
				return
			}
		case <-wake:
			timer.Stop()
		case <-c.Closing():
			timer.Stop()
		}
	}
}

func (c *ctx) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

// Err reports cancellation as soon as Close() is called (i.e. once Closing() is
// signaled), ahead of Done(). It returns context.Canceled while closing or
// closed, and nil while still running.
func (c *ctx) Err() error {
	select {
	case <-c.chClosing:
		return context.Canceled
	default:
		return nil
	}
}

func (c *ctx) Value(key any) any {
	return nil
}

func (c *ctx) Info() Info {
	return c.task.Info
}

func (c *ctx) Log() alog.Logger {
	return c.log
}

func printContextTree(ctx Context, out *strings.Builder, depth int, prefix []rune, lastChild bool) {
	// First character for this node
	icon := ' '
	if depth > 0 {
		icon = '┣'
		if lastChild {
			icon = '┗'
		}
	}

	// Print this node
	info := ctx.Info()
	prefix = append(prefix, icon, ' ')
	out.WriteString(info.TaskID.AsLabel())
	out.WriteString(string(prefix))
	out.WriteString(ctx.Log().GetLogLabel())
	out.WriteByte('\n')

	// Set up prefix for children
	// Remove the current node's icon and space
	prefix = prefix[:len(prefix)-2]
	// Add vertical line (or space if last child) plus padding
	if depth > 0 && !lastChild {
		prefix = append(prefix, '┃', ' ', ' ')
	} else {
		prefix = append(prefix, ' ', ' ', ' ')
	}

	// Print children
	var subBuf [20]Context
	children := ctx.GetChildren(subBuf[:0])
	for i, ci := range children {
		printContextTree(ci, out, depth+1, prefix, i == len(children)-1)
	}
}

func (c *ctx) GetChildren(in []Context) []Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append(in, c.subs...)
}

func (c *ctx) ForEachChild(fn func(child Context)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, child := range c.subs {
		fn(child)
	}
}

// StartChild starts the given Task as a child of c, returning its Context.
// If c is no longer running, ErrNotRunning is returned. If Task.OnStart returns
// an error, the child is closed and that error is returned.
func (c *ctx) StartChild(task Task) (Context, error) {
	if task.TaskID.IsNil() {
		task.TaskID = tag.NowID()
	}
	if task.Label == "" {
		task.Label = task.Info.TaskID.AsLabel()
	}
	child := &ctx{
		log:       alog.NewLogger(task.Label),
		state:     Running,
		task:      task,
		chClosing: make(chan struct{}),
		chClosed:  make(chan struct{}),
		active:    1, // reservation held across setup; released by OnStart error, OnRun, or below
	}

	// Attach to the parent. A nil receiver (task.Start) starts a parentless root.
	if c != nil {
		c.mu.Lock()
		if c.state != Running {
			c.mu.Unlock()
			return nil, ErrNotRunning
		}
		c.subs = append(c.subs, child)
		c.active++
		c.signalChange()
		c.mu.Unlock()
	}

	// The monitor drives the child through its closing lifecycle. It is launched
	// before OnStart so a parent close can interrupt a blocking OnStart; the
	// reservation above keeps the child from finalizing mid-setup.
	go child.runMonitor(c)

	// OnStart runs synchronously; an error closes the child and is returned.
	if child.task.OnStart != nil {
		err := child.task.OnStart(child)
		child.task.OnStart = nil
		if err != nil {
			child.Close()
			child.release()
			return nil, err
		}
	}

	// OnRun is the async work body. It inherits the reservation and releases it
	// on return, then arms idle-close if configured.
	if child.task.OnRun != nil {
		go func() {
			child.task.OnRun(child)
			idleClose := child.task.Info.IdleClose
			child.task.OnRun = nil
			child.release()
			if idleClose > 0 {
				child.CloseWhenIdle(idleClose)
			}
		}()
	} else {
		child.release()
	}

	return child, nil
}

// runMonitor drives a single child Context through its closing lifecycle.
// Exactly one runs per Context, launched by StartChild.
func (child *ctx) runMonitor(parent *ctx) {
	// Propagate a parent close down to the child; otherwise wait for the child
	// to be closed on its own.
	if parent != nil {
		select {
		case <-parent.Closing():
			child.Close()
		case <-child.Closing():
		}
	}
	<-child.Closing()

	// Closing has begun: run the cleanup hooks before draining children.
	if child.task.OnClosing != nil {
		child.task.OnClosing()
	}
	if parent != nil && parent.task.OnChildClosing != nil {
		parent.task.OnChildClosing(child)
	}

	// Wait for the child's own work (its children and OnRun) to finish.
	child.waitIdle()

	// Detach from the parent. If this was the parent's last child, the parent
	// may now idle-close.
	var parentIdleClose time.Duration
	if parent != nil {
		parent.mu.Lock()
		parent.removeChildLocked(child)
		parent.active--
		if parent.active == 0 {
			parentIdleClose = parent.task.Info.IdleClose
		}
		parent.signalChange()
		parent.mu.Unlock()
	}

	// Finalize: nothing remains but the OnClosed hook and releasing Done().
	child.mu.Lock()
	child.state = Closed
	child.mu.Unlock()
	if child.task.OnClosed != nil {
		child.task.OnClosed()
	}
	close(child.chClosed)

	if parentIdleClose > 0 {
		parent.CloseWhenIdle(parentIdleClose)
	}
}

// removeChildLocked removes child from c.subs. The caller must hold c.mu.
func (c *ctx) removeChildLocked(child *ctx) {
	for i, sub := range c.subs {
		if sub == child {
			last := len(c.subs) - 1
			copy(c.subs[i:], c.subs[i+1:])
			c.subs[last] = nil // release the reference for GC
			c.subs = c.subs[:last]
			return
		}
	}
}

func (c *ctx) Go(label string, fn func(ctx Context)) (Context, error) {
	return c.StartChild(Task{
		Info: Info{
			Label:     label,
			IdleClose: time.Nanosecond,
		},
		OnRun: fn,
	})
}

func (c *ctx) Closing() <-chan struct{} {
	return c.chClosing
}

func (c *ctx) Done() <-chan struct{} {
	return c.chClosed
}

const (
	Unstarted int32 = iota
	Running
	Closing
	Closed
)

func (t *Task) TaskInfo() *Info {
	return &t.Info
}
