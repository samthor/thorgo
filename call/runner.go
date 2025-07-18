package call

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/samthor/thorgo/queue"
	"golang.org/x/time/rate"
)

type helloResponseMessage[Init any] struct {
	Ok    bool `json:"ok"`
	Init  Init `json:"i"`
	Limit struct {
		Call   *LimitConfig `json:"c,omitzero"`
		Packet *LimitConfig `json:"p,omitzero"`
	} `json:"l"`
}

func (ch *Handler[Init]) runSocket(ctx context.Context, req *http.Request, sock *websocket.Conn) error {
	var init Init
	if ch.InitHandler != nil {
		var err error
		init, err = ch.InitHandler(req)
		if err != nil {
			return err
		}
	}

	helloCtx, helloCancel := context.WithTimeout(ctx, helloTimeout)
	defer helloCancel()

	// wait for hello msg
	var helloMessage struct {
		Protocol string `json:"p"`
	}
	wsjson.Read(helloCtx, sock, &helloMessage)
	if helloMessage.Protocol != "1" {
		return websocket.CloseError{
			Code:   SocketCodeUnknownProtocol,
			Reason: fmt.Sprintf("unknown protocol: %v", 1),
		}
	}

	ch.once.Do(func() {
		if ch.noopTimeout <= 0 {
			ch.noopTimeout = noopTimeout
		}
	})

	// send initial response to hello
	var responseMessage helloResponseMessage[Init]
	responseMessage.Ok = true
	responseMessage.Init = init
	responseMessage.Limit.Call = ch.CallLimit
	responseMessage.Limit.Packet = ch.PacketLimit

	err := wsjson.Write(helloCtx, sock, responseMessage)
	if err != nil {
		return err
	}

	session := &activeSession[Init]{
		ch:            ch,
		ctx:           ctx,
		conn:          sock,
		callLimit:     buildLimiter(ch.CallLimit),
		packetLimit:   buildLimiter(ch.PacketLimit),
		outgoingQueue: queue.New[controlMessage](),
		calls:         map[int]*activeCall{},
	}

	return session.runProtocol1Socket(ctx)
}

type controlMessage struct {
	CallId int             `json:"c"`
	Stop   *string         `json:"stop,omitzero"`
	Packet json.RawMessage `json:"-"`
}

type activeCall struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	q        queue.Queue[json.RawMessage]
	listener queue.Listener[json.RawMessage]
	send     func(v json.RawMessage)
}

func (a *activeCall) Context() context.Context {
	return a.ctx
}

func (a *activeCall) ReadJSON(v any) error {
	next, ok := a.listener.Next()
	if !ok {
		return context.Cause(a.ctx)
	}
	err := json.Unmarshal(next, v)
	if err != nil {
		a.cancel(err)
	}
	return err
}

func (a *activeCall) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		a.cancel(err)
	} else {
		a.send(b)
	}
	return err
}

type activeSession[Init any] struct {
	ctx  context.Context // context of socket
	init Init
	ch   *Handler[Init]

	conn        *websocket.Conn
	callLimit   *rate.Limiter
	packetLimit *rate.Limiter

	outgoingQueue queue.Queue[controlMessage]

	callsLock sync.RWMutex
	calls     map[int]*activeCall
}

func (as *activeSession[Init]) getCall(id int) *activeCall {
	as.callsLock.RLock()
	defer as.callsLock.RUnlock()
	return as.calls[id]
}

func (as *activeSession[Init]) updateCall(id int, active *activeCall) {
	as.callsLock.Lock()
	defer as.callsLock.Unlock()
	if active == nil {
		delete(as.calls, id)
	} else {
		as.calls[id] = active
	}
}

func (as *activeSession[Init]) runOutgoing(l queue.Listener[controlMessage]) error {
	lastId := -1
	for {
		// awkwardly do timeout-per-loop
		t := time.AfterFunc(as.ch.noopTimeout, func() { as.outgoingQueue.Push(controlMessage{}) })

		next, ok := l.Next()
		t.Stop() // cancel timeout
		if !ok {
			return nil
		}

		if next.CallId == 0 {
			// we hit the timeout msg
			as.conn.Write(as.ctx, websocket.MessageText, []byte(":{}"))
			continue
		}

		active := as.getCall(next.CallId)
		if active == nil {
			continue // no longer exists
		}

		if lastId != next.CallId || next.Stop != nil {
			lastId = next.CallId

			b, _ := json.Marshal(next)
			b = append([]byte(":"), b...)
			as.conn.Write(as.ctx, websocket.MessageText, b)

			if next.Stop != nil {
				as.updateCall(next.CallId, nil)
				continue
			}
		}

		err := wsjson.Write(as.ctx, as.conn, next.Packet)
		if err != nil {
			return err
		}
	}
}

func (session *activeSession[Init]) runProtocol1Socket(ctx context.Context) error {
	// run outgoing queue (ignore err)
	go session.runOutgoing(session.outgoingQueue.Join(ctx))

	// handle incoming stuff
	lastIncomingId := -1
	lastNewCall := 0
	for {
		_, b, err := session.conn.Read(ctx)
		if err != nil {
			return err
		}
		if !session.packetLimit.Allow() {
			// drop; sending too many packets
			return websocket.CloseError{Code: SocketCodeExcessTraffic}
		}

		if len(b) == 0 || b[0] != ':' {
			// normal message
			active := session.getCall(lastIncomingId)
			if active != nil {
				active.q.Push(b)
			}
			continue
		}

		// look for control message
		var c controlMessage
		json.Unmarshal(b[1:], &c)
		if c.CallId <= 0 {
			continue
		}
		active := session.getCall(c.CallId)

		// handle incoming stop
		if c.Stop != nil {
			if active != nil {
				active.cancel(fmt.Errorf("client: %v", c.Stop))
				session.updateCall(c.CallId, nil)
			}
			continue
		}

		// already exists, just a swap
		lastIncomingId = c.CallId
		if active != nil {
			continue
		}

		// create new call (if in order)
		if c.CallId <= lastNewCall {
			// can't re-use IDs
			return websocket.CloseError{Code: SocketCodeBadCallId}
		}

		if !session.callLimit.Allow() {
			// drop; sending too many calls
			return websocket.CloseError{Code: SocketCodeExcessTraffic}
		}

		callCtx, cancel := context.WithCancelCause(ctx)

		q := queue.New[json.RawMessage]()
		l := q.Join(callCtx)

		active = &activeCall{
			ctx:      callCtx,
			cancel:   cancel,
			q:        q,
			listener: l,
			send: func(v json.RawMessage) {
				session.outgoingQueue.Push(controlMessage{
					CallId: c.CallId,
					Packet: v,
				})
			},
		}
		session.updateCall(c.CallId, active)

		go func() {
			err := session.ch.CallHandler(active, session.init)
			cancel(err)
		}()

		context.AfterFunc(callCtx, func() {
			// we shutdown the call as a side-effect
			err := context.Cause(callCtx)
			var stop string
			if err != context.Canceled {
				stop = err.Error()
			}

			session.outgoingQueue.Push(controlMessage{
				CallId: c.CallId,
				Stop:   &stop,
			})
		})
	}
}
