package context

import (
	"context"
	"sync"
)

// NewGroup creates a new ContextGroup with the given initial context.
// When all added contexts are complete, cancels the contained context.
// If the passed contexts are already cancelled, this might return a cancelled group.
func NewGroup(initial context.Context, rest ...context.Context) *Group {
	ctx, cancel := context.WithCancel(context.Background())

	g := &Group{
		C:      ctx,
		cancel: cancel,
		active: 1 + len(rest),
	}

	go g.waitFor(initial)
	for _, r := range rest {
		go g.waitFor(r)
	}

	return g
}

type Group struct {
	// The new context.
	C context.Context

	cancel func() // internal cancel for the new ctx

	lock   sync.Mutex
	done   bool
	active int
}

func (g *Group) IsDone() bool {
	select {
	case <-g.C.Done():
		return true
	default:
		return false
	}
}

func (g *Group) waitFor(ctx context.Context) {
	<-ctx.Done()

	g.lock.Lock()
	defer g.lock.Unlock()

	g.active--
	if g.active == 0 {
		g.cancel()
		g.done = true
	}
}

func (g *Group) Add(ctx context.Context) bool {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.done {
		return false
	}

	g.active++
	go g.waitFor(ctx)
	return true
}
