package call

import (
	"context"
	"net/http"

	"github.com/samthor/thorgo/transport"
)

// Handler describes a call handler type that can be served over HTTP.
type Handler[Init any] struct {
	// CallHandler points to an object which implements Init and Call.
	// It is the only required part of Handler.
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

	// ExtraLimit can be used to allow a little extra on the optional packet limits.
	// This isn't announced to clients, but deals with them being a little dumb/aggressive.
	ExtraLimit float64

	// EventStart can be used to log, or start another goroutine etc.
	// It is called inline when a new connection begins, before it is passed to Init, but after it is configured as a WebSocket.
	EventStart func(ctx context.Context, r *http.Request)
}

type CallHandler[Init any] interface {
	// Init handles the initial request (i.e., a new WebSocket) and prepares an Init for the session.
	Init(context.Context, *http.Request) (init Init, err error)

	// Call is invoked for each unique call.
	// The ready function must be called to allow other calls to start (existing calls will still run).
	// This is useful to allow in-order setup (since WebSocket is ordered).
	Call(tr transport.Transport, init Init, ready func()) (err error)
}
