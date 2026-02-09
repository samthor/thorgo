package transport

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrProtocol = errors.New("bad smux")
	ErrBuffer   = errors.New("buffer full")
)

const (
	SMuxMessageBuffer = 64
)

// SMuxArg unmarshals the initial argument used to start the SMux call, if any.
func SMuxArg[X any](tr Transport) (x X, ok bool) {
	st, ok := tr.(*smuxTransport)
	if !ok || len(st.startArg) == 0 {
		return
	}
	err := json.Unmarshal(st.startArg, &x)
	if err != nil {
		return
	}
	return x, true
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
