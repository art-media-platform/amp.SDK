package task

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/log"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// ctx implements Context
type ctx struct {
	log            log.Logger
	task           Task
	state          int32
	idle           bool
	idleCloseRetry atomic.Int64 // time.Duration
	idleCloseMin   time.Time

	chClosing chan struct{}  // signals Close() has been called and close execution has begun.
	chClosed  chan struct{}  // signals Close() has been called and all close execution is done.
	err       error          // See context.Err() for spec
	busy      sync.WaitGroup // blocks until all execution is complete
	subsMu    sync.Mutex     // Locked when .subs is being accessed
	subs      []Context
}

// Errors
var (
	ErrAlreadyStarted = errors.New("already started")
	ErrNotRunning     = errors.New("not running")
	ErrClosed         = errors.New("closed")
)

func (p *ctx) Close() error {
	first := atomic.CompareAndSwapInt32(&p.state, Running, Closing)
	if first {
		close(p.chClosing)
	}
	return nil
}

func (p *ctx) PreventIdleClose(delay time.Duration) bool {
	p.subsMu.Lock()
	p.idleCloseMin = time.Now().Add(delay)
	p.idle = false
	p.subsMu.Unlock()

	select {
	case <-p.Closing():
		return false
	default:
		return true
	}
}

func (p *ctx) CloseWhenIdle(delay time.Duration) {
	if delay <= 0 {
		delay = 0
	}

	// Can this be folded into the main go routine in StartChild() to save a goroutine?
	prevDelay := p.idleCloseRetry.Swap(int64(delay))

	// Only spawn a new timer when the delay is changed from 0
	if prevDelay > 0 {
		return
	}

	go func() {
		var timer *time.Timer

		for idleClose := true; idleClose; {
			p.idle = true
			p.busy.Wait() // wait until there is a chance of catching ctx idle

			retry := false

			p.subsMu.Lock()
			delay := time.Duration(p.idleCloseRetry.Load())
			if !p.idle {
				retry = true
			} else if delay <= 0 {
				idleClose = false
			} else {
				if !p.idleCloseMin.IsZero() {
					minDelay := time.Until(p.idleCloseMin)
					if minDelay <= 0 {
						p.idleCloseMin = time.Time{}
					}
					// Wait for the more restrictive time constraint
					if delay < minDelay {
						delay = minDelay
					}
				}
			}
			p.subsMu.Unlock()

			if retry || !idleClose {
				continue
			}

			if delay > 0 {
				if timer == nil {
					timer = time.NewTimer(delay)
				} else {
					timer.Reset(delay)
				}
				select {
				case <-timer.C:
				case <-p.Closing():
					idleClose = false
				}
			}

			// If no new children were added while we were waiting, then we have been idle and can close.
			// Note in the case that we're closing, the below has no effect
			if idleClose {
				p.subsMu.Lock()
				if p.idle {
					p.Close()
					idleClose = false
				}
				p.subsMu.Unlock()
			}
		}

		if timer != nil {
			timer.Stop()
		}
	}()
}

func (p *ctx) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (p *ctx) Err() error {
	select {
	case <-p.Done():
		if p.err == nil {
			return context.Canceled
		}
		return p.err
	default:
		return nil
	}
}

func (p *ctx) Value(key any) any {
	return nil
}

func (p *ctx) Info() Info {
	return p.task.Info
}

func (p *ctx) Log() log.Logger {
	return p.log
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

func (p *ctx) GetChildren(in []Context) []Context {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()
	return append(in, p.subs...)
}

func (p *ctx) ForEachChild(fn func(child Context)) {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()
	for _, child := range p.subs {
		fn(child)
	}
}

// StartChild starts the given child Context as a "sub" task.
func (p *ctx) StartChild(task Task) (Context, error) {
	if task.TaskID.IsNil() {
		task.TaskID = tag.NowID()
	}
	if task.Label == "" {
		task.Label = task.Info.TaskID.AsLabel()
	}
	child := &ctx{
		log:       log.NewLogger(task.Label),
		state:     Running,
		task:      task,
		chClosing: make(chan struct{}),
		chClosed:  make(chan struct{}),
	}

	// If a parent is given, add the child to the parent's list of children.
	if p != nil {
		var err error
		p.subsMu.Lock()
		if p.state == Running {
			p.busy.Add(1)
			p.idle = false
			p.subs = append(p.subs, child)
		} else {
			err = ErrNotRunning
		}
		p.subsMu.Unlock()

		if err != nil {
			return nil, err
		}
	}

	go func() {

		// If there is a parent, wait until child.Close() *or* p.Close()
		// TODO: merge CloseWhenIdle() into this block?
		if p != nil {
			select {
			case <-p.Closing():
				child.Close()
			case <-child.Closing():
			}
		}

		// Wait for child to begin closing phase
		<-child.Closing()

		// Fire callback if given
		if child.task.OnClosing != nil {
			child.task.OnClosing()
		}

		if p != nil && p.task.OnChildClosing != nil {
			p.task.OnChildClosing(child)
		}

		// Once all child's children are closed, proceed with completion.
		child.busy.Wait()

		var idleClose time.Duration

		if p != nil {

			p.subsMu.Lock()
			{
				// remove the child from its parent
				N := len(p.subs)
				for i := 0; i < N; i++ {
					if p.subs[i] == child {
						copy(p.subs[i:], p.subs[i+1:N])
						N--
						p.subs[N] = nil // show GC some love
						p.subs = p.subs[:N]
						break
					}
				}

				// If removing the last child and in IdleClose mode, queue the parent to be closed
				if N == 0 {
					idleClose = p.task.Info.IdleClose
				}
			}
			p.subsMu.Unlock()
		}

		// Move to Closed state now that all all that remains is the OnClosed callback and release of the chClosed chan.
		child.state = Closed
		if child.task.OnClosed != nil {
			child.task.OnClosed()
		}
		close(child.chClosed)

		// With the child now fully closed, the parent is no longer waiting on this child
		if p != nil {
			p.busy.Done()
		}

		if idleClose > 0 {
			p.CloseWhenIdle(idleClose)
		}
	}()

	if child.task.OnStart != nil {
		err := child.task.OnStart(child)
		child.task.OnStart = nil
		if err != nil {
			child.Close()
			return nil, err
		}
	}

	if child.task.OnRun != nil {
		child.busy.Add(1)
		go func() {
			child.task.OnRun(child)
			child.task.OnRun = nil
			child.busy.Done()

			// If idleclose is set, try to do so
			if child.task.Info.IdleClose > 0 {
				child.CloseWhenIdle(child.task.Info.IdleClose)
			}
		}()
	}

	return child, nil
}

func (p *ctx) Go(label string, fn func(ctx Context)) (Context, error) {
	return p.StartChild(Task{
		Info: Info{
			Label:     label,
			IdleClose: time.Nanosecond,
		},
		OnRun: fn,
	})
}

func (p *ctx) Closing() <-chan struct{} {
	return p.chClosing
}

func (p *ctx) Done() <-chan struct{} {
	return p.chClosed
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
