package transport

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

var (
	ErrMuxBufferFull = errors.New("mux buffer full")
	ErrRemoteStop    = errors.New("remote normal stop")
)

const (
	// DefaultMuxMessageBuffer is the default number of messages allowed to be pending on a sub-transport.
	DefaultMuxMessageBuffer = 128
)

type MuxOptions struct {
	// MuxMessageBuffer allows for this many messages to be pending on a sub-transport before it is canceled.
	// Defaults to DefaultMuxMessageBuffer.
	MuxMessageBuffer int
}

type MuxHandler[ID comparable] interface {
	// Handler is invoked when a new ID is observed.
	Handler(id ID, t Transport) (err error)

	// Default is invoked when a packet with no ID is observed.
	// This is invoked synchronously.
	// If it returns an error, the Mux is closed.
	Default(msg json.RawMessage) (err error)
}

// Mux blocks and processes incoming packets on the given Transport, demultiplexing them.
//
// The wire protocol is a stream of JSON objects with the following fields:
//   - "id": The ID of the sub-transport.
//     If omitted, the previous ID is used.
//     On incoming packets, the zero value (e.g., "" or zero) routes to the default handler.
//   - "p": The payload for the sub-transport.
//   - "stop": If present, closes the sub-transport.
//
// Both sides maintain a "sticky" ID; it is only sent when it changes from the previously sent or received ID on that physical transport.
func Mux[ID comparable](tr Transport, cfg MuxHandler[ID], options *MuxOptions) (err error) {
	m := &muxImpl[ID]{
		muxMessageBuffer: DefaultMuxMessageBuffer,
		tr:               tr,
		known:            make(map[ID]*subTransport[ID]),
	}
	if options != nil && options.MuxMessageBuffer > 0 {
		m.muxMessageBuffer = options.MuxMessageBuffer
	}

	var lastID ID

	for {
		var raw struct {
			ID   *ID             `json:"id,omitzero"`
			P    json.RawMessage `json:"p"`
			Stop *string         `json:"stop"`
		}
		if err := m.tr.ReadJSON(&raw); err != nil {
			return err
		}

		// missing ID uses prior ID; zero ID routes to Default
		var id ID
		if raw.ID == nil {
			id = lastID
		} else {
			id = *raw.ID
			lastID = id
		}

		var zeroID ID
		if id == zeroID {
			if err := cfg.Default(raw.P); err != nil {
				return err
			}
			continue
		}

		m.lock.Lock()
		sub, ok := m.known[id]

		// stop is performed under lock
		if raw.Stop != nil {
			reason := *raw.Stop
			if ok {
				err := ErrRemoteStop
				if reason != "" {
					err = errors.New("remote: " + reason)
				}
				sub.cancel(err)

				// Inline remove to allow immediate re-use of the ID.
				delete(m.known, id)
			}
			m.lock.Unlock()
			continue
		}

		if !ok {
			sub = m.newSubTransport(id)
			m.known[id] = sub

			runner := func(t Transport) (err error) { return cfg.Handler(id, t) }
			go func() {
				err = runner(sub)
				sub.cancel(err)

				// send stop packet, + ignore err (shutdown)
				var reason string
				if err != nil {
					reason = err.Error()
				}
				m.writePacket(sub, &reason, nil)
			}()
		}

		m.lock.Unlock()

		select {
		case sub.in <- raw.P:
		case <-sub.ctx.Done():
			// ignore
		default:
			// sub-transport buffer full; kill it
			sub.cancel(ErrMuxBufferFull)
		}
	}
}

type muxImpl[ID comparable] struct {
	muxMessageBuffer int

	lock   sync.Mutex
	lastID ID
	tr     Transport
	known  map[ID]*subTransport[ID]
}

// writePacket writes data for the given sub, not under lock.
func (m *muxImpl[ID]) writePacket(sub *subTransport[ID], stop *string, body any) (err error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.known[sub.id] != sub {
		return context.Cause(sub.ctx) // not us anymore
	}

	var out struct {
		ID   *ID     `json:"id,omitempty"`
		P    any     `json:"p,omitempty"`
		Stop *string `json:"stop,omitempty"`
	}

	if m.lastID != sub.id {
		out.ID = &sub.id
		m.lastID = sub.id
	}

	if stop != nil {
		out.Stop = stop
		delete(m.known, sub.id) // remove self under lock

	} else {
		out.P = body
	}

	return m.tr.WriteJSON(out)
}

func (m *muxImpl[ID]) newSubTransport(id ID) (st *subTransport[ID]) {
	ctx, cancel := context.WithCancelCause(m.tr.Context())
	return &subTransport[ID]{
		mux:    m,
		id:     id,
		ctx:    ctx,
		cancel: cancel,
		in:     make(chan json.RawMessage, m.muxMessageBuffer),
	}
}

type subTransport[ID comparable] struct {
	mux    *muxImpl[ID]
	id     ID
	ctx    context.Context
	cancel context.CancelCauseFunc
	in     chan json.RawMessage
}

func (s *subTransport[ID]) Context() (ctx context.Context) {
	return s.ctx
}

func (s *subTransport[ID]) ReadJSON(v any) (err error) {
	select {
	case raw := <-s.in:
		return json.Unmarshal(raw, v)
	case <-s.ctx.Done():
		return context.Cause(s.ctx)
	}
}

func (s *subTransport[ID]) WriteJSON(v any) (err error) {
	return s.mux.writePacket(s, nil, v)
}
