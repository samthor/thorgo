package lifecycle

import (
	"context"
	"sync"
)

// CGroup provides a [context.Context] while any contained context is active.
type CGroup interface {
	// Adds the context to this group.
	// Returns true if the underlying context is not yet done and this context was added to the active set, even briefly.
	Add(c context.Context) (ok bool)

	// Starts or retrieves the prior context for this CGroup.
	// This will be already canceled if no contexts were added.
	Start() (ctx context.Context)

	// Wait ensures this group has started, and then blocks until the [context.Context] is completed, returning its cause if not the default [context.Canceled].
	Wait() (err error)

	// Go runs the given method as part of this group.
	// It will only start after Start() has been called with valid contexts.
	// Any returned error will cancel the group's [context.Context] directly, rather than with any provided cause.
	// Returns true if the method has a chance of running.
	Go(fn func(c context.Context) (err error)) (ok bool)

	// Halt runs the given method when this group may be about to shut down.
	//
	// It is passed a channel which is closed if the group restarts.
	// The method passed here should prevent further Add() calls in your code before doing teardown work.
	// Otherwise, you risk a resume race.
	//
	// The passed method will only run after a successful Start() and then a potential shutdown.
	// Any returned error will cancel the group's [context.Context] directly, rather than with any provided cause.
	// Returns true if the method has a chance of running.
	Halt(fn func(c context.Context, resume <-chan struct{}) (err error)) (ok bool)
}

// NewCGroup creates a new CGroup, which simply provides a [context.Context] while any passed context is active.
// The new context is not derived from anything.
func NewCGroup() (cg CGroup) {
	return NewCGroupCause(nil)
}

// NewCGroupCause creates a new CGroup that will eventually cancel with the specified cause.
func NewCGroupCause(cause error) (cg CGroup) {
	return &cgroup{cause: cause}
}

type haltFunc func(c context.Context, resume <-chan struct{}) error

type cgroup struct {
	cause  error
	lock   sync.Mutex
	ctx    context.Context
	cancel context.CancelCauseFunc
	active int
	tasks  []func(context.Context) error
	halts  []haltFunc

	resumeCh    chan struct{}
	resumeStart func(haltFunc)
}

func (cg *cgroup) Add(c context.Context) (ok bool) {
	if IsDone(c) {
		return
	}

	cg.lock.Lock()
	defer cg.lock.Unlock()

	if cg.ctx != nil && IsDone(cg.ctx) {
		return // don't include, already done
	}
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

		if cg.ctx == nil {
			// if all contexts run and die before Start(), we can still get a valid context if another one is added later :shrug:
			return
		}

		if cg.resumeStart != nil || cg.resumeCh != nil {
			panic("shutdown with resumeStart already set")
		}

		resumeCh := make(chan struct{})
		resumeGroup := &sync.WaitGroup{}

		resumeStart := func(hf haltFunc) {
			resumeGroup.Add(1)
			go func() {
				defer resumeGroup.Done()
				err := hf(cg.ctx, resumeCh)
				if err != nil {
					cg.cancel(err)
				}
			}()
		}
		cg.resumeStart = resumeStart
		cg.resumeCh = resumeCh

		for _, halt := range cg.halts {
			resumeStart(halt)
		}

		go func() {
			resumeGroup.Wait()

			cg.lock.Lock()
			defer cg.lock.Unlock()

			if cg.resumeCh == resumeCh {
				cg.cancel(cg.cause)
			}
		}()
	})

	if cg.ctx == nil {
		return true
	}

	if cg.resumeCh != nil {
		close(cg.resumeCh)
		cg.resumeCh = nil
		cg.resumeStart = nil
	}

	return true
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

	if IsDone(cg.ctx) {
		return false
	}
	go cg.start(fn)
	return true
}

func (cg *cgroup) Halt(fn func(context.Context, <-chan struct{}) error) bool {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	if cg.ctx != nil {
		if IsDone(cg.ctx) {
			return false
		}

		// start immediately, we're in shutdown phase
		if cg.resumeStart != nil {
			cg.resumeStart(fn)
		}
	}

	// push into stack for later
	cg.halts = append(cg.halts, fn)
	return true
}

func (cg *cgroup) start(fn func(context.Context) error) {
	err := fn(cg.ctx)
	if err != nil {
		cg.cancel(err)
	}
}
