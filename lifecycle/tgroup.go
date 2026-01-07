package lifecycle

import (
	"context"
	"sync"
)

// TGroup provides [context.Context] if tokens pass check functions.
type TGroup[T comparable] interface {
	// Provide provides a context tagged with a unique T.
	// Calling Provide with the same T unions the lifecycle of the context.
	Provide(token T, ctx context.Context) (ok bool)

	// Revoke revokes a prior T early, ignoring its [context.Context].
	Revoke(token T) (ok bool)

	// Access provides a derived context that is valid while any T passes the given check, and has a valid context.
	Access(parent context.Context, check Check[T]) (derived context.Context)
}

// Check checks the given token.
// If the func is nil, treat as returning nil err.
type Check[T comparable] func(ctx context.Context, token T) (err error)

// TGroup creates a new TGroup for the given T.
func NewTGroup[T comparable]() (t TGroup[T]) {
	return &tgroup[T]{
		groups:   map[T]CGroup{},
		revokeCh: make(chan struct{}),
	}
}

type tgroup[T comparable] struct {
	lock     sync.RWMutex
	groups   map[T]CGroup
	revokeCh chan struct{}
}

func (t *tgroup[T]) Revoke(token T) (ok bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.internalRevoke(token)
}

func (t *tgroup[T]) Provide(token T, ctx context.Context) (ok bool) {
	if IsDone(ctx) {
		return // already expired, don't bother
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	group := t.groups[token]
	if group != nil {
		if !group.Add(ctx) {
			// should never happen, because Add() will trigger resume below
			panic("couldn't add to group inside lock section")
		}
		return true
	}

	group = NewCGroup()
	group.Add(ctx)
	group.Start()
	t.groups[token] = group

	group.Halt(func(c context.Context, resume <-chan struct{}) (err error) {
		t.lock.Lock()
		defer t.lock.Unlock()

		select {
		case <-resume:
			return nil
		default:
		}

		t.internalRevoke(token)
		return nil
	})

	return true
}

// internalRevoke must be called under rw lock.
func (t *tgroup[T]) internalRevoke(token T) (ok bool) {
	if g := t.groups[token]; g == nil {
		return false
	}

	delete(t.groups, token)

	// force everyone to re-run
	close(t.revokeCh)
	t.revokeCh = make(chan struct{})

	return true
}

func (t *tgroup[T]) internalCheck(ctx context.Context, check Check[T]) (err error) {
	err = context.Canceled

	for t := range t.groups {
		if check == nil {
			return nil // has at least one valid token
		}
		err = check(ctx, t)
		if err == nil {
			return nil // something passed
		}
	}

	return err
}

func (t *tgroup[T]) Access(parent context.Context, check Check[T]) (derived context.Context) {
	if IsDone(parent) {
		return parent
	}

	var cancel context.CancelCauseFunc
	derived, cancel = context.WithCancelCause(parent)

	t.lock.RLock()
	defer t.lock.RUnlock()

	err := t.internalCheck(parent, check)
	if err != nil {
		cancel(err) // never was ok, cancel immediately before return
		return
	}

	// ok, listen to current revokeCh until no longer valid
	revokeCh := t.revokeCh

	go func() {
		for err == nil {
			<-revokeCh

			t.lock.RLock()
			revokeCh = t.revokeCh
			err = t.internalCheck(parent, check)
			t.lock.RUnlock()
		}
		cancel(err)
	}()

	return
}
