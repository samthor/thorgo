package lgroup

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type nonceKey struct {
	_ int
}

// NewLGroup creates a new LGroup which manages the lifecycles of contexts and helper functions over them.
// It calls the passed cancel function when complete.
// The caller must eventually call start(); to do otherwise leaks.
// The start() method allows setup/join before kickoff, that way the LGroup doesn't start dead.
func NewLGroup[Init any](cancel context.CancelCauseFunc) (group LGroup[Init], start func()) {
	startNonce := &nonceKey{}
	startCh := make(chan struct{})

	lg := &lgroup[Init]{
		active: map[*nonceKey]*activeContext[Init]{
			startNonce: {
				activeHandlers: atomic.Int32{},
			},
		},
		cancel:  cancel,
		startCh: startCh,
		doneCh:  make(chan struct{}),
	}

	// use our internal mechanisms to _at least_ block until each context is done
	lg.Register(func(ctx context.Context, i Init) error {
		<-ctx.Done()
		return nil
	})

	return lg, func() {
		lg.lock.Lock()
		defer lg.lock.Unlock()

		close(startCh)

		if startNonce != nil {
			lg.releaseActive(startNonce)
			startNonce = nil
		}
	}
}

type activeContext[Init any] struct {
	C              context.Context
	init           Init
	activeHandlers atomic.Int32
}

// run must be run under lock
func (lg *lgroup[Init]) run(nonce *nonceKey, ac *activeContext[Init], handler func(context.Context, Init) error) {
	ac.activeHandlers.Add(1)

	go func() {
		<-lg.startCh // caller must trigger start()
		err := handler(ac.C, ac.init)
		if err != nil {
			lg.cancel(err)
		}

		lg.lock.Lock()
		defer lg.lock.Unlock()

		out := ac.activeHandlers.Add(-1)
		if out < 0 {
			panic("got -ve activeHandlers")
		} else if out != 0 {
			return
		}

		lg.releaseActive(nonce)
	}()
}

type lgroup[Init any] struct {
	startCh <-chan struct{}
	doneCh  chan struct{}
	cancel  context.CancelCauseFunc

	lock     sync.Mutex
	active   map[*nonceKey]*activeContext[Init]
	handlers []func(context.Context, Init) error
}

func (lg *lgroup[Init]) Join(ctx context.Context, init Init) bool {
	lg.lock.Lock()
	defer lg.lock.Unlock()

	select {
	case <-lg.doneCh:
		return false
	default:
	}

	nonce := &nonceKey{}

	ac := &activeContext[Init]{
		C:    ctx,
		init: init,
	}
	lg.active[nonce] = ac

	if len(lg.handlers) == 0 {
		panic("lgroup internal handler missing?")
	}

	for _, fn := range lg.handlers {
		lg.run(nonce, ac, fn)
	}
	return true
}

func (lg *lgroup[Init]) Register(fn func(context.Context, Init) error) {
	lg.lock.Lock()
	defer lg.lock.Unlock()

	lg.handlers = append(lg.handlers, fn)

	for nonce, ac := range lg.active {
		if ac.C == nil {
			continue // this is our start nonce
		}
		lg.run(nonce, ac, fn)
	}
}

func (lg *lgroup[Init]) Done() <-chan struct{} {
	return lg.doneCh
}

// must be held under lock
func (lg *lgroup[Init]) releaseActive(nonce *nonceKey) {
	prior, ok := lg.active[nonce]
	if !ok || prior.activeHandlers.Load() != 0 {
		panic(fmt.Sprintf("missing prior active nonce or invalid activeHandlers: prior=%+v", prior))
	}

	delete(lg.active, nonce)
	if len(lg.active) == 0 {
		close(lg.doneCh)
	}
}
