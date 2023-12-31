package context

import (
	"context"
	"sync"
	"time"
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

// NewTimeoutGroup creates a new ContextGroup that, when it has no active contexts, expires after the given duration.
// If the duration is zero/low, this may be expired by the time the caller returns.
func NewTimeoutGroup(d time.Duration, rest ...context.Context) *Group {
	ctx, cancel := context.WithCancel(context.Background())

	g := &Group{
		C:       ctx,
		cancel:  cancel,
		active:  len(rest),
		timeout: d,
	}

	for _, r := range rest {
		go g.waitFor(r)
	}
	g.maybeQueueTimeout()

	return g
}

type Group struct {
	// The new context.
	C context.Context

	cancel func() // internal cancel for the new ctx

	lock    sync.Mutex
	done    bool
	active  int
	timeout time.Duration

	stopTimer func() bool
	seq       int
}

// maybeQueueTimeut must be called under lock.
func (g *Group) maybeQueueTimeout() bool {
	if g.active > 0 {
		return false
	} else if g.active < 0 {
		panic("-ve active?")
	}

	localSeq := g.seq
	timer := time.AfterFunc(g.timeout, func() {
		g.lock.Lock()
		defer g.lock.Unlock()

		if localSeq != g.seq || g.active > 0 {
			return
		}

		g.cancel()
		g.done = true
	})
	g.stopTimer = timer.Stop
	return true
}

// IsDone immediately returns whether this group is done.
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
	g.maybeQueueTimeout()
}

// Add adds this context to the group.
// Returns true if this was successful, and false if the group is already complete.
func (g *Group) Add(ctx context.Context) bool {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.done {
		return false
	}

	g.seq++
	if g.stopTimer != nil {
		g.stopTimer()
		g.stopTimer = nil
	}

	g.active++
	go g.waitFor(ctx)
	return true
}
