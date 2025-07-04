package future

import (
	"context"
	"sync"
)

// Future represents some future result.
type Future[T any] interface {
	// Wait for the future to resolve. Returns the context cause if it's canceled.
	Wait(ctx context.Context) (T, error)

	// Sync checks the future's result immediately, returning false if not yet available.
	Sync() (T, error, bool)
}

type futureImpl[T any] struct {
	doneCh <-chan struct{}
	result T
	err    error
	once   sync.Once
}

func (f *futureImpl[T]) Wait(ctx context.Context) (res T, err error) {
	err = context.Cause(ctx)
	if err != nil {
		// reminder to self: select {} chooses a random choice, have to do this first
		return
	}

	select {
	case <-ctx.Done():
		err = context.Cause(ctx)
		return
	case <-f.doneCh:
	}
	return f.result, f.err
}

func (f *futureImpl[T]) Sync() (res T, err error, ok bool) {
	select {
	case <-f.doneCh:
	default:
		return
	}
	return f.result, f.err, true
}

// New creates a new resolvable future.
func New[T any]() (Future[T], func(result T, err error)) {
	doneCh := make(chan struct{})

	f := &futureImpl[T]{
		doneCh: doneCh,
	}
	resolve := func(result T, err error) {
		// ignore additional calls
		f.once.Do(func() {
			f.err = err
			f.result = result
			close(doneCh)
		})
	}

	return f, resolve
}
