package lifecycle

import (
	"context"
	"iter"
	"sync/atomic"
)

// Worker runs a task that processes data from a channel and reports its lifecycle status.
func Worker[E any](ctx context.Context, dataCh <-chan E, fn WorkerFunc[E]) (st WorkerStatus) {
	w := &workerImpl[E]{
		ctx:     ctx,
		dataCh:  dataCh,
		readyCh: make(chan struct{}),
		idleCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	go func() {
		w.err = fn(ctx, w.run)

		select {
		case <-w.readyCh:
			// idle must be already closed
		default:
			close(w.idleCh)
		}

		close(w.doneCh)
	}()

	return w
}

type WorkerStatus interface {
	// Ready returns a channel which is closed when this Worker's iterator has been started.
	// This may never be closed.
	Ready() (ch <-chan struct{})

	// Idle returns a unique channel which is closed when this Worker is no longer reading its iterator.
	// It yields true if Ready() was previously closed.
	Idle() (ch <-chan bool)

	// Done returns a unique channel which is closed when this Worker is finally shutdown.
	// This returns any error reason (context or direct).
	Done() (ch <-chan error)
}

// WorkerFunc runs a task.
// The passed iterator can only be called once, or it will panic.
type WorkerFunc[E any] func(ctx context.Context, events iter.Seq[E]) (err error)

type workerImpl[E any] struct {
	ctx     context.Context
	dataCh  <-chan E
	started atomic.Bool

	readyCh chan struct{}
	idleCh  chan struct{}

	err    error
	doneCh chan struct{}
}

func (w *workerImpl[E]) Ready() (ch <-chan struct{}) {
	return w.readyCh
}

func (w *workerImpl[E]) Idle() (ch <-chan bool) {
	out := make(chan bool)

	go func() {
		// wait for shutdown reason
		select {
		case <-w.idleCh:
		case <-w.ctx.Done(): // TODO: is this racey?
		}

		// were we ever read?
		select {
		case <-w.readyCh:
			out <- true
		default:
		}
		close(out)
	}()

	return out
}

func (w *workerImpl[E]) Done() (ch <-chan error) {
	out := make(chan error, 1)

	go func() {
		select {
		case <-w.doneCh:
			if w.err != nil {
				out <- w.err
			}
		case <-w.ctx.Done():
			out <- context.Cause(w.ctx)
		}
		close(out)
	}()

	return out
}

func (w *workerImpl[E]) run(yield func(e E) (more bool)) {
	if !w.started.CompareAndSwap(false, true) {
		panic("run called twice")
	}

	select {
	case <-w.ctx.Done():
		// shutdown: we got called, but ctx was already cancelled
		return
	default:
	}

	close(w.readyCh) // will panic if called twice
	defer close(w.idleCh)

	for {
		// always check early
		select {
		case <-w.ctx.Done():
			// shutdown: external, ctx cancelled (immediate)
			return
		default:
		}

		// wait
		select {
		case next, ok := <-w.dataCh:
			if !ok {
				// shutdown: external, no more data
				return
			} else if !yield(next) {
				// shutdown: inner, break out of iterator
				return
			}
		case <-w.ctx.Done():
			// shutdown: external, ctx cancelled (immediate)
			return
		}
	}
}
