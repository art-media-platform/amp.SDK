package closer

import (
	"context"
	"net/url"
	"slices"
	"sync"
	"time"
)

// A lightweight context.Context that can be closed explicitly.
//
// All methods are concurrency safe.
type Context interface {

	// Use whenever a context.Context is needed
	context.Context

	// Accesses string value map exposed via Context.Value(), returning a copy entry for the requested key.
	Values(key string) []string

	// Replaces entries to existing value map exposed via Context.Value()
	MergeValues(src url.Values)

	// Close signals Done() and assigns the value that Err() will return.
	//
	// No-op if the context is already closed.
	Close(err error)
}

// ctx implements closer.Context
var _ Context = (*ctx)(nil)

// ctx implements closer.Context
type ctx struct {
	err      error
	once     sync.Once
	closed   chan struct{}
	valuesMu sync.Mutex
	values   url.Values
	deadline time.Time
}

// Close signals the context as closed, and assigns the value that this Context.Err() will return.
//
// This is a no-op if the context is already closed.
func (p *ctx) Close(err error) {
	p.once.Do(func() {
		p.err = err
		close(p.closed)
	})
}

// Returns a new closer.Context that signals Done() once when either: the input context signals, or if Close() is called.
// If existing == nil, the returned context only closes once Close() is called.
func WrapContext(input context.Context) Context {
	p := &ctx{
		closed: make(chan struct{}),
	}
	if input != nil {
		if deadline, ok := input.Deadline(); ok {
			p.deadline = deadline
		}
		go func() {
			select {
			case <-input.Done():
				p.Close(input.Err())
			case <-p.closed:
				// already closed, muchachos
			}
		}()
	}

	return p
}

// Done returns a channel that is closed when the signal is closed
func (p *ctx) Done() <-chan struct{} {
	return p.closed
}

// Conforms to context.Context with the exception that the returned error could be nil if Close(nil) was called.
func (p *ctx) Err() error {
	select {
	case <-p.closed:
		return p.err
	default:
		return nil
	}
}

// Conforms to context.Context
func (p *ctx) Value(key any) any {
	str, ok := key.(string)
	if !ok {
		return nil
	}
	values := p.Values(str)
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

// Conforms to context.Context
func (p *ctx) Values(key string) (values []string) {
	p.valuesMu.Lock()
	defer p.valuesMu.Unlock()

	if p.values != nil {
		if src := p.values[key]; len(src) > 0 {
			values = slices.Clone(src)
		}
	}
	return
}

func (p *ctx) MergeValues(valueMap url.Values) {
	p.valuesMu.Lock()
	defer p.valuesMu.Unlock()

	if p.values == nil {
		p.values = url.Values{}
	}
	for k, values_in := range valueMap {
		p.values[k] = slices.Clone(values_in)
	}
}

func (p *ctx) Deadline() (deadline time.Time, ok bool) {
	return p.deadline, !p.deadline.IsZero()
}
