package doc

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/samthor/thorgo/lifecycle"
	"github.com/samthor/thorgo/queue"
)

// New creates a new Holder, which is a keyed storage of instance objects that have lifetimes based on their users.
func New[K comparable, V any](config Config[K, V]) Holder[K, V] {
	return &holderImpl[K, V]{
		config:   config,
		active:   map[K]*activeDoc[V]{},
		activity: queue.New[internalActivity[K]](),
	}
}

type internalActivity[K comparable] struct {
	key   K
	start bool
}

type holderImpl[K comparable, V any] struct {
	config Config[K, V]

	lock     sync.RWMutex
	active   map[K]*activeDoc[V]
	activity queue.Queue[internalActivity[K]]
}

func (h *holderImpl[K, V]) For(ctx context.Context, key K) (inst V, done <-chan error, err error) {
retry:
	// Hold lock, check status, retry if needed until we get a valid document.
	h.lock.Lock()
	active := h.active[key]
	failures := 0

	if active != nil {
		select {
		case <-active.halted:
			// The doc is halted so remove it from the active set.
			// This is basically "active=nil".
			failures = active.failures
			delete(h.active, key)
			active = nil

		default:
		}
	}

	if active != nil {
		h.lock.Unlock()

		select {
		case <-ctx.Done():
			err = context.Cause(ctx)
			return

		case <-active.halting:
			// The doc is halting (or already halted), wait it for it to be halt*ed* and try again.
			select {
			case <-active.halted:
				goto retry
			case <-ctx.Done():
				err = context.Cause(ctx)
				return
			}

		default:
		}

		// The doc is already ready, or is being built by someone else.
		// Wait for any possible state, ideally ready.
		select {
		case <-active.ready:
			// The doc is ready, so join it.
			// Since we're outside the lock, it's possible for the group to be done / document halted.
			// Take the lock and be sure we're ok, otherwise retry.
			h.lock.Lock()
			ok := active.group.Add(ctx)
			h.lock.Unlock()

			if ok {
				doneCh := make(chan error, 1)
				go func() {
					<-active.halting
					doneCh <- active.err
				}()
				return active.inst, doneCh, nil
			}

		case <-ctx.Done():
			err = context.Cause(ctx)
			return

		case <-active.halting:
		case <-active.halted:
		}

		goto retry
	}

	// No active document, take ownership.
	active = &activeDoc[V]{
		ready:    make(chan struct{}),
		halting:  make(chan struct{}),
		halted:   make(chan struct{}),
		failures: failures,
	}
	h.active[key] = active
	h.lock.Unlock()

	// Delay by a reasonable exponential backoff.
	select {
	case <-ctx.Done():
		close(active.halting)
		close(active.halted)
		err = context.Cause(ctx)
		return
	case <-time.After(time.Duration(failures * failures * int(time.Millisecond))):
	}

	// Shutdown is called either via Halt() or via the inst-initiated shutdown.
	// It must be called under lock.
	shutdown := func(wrap func()) {
		select {
		case <-active.ready:
			h.activity.Push(internalActivity[K]{key: key, start: false})
		default:
		}

		select {
		case <-active.halting:
		default:
			close(active.halting)
			defer close(active.halted)
		}
		h.lock.Unlock()
		if wrap != nil {
			wrap()
		}
	}

	active.group = lifecycle.NewCGroup()
	active.group.Halt(func(ctx context.Context, resume <-chan struct{}) (err error) {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(h.config.ShutdownDelay):
		}

		h.lock.Lock()

		// Check under lock if we were resumed by another context joining.
		select {
		case <-resume:
			h.lock.Unlock()
			return nil
		default:
		}

		shutdown(func() {
			err = h.config.Destroy(ctx, key, active.inst)
		})
		return
	})

	active.group.Add(ctx)
	groupCtx := active.group.Start()
	instCtx, instCancel := context.WithCancelCause(groupCtx)

	// We didn't have to wait, so that means we get to create it!
	createReturned := false
	var earlyCancelError error
	cancel := func(cause error) {
		instCancel(cause) // cancel "ourselves"

		h.lock.Lock()
		if !createReturned {
			earlyCancelError = cause
			h.lock.Unlock()
			return
		}

		if active.err == nil {
			active.err = cause
		}
		shutdown(nil)
	}
	inst, err = h.config.Create(instCtx, cancel, key)

	h.lock.Lock()
	defer h.lock.Unlock()

	if err == nil && earlyCancelError != nil {
		err = earlyCancelError
	}
	createReturned = true

	if err != nil {
		select {
		case <-active.halting:
			// The group was shut down by the halt or by an early cancel.
			// Unlikely but check before double-closing.
		default:
			active.failures++
			close(active.halting)
			close(active.halted)
		}
		return
	}

	active.inst = inst
	h.activity.Push(internalActivity[K]{key: key, start: true})

	active.failures = 0
	close(active.ready)

	doneCh := make(chan error, 1)
	go func() {
		<-active.halting
		doneCh <- active.err
	}()

	return active.inst, doneCh, nil
}

func (h *holderImpl[K, V]) Active(ctx context.Context, filter func(key K) (include bool)) (i iter.Seq[Action[K]]) {
	if filter == nil {
		filter = func(k K) (include bool) { return true }
	}

	h.lock.Lock()
	defer h.lock.Unlock()

	init := map[K]bool{}
	for k := range h.active {
		if filter(k) {
			init[k] = true
		}
	}

	l := h.activity.Join(ctx)

	return func(yield func(Action[K]) bool) {
		if lifecycle.IsDone(ctx) || !yield(init) {
			return
		}

		for next := range l.BatchIter() {
			out := map[K]bool{}
			for _, act := range next {
				if !filter(act.key) {
					continue
				}

				was, exists := out[act.key]
				if !exists {
					out[act.key] = act.start
				} else if was == act.start {
					panic("start/stop must flip-flop")
				} else {
					delete(out, act.key)
				}
			}

			if len(out) != 0 && !yield(out) {
				return
			}
		}
	}
}

type activeDoc[V any] struct {
	inst     V
	group    lifecycle.CGroup
	failures int // for exp. backoff

	err error // controlled error from inst

	ready   chan struct{} // if closed, doc is ready
	halting chan struct{} // if closed, doc is shutting down and should no longer be used
	halted  chan struct{} // if closed, may be recreated
}
