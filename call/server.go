package call

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type ActiveCall interface {
	// SessionID returns the globally unique ID for this session in this process.
	// It is always an unsigned int safe for use in JavaScript.
	SessionID() int

	// ReadJSON reads JSON for the call into the given pointer.
	ReadJSON(v any) error

	// WriteJSON writes JSON from the given pointer to the call.
	WriteJSON(v any) error
}

// CallHandler is invoked for each new remote call.
type CallHandler func(context.Context, ActiveCall) error

// Handler describes a call handler type that can be served over HTTP.
type Handler struct {
	Handler CallHandler

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
	sessionCh   <-chan int
	noopTimeout time.Duration
}

func (ch *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	options := &websocket.AcceptOptions{InsecureSkipVerify: ch.SkipOriginVerify}
	sock, err := websocket.Accept(w, r, options)
	if err != nil {
		log.Printf("got err setting up websocket %s: %v", r.URL.Path, err)
		http.Error(w, "could not set up websocket", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	err = ch.runSocket(ctx, sock)
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
