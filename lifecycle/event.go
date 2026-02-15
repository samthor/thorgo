package lifecycle

import (
	"context"
	"iter"
	"sync/atomic"
)

func RunFoo[E any](ctx context.Context, dataCh <-chan E, fn FooFunc[E]) (st FooStatus) {
	f := &fooImpl[E]{
		ctx:     ctx,
		dataCh:  dataCh,
		readyCh: make(chan struct{}),
		idleCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	go func() {
		f.err = fn(ctx, f.run)

		select {
		case <-f.readyCh:
			// idle must be already closed
		default:
			close(f.idleCh)
		}

		close(f.doneCh)
	}()

	return f
}

type FooStatus interface {
	// Ready returns a channel which is closed when this Foo's iterator has been started.
	// This may never be closed.
	Ready() (ch <-chan struct{})

	// Idle returns a unique channel which is closed when this Foo is no longer reading its iterator.
	// It yields true if Ready() was previously closed.
	Idle() (ch <-chan bool)

	// Done returns a unique channel which is closed when this Foo is finally shutdown.
	// This returns any error reason (context or direct).
	Done() (ch <-chan error)
}

// FooFunc runs a task.
// The passed iterator can only be called once, or it will panic.
type FooFunc[E any] func(ctx context.Context, events iter.Seq[E]) (err error)

type fooImpl[E any] struct {
	ctx     context.Context
	dataCh  <-chan E
	started atomic.Bool

	readyCh chan struct{}
	idleCh  chan struct{}

	err    error
	doneCh chan struct{}
}

func (f *fooImpl[E]) Ready() (ch <-chan struct{}) {
	return f.readyCh
}

func (f *fooImpl[E]) Idle() (ch <-chan bool) {
	out := make(chan bool)

	go func() {
		// wait for shutdown reason
		select {
		case <-f.idleCh:
		case <-f.ctx.Done(): // TODO: is this racey?
		}

		// were we ever read?
		select {
		case <-f.readyCh:
			out <- true
		default:
		}
		close(out)
	}()

	return out
}

func (f *fooImpl[E]) Done() (ch <-chan error) {
	out := make(chan error, 1)

	go func() {
		select {
		case <-f.doneCh:
			if f.err != nil {
				out <- f.err
			}
		case <-f.ctx.Done():
			out <- context.Cause(f.ctx)
		}
		close(out)
	}()

	return out
}

func (f *fooImpl[E]) run(yield func(e E) (more bool)) {
	if !f.started.CompareAndSwap(false, true) {
		panic("run called twice")
	}

	select {
	case <-f.ctx.Done():
		// shutdown: we got called, but ctx was already cancelled
		return
	default:
	}

	close(f.readyCh) // will panic if called twice

outer:
	for {
		// always check early
		select {
		case <-f.ctx.Done():
			// shutdown: external, ctx cancelled (immediate)
			break outer
		default:
		}

		// wait
		select {
		case next, ok := <-f.dataCh:
			if !ok {
				// shutdown: external, no more data
				break outer
			} else if !yield(next) {
				// shutdown: inner, break out of iterator
				break outer
			}
		case <-f.ctx.Done():
			// shutdown: external, ctx cancelled (immediate)
			break outer
		}
	}

	close(f.idleCh)
}
