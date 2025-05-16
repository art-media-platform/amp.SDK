package closer

import (
	"io"
	"sync/atomic"
)

// Wrapper is a reference counted io.Closer where the wrapped Closer is closed when its reference count reaches zero.
type Wrapper interface {

	// PushOpen atomically increments the reference count.
	PushOpen()

	// Close atomically "pops" the pushed open count.
	// If the ref count > 0, nil is always returned.
	// IF the ref count == 0, the wrapped Closer is closed and its error is returned.
	Close() error
}

// Wraps the given io.Closer into a Wrapper, initializing its reference count to 1.
func WrapCloser(target io.Closer) Wrapper {
	wr := &wrapper{
		closer:   target,
		refCount: 1,
	}
	return wr
}

type wrapper struct {
	closer   io.Closer
	refCount int32
}

func (w *wrapper) PushOpen() {
	atomic.AddInt32(&w.refCount, 1)
}

func (w *wrapper) Close() error {
	count := atomic.AddInt32(&w.refCount, -1)
	if count > 0 {
		return nil
	}
	if count < 0 {
		panic("closer.Wrapper: Close called too many times")
	}
	err := w.closer.Close()
	w.closer = nil
	return err
}
