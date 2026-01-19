package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"golang.org/x/time/rate"
)

const (
	// DefaultMaxPacketSize is the maximum size of a JSON packet we accept.
	DefaultMaxPacketSize = 32768

	// DefaultInMessageBuffer allows for this many packets to be pending before we close the connection.
	DefaultInMessageBuffer = 128

	// DefaultRateLimit is the number of messages per second we allow.
	DefaultRateLimit = 100

	// DefaultRateBurst is the maximum burst of messages we allow.
	DefaultRateBurst = 100
)

// HandshakeResponse is the response sent to the client after a successful hello.
type HandshakeResponse struct {
	Ok            bool `json:"ok"`
	MaxPacketSize int  `json:"max_packet_size"`
	RateLimit     int  `json:"rate_limit"`
	RateBurst     int  `json:"rate_burst"`
}

// SocketOpts configures the WebSocket handler.
type SocketOpts struct {
	// MaxPacketSize is the maximum size of a JSON packet we accept.
	// Defaults to DefaultMaxPacketSize if zero.
	MaxPacketSize int

	// InMessageBuffer allows for this many packets to be pending before we close the connection.
	// Defaults to DefaultInMessageBuffer if zero.
	InMessageBuffer int

	// RateLimit is the number of messages per second we allow.
	// Defaults to DefaultRateLimit if zero.
	RateLimit int

	// RateBurst is the maximum burst of messages we allow.
	// Defaults to DefaultRateBurst if zero.
	RateBurst int

	// PingEvery sends a ping every ~duration.
	PingEvery time.Duration
}

func (o *SocketOpts) setDefaults() {
	if o.MaxPacketSize == 0 {
		o.MaxPacketSize = DefaultMaxPacketSize
	}
	if o.InMessageBuffer == 0 {
		o.InMessageBuffer = DefaultInMessageBuffer
	}
	if o.RateLimit == 0 {
		o.RateLimit = DefaultRateLimit
	}
	if o.RateBurst == 0 {
		o.RateBurst = DefaultRateBurst
	}
}

// Handler describes the wrapped method for NewWebSocketHandler.
// It runs a Transport.
type Handler func(tr Transport) (err error)

// NewWebSocketHandler returns an http.Handler that upgrades requests to WebSocket connections and wraps them in a Transport interface.
// This always sets InsecureSkipVerify, you should wrap this with something that checks the origin.
// The provided handle function is called for each established connection.
// When the handle function returns, the WebSocket connection is closed.
func NewWebSocketHandler(opts SocketOpts, transportHandler Handler) (h http.Handler) {
	opts.setDefaults()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return // websocket.Accept already writes an error response if it fails.
		}
		c.SetReadLimit(int64(opts.MaxPacketSize)) // set sane read limit

		// Define an primary readCtx that cancels after our "normal" shutdown.
		// Don't use the http.Request Context, see websocket.Accept comment.
		// Without this, the pending wsjson.Read call proactively shuts down the connection before we Close.
		readCtx, readCancel := context.WithCancel(context.Background())

		// Wrap the connection in our Transport implementation.
		ctx, cancel := context.WithCancelCause(readCtx)
		tr := &wsTransport{
			ctx:     ctx,
			cancel:  cancel,
			conn:    c,
			inCh:    make(chan []byte, opts.InMessageBuffer),
			limiter: rate.NewLimiter(rate.Limit(opts.RateLimit+1), opts.RateBurst+1), // +1 for hello msg and general safety
		}

		context.AfterFunc(ctx, func() {
			err := context.Cause(ctx)
			closeErr := websocket.CloseError{Code: websocket.StatusNormalClosure}

			var transportErr TransportError
			if errors.As(err, &transportErr) {
				closeErr.Code = 3000
				closeErr.Reason = transportErr.Encode()
			} else if errors.As(err, &closeErr) {
				// ok
			} else if err == nil || errors.Is(err, context.Canceled) {
				// ok
			} else {
				// don't emit internal errors
				closeErr.Code = websocket.StatusInternalError
			}

			c.Close(closeErr.Code, closeErr.Reason)
			readCancel() // only cancel readCtx after ctx
		})

		// ping if requested
		pingEvery := opts.PingEvery
		if pingEvery > 0 {
			go func() {
				for {
					// ping ~75% - 125% of requested time
					d := time.Duration((0.5*rand.Float64() + 0.75) * float64(opts.PingEvery))
					select {
					case <-ctx.Done():
						return
					case <-time.Tick(d):
					}
					c.Ping(ctx)
				}
			}()
		}

		go func() {
			err := tr.runRead(readCtx)
			cancel(err)
		}()

		err = tr.run(opts, transportHandler)
		cancel(err)
	})
}

type wsTransport struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc // only used for read/write JSON failure
	conn    *websocket.Conn
	inCh    chan []byte
	limiter *rate.Limiter
}

func (t *wsTransport) run(opts SocketOpts, transportHandler Handler) (err error) {
	// Handshake: Expect "hello" packet with version "1".
	// We only support version 1 for now and will error if any other version is seen.
	var hello struct {
		Type    string `json:"type"`
		Version string `json:"version"`
	}
	if err = t.ReadJSON(&hello); err != nil {
		return websocket.CloseError{Code: websocket.StatusPolicyViolation, Reason: "failed to read hello"}
	}
	if hello.Type != "hello" || hello.Version != "1" {
		return websocket.CloseError{Code: websocket.StatusPolicyViolation, Reason: "invalid hello or version"}
	}

	// Reply with hello response.
	// We report the default maximum packet size.
	resp := HandshakeResponse{
		Ok:            true,
		MaxPacketSize: opts.MaxPacketSize,
		RateLimit:     opts.RateLimit,
		RateBurst:     opts.RateBurst,
	}
	if err = t.WriteJSON(resp); err != nil {
		return
	}
	return transportHandler(t)
}

func (t *wsTransport) runRead(ctx context.Context) (err error) {
	for {
		typ, b, err := t.conn.Read(ctx) // t.ctx
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		if typ != websocket.MessageText {
			return websocket.CloseError{Code: websocket.StatusUnsupportedData, Reason: "unexpected message type"}
		}

		// Check rate limiter permission.
		if !t.limiter.Allow() {
			return websocket.CloseError{Code: websocket.StatusPolicyViolation, Reason: "rate limit exceeded"}
		}

		select {
		case t.inCh <- b:
		default:
			// Channel full, slow consumer
			return websocket.CloseError{Code: websocket.StatusPolicyViolation, Reason: "input channel full"}
		}
	}
}

func (t *wsTransport) Context() (ctx context.Context) {
	return t.ctx
}

func (t *wsTransport) ReadJSON(v any) (err error) {
	select {
	case b := <-t.inCh:
		err = json.Unmarshal(b, v)
		if err != nil {
			t.cancel(err) // kill ctx if we fail to read
		}
		return err
	case <-t.ctx.Done():
		return context.Cause(t.ctx)
	}
}

func (t *wsTransport) WriteJSON(v any) (err error) {
	err = wsjson.Write(t.ctx, t.conn, v)
	if err != nil {
		t.cancel(err) // kill ctx if we fail to write
	}
	return err
}
