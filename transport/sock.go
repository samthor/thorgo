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

// SocketJSON wraps an open websocket.Conn, converting it to an untyped Transport.
// It derives a new Context which is canceled only if a Read/Send operation fails, but which also closes the socket itself.
func SocketJSON(ctx context.Context, sock *websocket.Conn) Transport {
	ctx = internal.RegisterHttpContext(ctx)
	socketCtx, cancel := context.WithCancelCause(ctx)

	context.AfterFunc(socketCtx, func() {
		err := socketCtx.Err()
		var closeError websocket.CloseError

		if errors.As(err, &closeError) {
			sock.Close(closeError.Code, closeError.Reason)
		} else if err != context.Canceled {
			sock.Close(websocket.StatusInternalError, "")
		} else {
			sock.Close(websocket.StatusNormalClosure, "")
		}
	})

	return &socketTransport{
		ctx:    socketCtx,
		cancel: cancel,
		sock:   sock,
	}
}
