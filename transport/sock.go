package transport

import (
	"context"
	"errors"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/samthor/thorgo/internal"
)

type socketTransport struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	sock   *websocket.Conn
}

func (s *socketTransport) Context() context.Context {
	return s.ctx
}

func (s *socketTransport) Read(t any) error {
	err := wsjson.Read(s.ctx, s.sock, t)
	if err != nil {
		s.cancel(err)
	}
	return err
}

func (s *socketTransport) Send(o any) error {
	err := wsjson.Write(s.ctx, s.sock, o)
	if err != nil {
		s.cancel(err)
	}
	return err
}

// SocketJSON wraps an open websocket.Conn, converting it to an untyped Transport that reads and writes JSON.
// It derives a new Context which is canceled only if a Read/Send operation fails, but which also closes the socket itself.
// If the cancel method is called with a [websocket.CloseError], the socket is closed in that way.
func SocketJSON(ctx context.Context, sock *websocket.Conn) (t Transport, cancel context.CancelCauseFunc) {
	ctx = internal.RegisterHttpContext(ctx)
	socketCtx, cancel := context.WithCancelCause(ctx)

	context.AfterFunc(socketCtx, func() {
		cause := context.Cause(socketCtx)
		var closeError websocket.CloseError

		if errors.As(cause, &closeError) {
			sock.Close(closeError.Code, closeError.Reason)
		} else if cause != context.Canceled {
			sock.Close(websocket.StatusInternalError, "")
		} else {
			sock.Close(websocket.StatusNormalClosure, "")
		}
	})

	return &socketTransport{
		ctx:    socketCtx,
		cancel: cancel,
		sock:   sock,
	}, cancel
}
