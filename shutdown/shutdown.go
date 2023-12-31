package shutdown

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// LazyShutdown basically provides a context and HTTP server helpers which allow shutdown after inactivity.
// This can be useful for hosting services whose running binary should naturally evict itself after some time.
type LazyShutdown struct {
	lock   sync.RWMutex
	timer  *time.Timer
	wait   time.Duration
	doneCh chan struct{}
	reason error
	active int64
}

// New builds a new LazyShutdown.
func New(wait time.Duration) *LazyShutdown {
	ls := &LazyShutdown{
		wait:   wait,
		timer:  time.NewTimer(wait),
		doneCh: make(chan struct{}),
	}

	go func() {
		// This is a bit racey: the timer might be reset after already closed.
		// But if it has, then even if new locks/requests are happening, this LazyShutdown is already considered to be dying, doneCh will be closed.
		<-ls.timer.C
		ls.lock.Lock()
		defer ls.lock.Unlock()
		close(ls.doneCh)
	}()

	return ls
}

func (ls *LazyShutdown) addLock(delta int64) {
	select {
	case <-ls.doneCh:
		return // nothing to do, server closed
	default:
	}

	ls.lock.Lock()
	defer ls.lock.Unlock()
	ls.timer.Stop()

	if delta < -1 || delta > 1 {
		panic("bad delta")
	}
	ls.active += delta
	if ls.active < 0 {
		panic("-ve active")
	}

	for {
		select {
		case <-ls.timer.C:
		default:
			if ls.active == 0 {
				ls.timer.Reset(ls.wait)
			}
			return
		}
	}
}

// Serve servers HTTP until the LazyShutdown shuts down. Active requests do not explicitly cause it
// to stay alive, only requests wrapped by ServeWrap.
func (ls *LazyShutdown) Serve(addr string, handler http.Handler) error {
	server := &http.Server{Addr: addr, Handler: handler}

	go func() {
		<-ls.Done()
		// Doing both is important. Close() is aggressive, shutting down _now_.
		// Shutdown() properly fires the RegisterOnShutdown handlers, but waits for connections to stop.
		server.Close()
		server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}

// ServeWrap is as Serve, but ensures that _all_ requests handled by this server prevent shutdown.
func (ls *LazyShutdown) ServeWrap(addr string, handler http.Handler) error {
	if handler == nil {
		handler = http.DefaultServeMux
	}

	var h http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		ls.Lock()
		defer ls.Unlock()
		handler.ServeHTTP(w, r)
	}
	return ls.Serve(addr, h)
}

// Err shuts down when an error is passed here.
// This is useful for ListenAndServe, which always returns an error.
func (ls *LazyShutdown) Err(err error) {
	if err != nil {
		close(ls.doneCh)
		ls.reason = err
	}
}

// Reason returns any previously recorded shutdown error via Err().
func (ls *LazyShutdown) Reason() error {
	return ls.reason
}

// Done returns a channel which closes when this LazyShutdown has timed out.
func (ls *LazyShutdown) Done() <-chan struct{} {
	return ls.doneCh
}

// IsDone immediately returns whether this is done.
func (ls *LazyShutdown) IsDone() bool {
	select {
	case <-ls.doneCh:
		return true
	default:
		return false
	}
}

// Reset resets the timer on this LazyShutdown only if is free to fire.
func (ls *LazyShutdown) Reset() {
	ls.addLock(0)
}

// Lock prevents this LazyShutdown from firing. This acts like a `sync.RWMutex`.
func (ls *LazyShutdown) Lock() {
	ls.addLock(1)
}

// Unlock allows this LazyShutdown to fire. It is a fatal error to unlock more than lock.
func (ls *LazyShutdown) Unlock() {
	ls.addLock(-1)
}

// WaitDuration returns the original setup duration.
func (ls *LazyShutdown) WaitDuration() time.Duration {
	return ls.wait
}

// WrapFunc wraps a http.HandlerFunc such that this LazyShutdown will not close while it is active.
func (ls *LazyShutdown) WrapFunc(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ls.Lock()
		defer ls.Unlock()
		fn(w, r)
	}
}

// Wrap wraps a http.Handler such that this LazyShutdown will not close while it is active.
func (ls *LazyShutdown) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ls.Lock()
		defer ls.Unlock()
		h.ServeHTTP(w, r)
	})
}
