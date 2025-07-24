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

	// Halt runs the given method when this group may be about to shut down.
	// It is passed a channel which is closed if the group restarts.
	// It will only run after a successful Start() and then a potential shutdown.
	// Any returned error will cancel the group's [context.Context] directly, rather than with any provided cause.
	// Returns true if the method has a chance of running.
	Halt(func(c context.Context, resume <-chan struct{}) error) bool
}

// New creates a new CGroup, which simply provides a [context.Context] while any passed context is active.
func New() CGroup {
	return NewCause(nil)
}

// NewCause creates a new CGroup that will eventually cancel with the specified cause.
func NewCause(cause error) CGroup {
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
	select {
	case <-c.Done():
		return
	default:
	}

	cg.lock.Lock()
	defer cg.lock.Unlock()

	if cg.ctx != nil {
		select {
		case <-cg.ctx.Done():
			return // don't include, already done
		default:
		}
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

	select {
	case <-cg.ctx.Done():
		return false
	default:
	}
	go cg.start(fn)
	return true
}

func (cg *cgroup) Halt(fn func(context.Context, <-chan struct{}) error) bool {
	cg.lock.Lock()
	defer cg.lock.Unlock()

	if cg.ctx != nil {
		select {
		case <-cg.ctx.Done():
			return false
		default:
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
