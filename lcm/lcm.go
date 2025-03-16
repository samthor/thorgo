package lcm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/samthor/thorgo/future"
	"github.com/samthor/thorgo/lgroup"
	"golang.org/x/sync/errgroup"
)

var (
	lcmLogs = false
)

// EnableLogs turns on logs for some things in this package.
// Just for debugging.
func EnableLogs() {
	lcmLogs = true
}

func lcmLog(key any, format string, args ...any) {
	if lcmLogs {
		log.Printf("[%s] "+format, key, args)
	}
}

// New returns a new Manager that manages the lifecycle of lazily-created objects.
func New[Key comparable, Object, Init any](
	build BuildFunc[Key, Object, Init],
	options ...Option,
) Manager[Key, Object, Init] {
	return NewWithContext(context.Background(), build, options...)
}

// New returns a new Manager that manages the lifecycle of lazily-created objects.
// The passed initial context should normally be context.Background, as it is used as the parent of all lazily-created objects.
// It could be something else if you wanted to be able to cancel all objects at once.
func NewWithContext[Key comparable, Object, Init any](
	ctx context.Context,
	build BuildFunc[Key, Object, Init],
	options ...Option,
) Manager[Key, Object, Init] {

	// apply options (just joinTask and timeout for now)
	for _, o := range options {
		if o.joinTask != nil {
			actualBuild := build
			build = func(k Key, s Status[Init]) (Object, error) {
				s.JoinTask(func(ctx context.Context, shutdownCh <-chan bool, i Init) error {
					return o.joinTask(ctx, shutdownCh)
				})
				return actualBuild(k, s)
			}
		}
	}

	return &managerImpl[Key, Object, Init]{
		ctx:       ctx,
		build:     build,
		connected: map[Key]*managerInfo[Object, Init]{},
	}
}

type managerImpl[Key comparable, Object, Init any] struct {
	ctx             context.Context
	build           BuildFunc[Key, Object, Init]
	shutdownTimeout time.Duration

	lock      sync.Mutex
	connected map[Key]*managerInfo[Object, Init]
}

type managerInfo[Object, Init any] struct {
	future     future.Future[Object]
	ctx        context.Context
	shutdownCh <-chan struct{} // when the thing is actually dead dead
	start      func()
	status     *statusImpl[Init]
}

func (m *managerImpl[Key, Object, Init]) SetTimeout(d time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.shutdownTimeout = d
}

func (m *managerImpl[Key, Object, Init]) Run(ctx context.Context, key Key, init Init) (Object, context.Context, error) {
	var info *managerInfo[Object, Init]

	for {
		m.lock.Lock()
		var ok bool
		info, ok = m.connected[key]
		if !ok {
			info = m.internalRun(key)
			info.status.lg.Join(ctx, init) // brand new, join immediately
			defer info.start()             // must always release lgroup
			m.lock.Unlock()
			break
		}

		var waitForShutdown bool
		select {
		case <-info.ctx.Done():
			waitForShutdown = true // runCtx is done but still in map: doing shutdown
		default:
			if !info.status.lg.Join(ctx, init) {
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
			// we waited for the thing to shut down; we can try creating it anew
		}
	}

	out, err := info.future.Wait(ctx)
	return out, info.ctx, err
}

// internalRun sets up the managerInfo for the given key task.
// It must be called under lock.
func (m *managerImpl[Key, Object, Init]) internalRun(key Key) *managerInfo[Object, Init] {
	lcmLog(key, "preparing...")
	f, resolve := future.New[Object]()

	runCtx, cancel := context.WithCancelCause(m.ctx)

	lg, start := lgroup.NewLGroup[Init](cancel)

	shutdownCh := make(chan struct{})
	info := &managerInfo[Object, Init]{
		ctx:        runCtx,
		future:     f,
		shutdownCh: shutdownCh,
		start:      start,
		status: &statusImpl[Init]{
			ctx:    runCtx,
			cancel: cancel,
			lg:     lg,
		},
	}
	m.connected[key] = info

	lgroupDone := info.status.lg.Done()

	go func() {
		out, err := m.build(key, info.status)
		lcmLog(key, "prepare done, err=%v", err)
		if err != nil {
			cancel(err) // could not even create self (cancel ctx before resolve)
		}
		resolve(out, err) // this will allow folks to join

		if err == nil {
			info.status.startTasks()

			select {
			case <-lgroupDone:
				// timeout because users bailed
			case <-runCtx.Done():
				// run context was cancelled:
				//   1. the parent context died (unlikely unless called with something other than context.Background)
				//   2. the runnable object killed itself
				err = context.Cause(runCtx)
			}

			// signal tasks; wait for all to be done
			close(info.status.taskCh)
			info.status.taskGroup.Wait()

			if err == nil {
				lcmLog(key, "clean stop, running afterTasks")
				err = info.status.runAfter()
			}

			cancel(err) // make sure run context is dead now, even with nil err
			lcmLog(key, "shutdown, err=%v", err)
		}

		// delete ourselves from map
		m.lock.Lock()
		defer m.lock.Unlock()
		close(shutdownCh) // under lock (this makes close/delete 'atomic')
		lcmLog(key, "cleanup")
		delete(m.connected, key)
	}()

	return info
}

type statusImpl[Init any] struct {
	ctx    context.Context
	cancel context.CancelCauseFunc

	lg lgroup.LGroup[Init]

	taskLock  sync.Mutex
	tasks     []TaskFunc
	taskCh    chan struct{}
	taskGroup errgroup.Group

	lock  sync.Mutex
	after []*afterStatus
}

type afterStatus struct {
	once sync.Once // either starts running f or stops f from running
	fn   func() error
}

// runAfter runs all tasks on this instance of Status, stopping early if any return a non-nil error.
// Tasks may continue to be added during this, in which case they'll also be run.
func (s *statusImpl[Init]) runAfter() error {
	var i int
	for {
		s.lock.Lock()
		if i == len(s.after) {
			s.lock.Unlock()
			break
		}

		next := s.after[i]
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

func (s *statusImpl[Init]) Context() context.Context {
	return s.ctx
}

func (s *statusImpl[Init]) startTasks() {
	s.taskLock.Lock()
	defer s.taskLock.Unlock()

	taskCh := make(chan struct{})
	s.taskCh = taskCh

	for _, t := range s.tasks {
		s.alwaysStartTask(t)
	}
	s.tasks = nil
}

func (s *statusImpl[Init]) alwaysStartTask(fn TaskFunc) {
	s.taskGroup.Go(func() error {
		err := fn(s.taskCh)
		if err != nil {
			s.cancel(err)
		}
		return err
	})
}

func (s *statusImpl[Init]) Task(fn TaskFunc) {
	s.taskLock.Lock()
	defer s.taskLock.Unlock()

	if s.taskCh == nil {
		s.tasks = append(s.tasks, fn)
		return
	}

	select {
	case <-s.taskCh:
		return
	default:
	}
	s.alwaysStartTask(fn)
}

func (s *statusImpl[Init]) After(fn func() error) (stop func() bool) {
	a := &afterStatus{fn: fn}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.after = append(s.after, a)

	return func() (stopped bool) {
		a.once.Do(func() { stopped = true })
		if stopped {
			a.fn = nil // remove ref (GC)
		}
		return stopped
	}
}

func (s *statusImpl[Init]) JoinTask(fn func(context.Context, <-chan bool, Init) error) {
	s.lg.Register(func(ctx context.Context, i Init) error {
		select {
		case <-s.Context().Done():
			return nil // don't start if we cancelled (e.g., thing failed to create)
		default:
		}

		shutdownCh := make(chan bool, 1)

		go func() {
			select {
			case <-s.Context().Done():
				// the outer ctx shut down first (basically: error), just close
			case <-ctx.Done():
				shutdownCh <- true   // the user ctx shut down (normal operation)
				<-s.Context().Done() // now wait for outer ctx
			}
			close(shutdownCh)
		}()

		return fn(ctx, shutdownCh, i)
	})
}

func (s *statusImpl[Init]) Check(err error) error {
	if err != nil {
		s.cancel(err)
	}
	return err
}

func (s *statusImpl[Init]) CheckWrap(str string, err error) error {
	if err == nil {
		return nil
	}
	return s.Check(fmt.Errorf("%s: %w", str, err))
}
