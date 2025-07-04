package sock

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/samthor/thorgo/transport"
)

// IsRequest returns whether this is probably a WebSocket request.
func IsRequest(r *http.Request) bool {
	h := r.Header

	return h.Get("Upgrade") == "websocket"
}

// Transport returns a http.HandlerFunc that wraps a websocket setup/teardown into a [transport.Transport] which sends and receives JSON packets.
// The [transport.Transport] provides a context which represents the socket; use its own context only if you're fine with waiting forever while the connection is open.
// The handler should return a [websocket.CloseError] to emit a specfic state to the caller.
func Transport(fn func(t transport.Transport) error, options *websocket.AcceptOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sock, err := websocket.Accept(w, r, options)
		if err != nil {
			log.Printf("got err setting up websocket %s: %v", r.URL.Path, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithCancelCause(context.Background())

		context.AfterFunc(ctx, func() {
			err := context.Cause(ctx) // this will be err from parent context

			var closeError websocket.CloseError
			if errors.As(err, &closeError) {
				sock.Close(closeError.Code, closeError.Reason)
			} else if err != nil && err != context.Canceled {
				sock.Close(websocket.StatusInternalError, "")
			} else {
				sock.Close(websocket.StatusNormalClosure, "")
			}
		})

		t := &socketTransport{ctx: ctx, cancel: cancel, sock: sock}
		cancel(fn(t))
	}
}

type socketTransport struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	sock   *websocket.Conn
}

func (s *socketTransport) Context() context.Context {
	return s.ctx
}

func (s *socketTransport) Read(target any) error {
	err := wsjson.Read(s.ctx, s.sock, target)
	if err != nil {
		s.cancel(err)
	}
	return err
}

func (s *socketTransport) Send(out any) error {
	err := wsjson.Write(s.ctx, s.sock, out)
	if err != nil {
		s.cancel(err)
	}
	return err
}
