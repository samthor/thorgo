package lcm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/samthor/thorgo/future"
)

// New returns a new Manager that manages the lifecycle of lazily-created objects.
func New[Key comparable, Object any](build BuildFunc[Key, Object]) Manager[Key, Object] {
	return NewWithContext(context.Background(), build)
}

// New returns a new Manager that manages the lifecycle of lazily-created objects.
// The passed initial context should normally be context.Background, as it is used as the parent of all lazily-created objects.
// It could be something else if you wanted to be able to cancel all objects at once.
func NewWithContext[Key comparable, Object any](ctx context.Context, build BuildFunc[Key, Object]) Manager[Key, Object] {
	return &managerImpl[Key, Object]{
		ctx:       ctx,
		build:     build,
		connected: map[Key]*managerInfo[Object]{},
	}
}

type managerImpl[Key comparable, Object any] struct {
	ctx             context.Context
	build           BuildFunc[Key, Object]
	shutdownTimeout time.Duration

	lock      sync.Mutex
	connected map[Key]*managerInfo[Object]
}

type managerInfo[Object any] struct {
	future     future.Future[Object]
	ctx        context.Context
	gt         GroupTimer
	shutdownCh <-chan struct{} // when the thing is actually dead dead
}

func (m *managerImpl[Key, Object]) SetTimeout(d time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.shutdownTimeout = d
}

func (m *managerImpl[Key, Object]) Run(ctx context.Context, key Key) (Object, context.Context, error) {
	var info *managerInfo[Object]

	for {
		m.lock.Lock()
		var ok bool
		info, ok = m.connected[key]
		if !ok {
			info = m.internalRun(ctx, key)
			m.lock.Unlock()
			break
		}

		var waitForShutdown bool
		select {
		case <-info.ctx.Done():
			waitForShutdown = true // runCtx is done but still in map: doing shutdown
		default:
			if !info.gt.Join(ctx) {
				waitForShutdown = true // timer expired but still in map: doing shutdown
			}
		}
		m.lock.Unlock()

		if !waitForShutdown {
			break // ok
		}

		select {
		case <-ctx.Done():
			// caller expired while waiting
			var out Object
			return out, nil, ctx.Err()
		case <-info.shutdownCh:
		}
	}

	out, err := info.future.Wait(ctx)
	return out, info.ctx, err
}

// internalRun sets up the managerInfo for the given key task.
// It must be called under lock.
func (m *managerImpl[Key, Object]) internalRun(ctx context.Context, key Key) *managerInfo[Object] {
	log.Printf("preparing key=%+v...", key)
	f, resolve := future.New[Object]()

	runCtx, cancel := context.WithCancelCause(m.ctx)

	gt := NewGroupTimer(m.shutdownTimeout, ctx)
	shutdownCh := make(chan struct{})
	info := &managerInfo[Object]{
		ctx:        runCtx,
		future:     f,
		gt:         gt,
		shutdownCh: shutdownCh,
	}
	m.connected[key] = info

	go func() {
		s := &statusImpl{ctx: runCtx, cancel: cancel}
		out, err := m.build(key, s)
		log.Printf("prepare key=%+v done, err=%v", key, err)
		if err != nil {
			cancel(err) // could not even create self (cancel ctx before resolve)
		}
		resolve(out, err)

		if err == nil {
			select {
			case <-gt.Done():
				// timeout because users bailed
			case <-runCtx.Done():
				// run context was cancelled:
				//   1. the parent context died (unlikely unless called with something other than context.Background)
				//   2. the runnable object killed itself
				err = context.Cause(runCtx)
			}
			log.Printf("done key=%+v err=%+v", key, err) // err may be nil here

			if err == nil {
				err = s.runAfterTasks()
			}

			cancel(err)
			log.Printf("shutdown key=%+v err=%+v", key, err) // err may be nil here, but the ctx is cancelled
		}

		// delete ourselves from map
		m.lock.Lock()
		defer m.lock.Unlock()
		close(shutdownCh) // under lock (this makes close/delete 'atomic')
		log.Printf("cleanup key=%+v", key)
		delete(m.connected, key)
	}()

	return info
}

type statusImpl struct {
	ctx    context.Context
	cancel context.CancelCauseFunc

	lock       sync.Mutex
	afterTasks []*afterStatus
}

type afterStatus struct {
	once sync.Once // either starts running f or stops f from running
	fn   func() error
}

// runAfterTasks runs all tasks on this instance of Status, stopping early if any return a non-nil error.
// Tasks may continue to be added during this, in which case they'll also be run.
func (s *statusImpl) runAfterTasks() error {
	var i int
	for {
		s.lock.Lock()
		if i == len(s.afterTasks) {
			s.lock.Unlock()
			break
		}

		next := s.afterTasks[i]
		s.lock.Unlock()
		i++

		var err error
		next.once.Do(func() {
			err = next.fn()
		})

		if err != nil {
			return err // bail early
		}
	}

	return nil
}

func (s *statusImpl) Context() context.Context {
	return s.ctx
}

func (s *statusImpl) After(fn func() error) (stop func() bool) {
	a := &afterStatus{fn: fn}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.afterTasks = append(s.afterTasks, a)

	return func() (stopped bool) {
		a.once.Do(func() { stopped = true })
		if stopped {
			// TODO: remove ref (GC)
		}
		return stopped
	}
}

func (s *statusImpl) Check(err error) error {
	if err != nil {
		s.cancel(err)
	}
	return err
}

func (s *statusImpl) CheckWrap(str string, err error) error {
	if err == nil {
		return nil
	}
	return s.Check(fmt.Errorf("%s: %w", str, err))
}
