package transport

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

// SMuxCall makes a call to the other side of this multiplexed Transport.
type SMuxCall func(ctx context.Context, arg any) (tr Transport)

// SMuxClient represents one side of a multiplexed connection built over a Transport.
// It can be both the client and/or the server.
// If the provided Handler is nil, simply closes all incoming calls.
func SMuxClient(tr Transport, h Handler) (top Transport, call SMuxCall) {
	client := &smuxClientImpl{
		handler: h,
		wrap:    tr,
		calls:   map[int]*smuxTransport{},
	}

	return client, client.call
}

type smuxClientImpl struct {
	handler Handler
	wrap    Transport

	readLock       sync.Mutex
	lastIncomingID int

	lock               sync.Mutex
	calls              map[int]*smuxTransport
	lastIncomingCallID int
	lastOutgoingCallID int
	lastOutgoingID     int
}

func (s *smuxClientImpl) Context() (ctx context.Context) {
	return s.wrap.Context()
}

func (s *smuxClientImpl) ReadJSON(v any) (err error) {
	s.readLock.Lock()
	defer s.readLock.Unlock()

	for {
		var cp ControlPacket[json.RawMessage]
		err = s.wrap.ReadJSON(&cp)
		if err != nil {
			return err
		}

		if cp.C == nil {
			if s.lastIncomingID == 0 {
				// packet sent to top-level, return inline
				return json.Unmarshal(cp.P, v)
			}

			// regular packet for call
			call := s.calls[s.lastIncomingID]
			if call == nil {
				continue // silently ignore unknown calls (maybe stop out of order)
			}
			select {
			case call.incoming <- cp.P:
			default:
				// can't send to call, no buffer available
				return ErrBuffer
			}
			continue

		} else if *cp.C != 0 {
			return ErrProtocol
		}

		// control packet
		var control smuxControlPacket
		err = json.Unmarshal(cp.P, &control)
		if err != nil {
			return err
		}
		err = s.processControl(control)
		if err != nil {
			return err
		}
	}
}

// processControl will be called under readLock.
func (s *smuxClientImpl) processControl(control smuxControlPacket) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if control.ID == 0 {
		s.lastIncomingID = 0
		return
	}

	id := -control.ID // flip ID from other side
	s.lastIncomingID = id

	call := s.calls[id]

	// other side is stopping us
	if control.Stop != nil {
		if call == nil {
			return nil // can't stop anyway
		}
		var cause error
		if *control.Stop != "" {
			cause = errors.New(*control.Stop)
		}
		call.cancel(cause)
		delete(s.calls, control.ID)
		s.lastIncomingID = 0
		return nil
	}

	// other side is calling us
	if len(control.Arg) != 0 {
		if id >= s.lastIncomingCallID {
			// must always be positive from them (negative for us)
			return ErrProtocol
		}
		s.lastIncomingCallID = id

		callCtx, cancel := context.WithCancelCause(s.Context())
		call = &smuxTransport{
			ctx:      callCtx,
			cancel:   cancel,
			incoming: make(chan json.RawMessage, SMuxMessageBuffer),
			startArg: control.Arg,
			send:     func(v any) (err error) { return s.internalWrite(id, v) },
		}
		s.calls[id] = call

		go func() {
			err := context.Canceled
			if s.handler != nil {
				err = s.handler(call)
				if err == nil {
					err = context.Canceled
				}
			}
			s.internalStop(id, err) // stop inbound call
		}()
	}

	return nil
}

func (s *smuxClientImpl) WriteJSON(v any) (err error) {
	return s.internalWrite(0, v)
}

func (s *smuxClientImpl) call(ctx context.Context, v any) (tr Transport) {
	tr, err := s.internalStart(ctx, v)
	if err != nil {
		ctx, cancel := context.WithCancelCause(s.Context())
		cancel(err)
		return &errorTransport{ctx}
	}
	return tr
}

func (s *smuxClientImpl) internalWrite(id int, v any) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	call := s.calls[id]
	if id != 0 && call == nil {
		return // ignore
	}

	// send new ID for other side to know about
	if s.lastOutgoingID != id {
		var cp ControlPacket[struct {
			ID int `json:"id"`
		}]
		cp.P.ID = id
		cp.C = new(int)

		err = s.wrap.WriteJSON(&cp)
		if err != nil {
			return err
		}
		s.lastOutgoingID = id
	}

	// send actual packet
	return s.wrap.WriteJSON(v)
}

// internalStop stops a prior inbound (-ve) or outbound (+ve) call.
func (s *smuxClientImpl) internalStop(id int, stopErr error) (err error) {
	if id == 0 {
		panic("can't stop id=0")
	}

	cp := ControlPacket[smuxControlPacket]{C: new(int)}
	cp.P.ID = id
	cp.P.Stop = new(string)

	if stopErr != context.Canceled {
		s := stopErr.Error()
		cp.P.Stop = &s
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	call := s.calls[id]
	if call == nil {
		return nil
	}
	call.cancel(err)
	delete(s.calls, id)
	close(call.incoming)

	s.lastOutgoingID = 0 // further packets are zero

	return s.wrap.WriteJSON(&cp)
}

// internalStart kicks off a call to the other end of this smux.
func (s *smuxClientImpl) internalStart(ctx context.Context, arg any) (tr Transport, err error) {
	startArg, err := json.Marshal(arg)
	if err != nil {
		return nil, err
	}

	// can't start with dead ctx
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case <-s.Context().Done():
		return nil, context.Cause(s.Context())
	default:
	}

	// merge ctx
	derivedCtx, cancel := context.WithCancelCause(s.Context())
	context.AfterFunc(ctx, func() {
		cancel(context.Cause(ctx))
	})

	s.lock.Lock()
	defer s.lock.Unlock()

	s.lastOutgoingCallID++
	id := s.lastOutgoingCallID

	call := &smuxTransport{
		ctx:      derivedCtx,
		cancel:   cancel,
		send:     func(v any) (err error) { return s.internalWrite(id, v) },
		startArg: startArg,
		incoming: make(chan json.RawMessage, SMuxMessageBuffer),
	}
	s.calls[id] = call

	cp := ControlPacket[smuxControlPacket]{C: new(int)}
	cp.P.ID = id
	cp.P.Arg = startArg

	err = s.wrap.WriteJSON(&cp)
	if err != nil {
		return nil, err
	}

	s.lastOutgoingID = id

	context.AfterFunc(derivedCtx, func() {
		s.internalStop(id, context.Cause(derivedCtx)) // stop outbound call
	})

	return call, nil
}

type smuxControlPacket struct {
	ID   int             `json:"id"`
	Arg  json.RawMessage `json:"arg,omitzero"`
	Stop *string         `json:"stop,omitzero"`
}
