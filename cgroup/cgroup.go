package cgroup

import (
	"context"
	"sync"
)

type CGroup interface {
	// Adds the context to this group.
	// Returns true if the underlying context is not yet done and this context was added to the active set, even briefly.
	Add(c context.Context) (ok bool)

	// Starts or retrieves the prior context for this CGroup.
	// This will be already canceled if no contexts were added.
	Start() context.Context

	// Wait ensures this group has started, and then blocks until the [context.Context] is completed, returning its cause if not the default [context.Canceled].
	Wait() error

	// Go runs the given method as part of this group.
	// It will only start after Start() has been called with valid contexts.
	// Any returned error will cancel the group's [context.Context] directly, rather than with any provided cause.
	// Returns true if the method has a chance of running.
	Go(func(c context.Context) error) bool
}

// New creates a new CGroup, which simply provides a [context.Context] while any passed context is active.
func New() CGroup {
	return NewCause(nil)
}

// NewCause creates a new CGroup that will eventually cancel with the specified cause.
func NewCause(cause error) CGroup {
	return &cgroup{
		cause: cause,
	}
}

type cgroup struct {
	cause  error
	lock   sync.Mutex
	ctx    context.Context
	cancel context.CancelCauseFunc
	active int
	tasks  []func(context.Context) error
}

func (cg *cgroup) Add(c context.Context) (ok bool) {
	select {
	case <-c.Done():
		return false
	default:
	}

	cg.lock.Lock()
	defer cg.lock.Unlock()

	cg.active++

	context.AfterFunc(c, func() {
		cg.lock.Lock()
		defer cg.lock.Unlock()

		cg.active--
		if cg.active < 0 {
			panic("got -ve active in CGroup")
		} else if cg.active > 0 {
			return
		}

		// if all contexts run and die before Start(), we can still get a valid context if another one is added later :shrug:
		if cg.ctx != nil {
			cg.cancel(cg.cause)
		}
	})

	if cg.ctx == nil {
		return true
	}
	select {
	case <-cg.ctx.Done():
		return false
	default:
		return true
	}
}

func (cg *cgroup) Start() context.Context {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	if cg.ctx != nil {
		return cg.ctx
	}

	cg.ctx, cg.cancel = context.WithCancelCause(context.Background())
	if cg.active == 0 {
		cg.cancel(cg.cause)
	} else {
		for _, fn := range cg.tasks {
			go cg.start(fn)
		}
	}

	cg.tasks = nil
	return cg.ctx
}

func (cg *cgroup) Wait() (err error) {
	ctx := cg.Start()
	<-ctx.Done()
	err = context.Cause(ctx)
	if err == context.Canceled {
		err = nil
	}
	return
}

func (cg *cgroup) Go(fn func(context.Context) error) bool {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	if cg.ctx == nil {
		cg.tasks = append(cg.tasks, fn)
		return true
	}

	select {
	case <-cg.ctx.Done():
		return false
	default:
	}
	go cg.start(fn)
	return true
}

func (cg *cgroup) start(fn func(context.Context) error) {
	err := fn(cg.ctx)
	if err != nil {
		cg.cancel(err)
	}
}
