package call

import (
	"context"
	"net/http"
	"time"

	"github.com/samthor/thorgo/transport"
)

// CallFunc is invoked for each new remote call.
type CallFunc[Init any] func(transport.Transport, Init) error

// Handler describes a call handler type that can be served over HTTP.
type Handler[Init any] struct {
	// CallHandler points to an object which implements Init and Call.
	// It takes preference over InitFunc/CallFunc.
	CallHandler CallHandler[Init]

	// InitFunc, if non-nil, handles the initial request and generates an Init.
	// Init is JSON-encoded to the caller, so make sure that it reveals fields intentionally.
	InitFunc func(context.Context, *http.Request) (Init, error)

	// CallFunc is invoked for each call.
	CallFunc CallFunc[Init]

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

	noopTimeout time.Duration
}

type CallHandler[Init any] interface {
	Init(context.Context, *http.Request) (Init, error)
	Call(transport.Transport, Init) error
}
