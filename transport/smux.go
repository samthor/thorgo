package transport

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrProtocol = errors.New("bad smux")
	ErrBuffer   = errors.New("buffer full")
	ErrNoMux    = errors.New("transport is not smux")
)

const (
	SMuxMessageBuffer = 64
)

// DecodeSMuxArg unmarshals the initial argument used to start the SMux call.
// This returns ErrNoMux if the Transport is not part of an SMux call.
func DecodeSMuxArg(tr Transport, v any) (err error) {
	st, ok := tr.(*smuxTransport)
	if !ok {
		return ErrNoMux
	}
	if len(st.startArg) == 0 {
		panic("startArg was nil")
	}
	return json.Unmarshal(st.startArg, v)
}

type smuxTransport struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	incoming chan json.RawMessage
	startArg json.RawMessage
	send     func(v any) (err error)
}

func (s *smuxTransport) Context() (ctx context.Context) {
	return s.ctx
}

func (s *smuxTransport) ReadJSON(v any) (err error) {
	select {
	case <-s.ctx.Done():
		return context.Cause(s.ctx)
	default:
	}

	select {
	case next := <-s.incoming:
		if next != nil {
			return json.Unmarshal(next, v)
		}
		// fall-through, random chance chose closed ch, but ctx will be done
	case <-s.ctx.Done():
	}
	return context.Cause(s.ctx)
}

func (s *smuxTransport) WriteJSON(v any) (err error) {
	return s.send(v)
}
