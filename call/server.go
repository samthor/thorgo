package call

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/samthor/thorgo/transport"
)

// CallHandler is invoked for each new remote call.
type CallHandler[Init any] func(transport.Transport, Init) error

// Handler describes a call handler type that can be served over HTTP.
type Handler[Init any] struct {
	// InitHandler, if non-nil, handles the initial request and generates an Init.
	// Init is JSON-encoded to the caller, so make sure that it reveals fields intentionally.
	InitHandler func(*http.Request) (Init, error)

	// CallHandler is invoked for each call.
	CallHandler CallHandler[Init]

	// SkipOriginVerify allows any hostname to connect here, not just our own.
	SkipOriginVerify bool

	// CallLimit optionally limits the number of calls allowed by a single session.
	// This is probably just each WebSocket connection.
	// A session will be killed if it exceeds this rate; the client must know it too.
	CallLimit *LimitConfig

	// PacketLimit optionally limits the number of packets allowed across all calls.
	// This includes call setup/metadata packets.
	// A session will be killed if it exceeds this rate; the client must know it too.
	PacketLimit *LimitConfig

	// unexported fields

	once        sync.Once
	noopTimeout time.Duration
}

func (ch *Handler[Init]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	options := &websocket.AcceptOptions{InsecureSkipVerify: ch.SkipOriginVerify}
	sock, err := websocket.Accept(w, r, options)
	if err != nil {
		log.Printf("got err setting up websocket %s: %v", r.URL.Path, err)
		http.Error(w, "could not set up websocket", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	err = ch.runSocket(ctx, r, sock)
	cancel(err)

	var closeError websocket.CloseError
	if errors.As(err, &closeError) {
		log.Printf("shutdown socket due to known reason: %+v", closeError)
		sock.Close(closeError.Code, closeError.Reason)
	} else if err != nil && err != context.Canceled {
		log.Printf("shutdown socket due to error: %v", err)
		sock.Close(websocket.StatusInternalError, "")
	} else {
		sock.Close(websocket.StatusNormalClosure, "")
	}
}
