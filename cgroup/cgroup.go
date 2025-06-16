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
	// This will be already cancelled if no contexts were added.
	Start() context.Context
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
		}

		if cg.active == 0 && cg.cancel != nil {
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

	if cg.ctx == nil {
		cg.ctx, cg.cancel = context.WithCancelCause(context.Background())
		if cg.active == 0 {
			cg.cancel(cg.cause)
		}
	}

	return cg.ctx
}
