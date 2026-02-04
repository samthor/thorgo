package transport

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

var (
	ErrProtocol = errors.New("bad smux")
	ErrBuffer   = errors.New("buffer full")
)

const (
	SMuxMessageBuffer = 64
)

// ArgHandler runs a Transport with an initial Arg.
type ArgHandler[Arg any] func(tr Transport, arg Arg) (err error)

// SMuxRaw is ala SMux but simply expects an empty object for call kickoff.
func SMuxRaw(tr Transport, handler Handler) (top Transport) {
	return SMux(tr, func(tr Transport, arg struct{}) (err error) {
		return handler(tr)
	})
}

// SMux wraps Transport and provides a multiplexer.
// The returned Transport must be waited on via ReadJSON() to operate.
func SMux[Arg any](tr Transport, handler ArgHandler[Arg]) (top Transport) {
	var impl *smuxImpl[Arg]

	if _, ok := tr.(*wsTransport); ok {
		impl = &smuxImpl[Arg]{
			send: func(control bool, p json.RawMessage) (err error) {
				if control {
					var zero int
					cp := ControlPacket[json.RawMessage]{C: &zero, P: p}
					return tr.WriteJSON(&cp)
				}
				return tr.WriteJSON(p)
			},
			read: func() (control bool, p json.RawMessage, err error) {
				var cp ControlPacket[json.RawMessage]
				if err := tr.ReadJSON(&cp); err != nil {
					return false, nil, err
				}
				if cp.C == nil {
					return false, cp.P, nil
				} else if *cp.C != 0 {
					return false, nil, ErrProtocol
				}
				return true, cp.P, nil
			},
		}
	} else {
		// Fallback for generic Transport (e.g., tests or other implementations)
		// We use a struct to wrap the control flag and the data.
		type transportPacket struct {
			Control bool            `json:"control"`
			Data    json.RawMessage `json:"data"`
		}

		impl = &smuxImpl[Arg]{
			send: func(control bool, p json.RawMessage) (err error) {
				tp := transportPacket{
					Control: control,
					Data:    p,
				}
				return tr.WriteJSON(tp)
			},
			read: func() (control bool, p json.RawMessage, err error) {
				var tp transportPacket
				if err := tr.ReadJSON(&tp); err != nil {
					return false, nil, err
				}
				return tp.Control, tp.Data, nil
			},
		}
	}

	impl.ctx = tr.Context()
	impl.handler = handler
	impl.calls = make(map[int]*smuxTransport)
	return impl
}

type smuxImpl[Arg any] struct {
	ctx  context.Context
	send func(control bool, p json.RawMessage) (err error)
	read func() (control bool, p json.RawMessage, err error)

	handler ArgHandler[Arg]

	readLock       sync.Mutex
	lastIncomingID int
	lastNewCallID  int

	controlLock    sync.Mutex
	lastOutgoingID int
	calls          map[int]*smuxTransport
}

func (s *smuxImpl[Arg]) Context() (ctx context.Context) {
	return s.ctx
}

func (s *smuxImpl[Arg]) processControl(p json.RawMessage) (err error) {
	var smuxControl struct {
		ID   int     `json:"id"`
		Stop *string `json:"stop"`
		Arg  *Arg    `json:"arg,omitempty"`
	}
	err = json.Unmarshal(p, &smuxControl)
	if err != nil {
		return err
	}

	if smuxControl.ID == 0 {
		s.lastIncomingID = 0
		return
	}

	call := s.calls[smuxControl.ID]
	if smuxControl.Stop != nil {
		if call == nil {
			return nil // can't stop anyway
		}
		var cause error
		if *smuxControl.Stop != "" {
			cause = errors.New(*smuxControl.Stop)
		}
		call.cancel(cause)
		delete(s.calls, smuxControl.ID)
		s.lastIncomingID = 0
		return nil
	}

	if smuxControl.Arg != nil {
		callID := smuxControl.ID

		if callID <= s.lastNewCallID {
			// must always go up
			return ErrProtocol
		}
		s.lastNewCallID = callID

		callCtx, cancel := context.WithCancelCause(s.Context())
		call = &smuxTransport{
			ctx:      callCtx,
			cancel:   cancel,
			incoming: make(chan json.RawMessage, SMuxMessageBuffer),
			send: func(v any) (err error) {
				return s.internalSend(callID, v, false)
			},
		}
		s.calls[callID] = call

		go func() {
			err := s.handler(call, *smuxControl.Arg)
			cancel(err)
			s.internalSend(callID, err, true)
		}()
	}

	// client is allowed to route to an unknown/bad call
	s.lastIncomingID = smuxControl.ID
	return nil
}

func (s *smuxImpl[Arg]) processPacket(control bool, p json.RawMessage) (err error) {
	s.controlLock.Lock()
	defer s.controlLock.Unlock()

	if control {
		err = s.processControl(p)
		if err != nil {
			return err
		}
		return nil
	}

	call := s.calls[s.lastIncomingID]
	if call == nil {
		return nil // silently ignore unknown calls (maybe stop out of order)
	}

	select {
	case call.incoming <- p:
		return nil
	default:
		// can't send to call, no buffer available
		return ErrBuffer
	}
}

func (s *smuxImpl[Arg]) ReadJSON(v any) (err error) {
	s.readLock.Lock()
	defer s.readLock.Unlock()

	for {
		control, p, err := s.read()
		if err != nil {
			return err
		}

		if !control && s.lastIncomingID == 0 {
			// packet sent to top-level, return inline
			return json.Unmarshal(p, v)
		}
		err = s.processPacket(control, p)
		if err != nil {
			return err
		}
	}
}

// internalSend enacts an outbound send.
// This does not require a lock.
func (s *smuxImpl[Arg]) internalSend(id int, v any, stop bool) (err error) {
	if id == 0 && stop {
		panic("can't stop id=0")
	}

	// build a handy stop packet
	var stopErr error
	if stop {
		err, ok := v.(error)
		var p struct {
			ID   int    `json:"id"`
			Stop string `json:"stop"`
		}
		p.ID = id
		if ok {
			stopErr = err
			p.Stop = err.Error()
		}
		v = p
	}

	// marshal inline and prepare for send
	enc, err := json.Marshal(v)
	if err != nil {
		return err
	}

	s.controlLock.Lock()
	defer s.controlLock.Unlock()

	if stop {
		call := s.calls[id]
		if call == nil {
			return // already stopped?
		}
		call.cancel(stopErr)
		delete(s.calls, id)

		// we don't care what the lastOutgoingID was, just send stop
		return s.send(true, enc)
	}

	// if the ID is wrong, we need to fix it
	if s.lastOutgoingID != id {
		var p struct {
			ID int `json:"id"`
		}
		p.ID = id
		controlEnc, err := json.Marshal(p)
		if err != nil {
			return err
		}
		err = s.send(true, controlEnc)
		if err != nil {
			return err
		}
		s.lastOutgoingID = id
	}

	return s.send(false, enc)
}

func (s *smuxImpl[Arg]) WriteJSON(v any) (err error) {
	return s.internalSend(0, v, false)
}

type smuxTransport struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	incoming chan json.RawMessage
	send     func(v any) (err error)
}

func (s *smuxTransport) Context() (ctx context.Context) {
	return s.ctx
}

func (s *smuxTransport) ReadJSON(v any) (err error) {
	next := <-s.incoming
	if next == nil {
		return ErrProtocol
	}
	return json.Unmarshal(next, v)
}

func (s *smuxTransport) WriteJSON(v any) (err error) {
	return s.send(v)
}
