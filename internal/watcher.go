package internal

import (
	"context"
	"log"
	"net/http"
	"sync"
)

// serverWatcher exists because *http.Server is bad at shutting down websockets/hijacked connections.
// It's not exposed publicly.
type serverWatcher struct {
	lock     sync.Mutex
	byServer map[*http.Server]map[*context.CancelFunc]struct{}
}

var (
	sw serverWatcher
)

// RegisterHttpContext returns a derived context that is cancelled once the server shuts down.
// It's needed only for hijacked connections.
func RegisterHttpContext(ctx context.Context) context.Context {
	return sw.RegisterHttpContext(ctx)
}

// RegisterHttpContext returns a derived context that is cancelled once the server shuts down.
// It's needed only for hijacked connections.
func (sw *serverWatcher) RegisterHttpContext(ctx context.Context) context.Context {
	prev := ctx.Value(sw)
	if prev != nil {
		return ctx
	}

	server, ok := ctx.Value(http.ServerContextKey).(*http.Server)
	if !ok {
		return ctx
	}

	sw.lock.Lock()
	defer sw.lock.Unlock()

	ok = false
	var closers map[*context.CancelFunc]struct{}
	if sw.byServer == nil {
		sw.byServer = make(map[*http.Server]map[*context.CancelFunc]struct{})
	} else {
		closers, ok = sw.byServer[server]
	}
	if !ok {
		closers = make(map[*context.CancelFunc]struct{})
		sw.byServer[server] = closers

		// haven't seen this server before, register shutdown func
		server.RegisterOnShutdown(func() {
			sw.lock.Lock()
			defer sw.lock.Unlock()

			// only log if we had any active
			if len(closers) != 0 {
				log.Printf("server shutdown killing %d active websocket", len(closers))
				for closer := range closers {
					(*closer)()
				}
			}
			delete(sw.byServer, server)
		})
	}

	ctx, cancel := context.WithCancel(ctx)
	closers[&cancel] = struct{}{}

	context.AfterFunc(ctx, func() {
		// when context is done (normally or not), cleanup closer
		sw.lock.Lock()
		defer sw.lock.Unlock()
		delete(closers, &cancel)
	})

	return context.WithValue(ctx, sw, true)
}
