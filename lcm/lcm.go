package lcm

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/samthor/thorgo/future"
)

// New returns a new Manager that manages the lifecycle of lazily-created objects.
// The passed initial context should be context.Background, as it is used as the parent of all lazily-created objects.
func New[Key comparable, Object any](
	ctx context.Context,
	build BuildFn[Key, Object],
) Manager[Key, Object] {
	return &managerImpl[Key, Object]{
		ctx:       ctx,
		build:     build,
		connected: map[Key]*managerInfo[Object]{},
	}
}

type managerImpl[Key comparable, Object any] struct {
	ctx             context.Context
	build           BuildFn[Key, Object]
	shutdownTimeout time.Duration

	lock      sync.Mutex
	connected map[Key]*managerInfo[Object]
}

type managerInfo[Object any] struct {
	future future.Future[Object]
	ctx    context.Context

	gt         GroupTimer
	shutdownCh <-chan struct{} // when the thing is actually dead dead
}

func (m *managerImpl[Key, Object]) SetTimeout(d time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.shutdownTimeout = d
}

func (m *managerImpl[Key, Object]) Run(ctx context.Context, key Key) (Object, context.Context, error) {
	retryTime := time.Millisecond

retry:
	m.lock.Lock()
	info, ok := m.connected[key]
	if !ok {
		info = m.internalRun(ctx, key)
		m.connected[key] = info
	} else {
		var waitForShutdown bool

		select {
		case <-info.ctx.Done():
			waitForShutdown = true // runCtx is done but still in map: doing shutdown
		default:
			if !info.gt.Join(ctx) {
				waitForShutdown = true // timer expired but still in map: doing shutdown
			}
		}

		if waitForShutdown {
			m.lock.Unlock()

			select {
			case <-ctx.Done():
				// caller expired while waiting
				var out Object
				return out, nil, ctx.Err()
			case <-info.shutdownCh:
			}

			select {
			case <-ctx.Done():
				// caller expired while blocking for retry
			case <-time.NewTicker(retryTime).C:
			}

			retryTime *= 2
			goto retry
		}
	}
	m.lock.Unlock()

	out, err := info.future.Wait(ctx)
	return out, info.ctx, err
}

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

	go func() {
		out, err := m.build(Build[Key]{
			Key:    key,
			C:      runCtx,
			Cancel: cancel,
		})
		log.Printf("prepare key=%+v done, err=%v", key, err)
		if err != nil {
			cancel(err) // could not even create self (cancel ctx before resolve)
		}
		resolve(out, err)

		if err == nil {
			<-gt.Done()
			log.Printf("done key=%+v err=%+v", key, runCtx.Err()) // err may be nil here

			hasShutdownLog := "no"
			var err error

			if hs, ok := any(out).(HasShutdownError); ok {
				err = hs.Shutdown()
				hasShutdownLog = "yes"
			} else if hs, ok := any(out).(HasShutdown); ok {
				hs.Shutdown()
				hasShutdownLog = "yes"
			}

			cancel(err)
			log.Printf("shutdown(%s) key=%+v err=%+v", hasShutdownLog, key, err) // err may be nil here, but the ctx is cancelled
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
