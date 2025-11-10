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
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

var (
	testNoopTimeout = time.Duration(0)
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
	helloCtx, helloCancel := context.WithTimeout(ctx, helloTimeout)
	defer helloCancel()

	var helloMessage struct {
		Protocol string `json:"p"`
	}
	var init Init

	// we want to be actively waiting for a wsjson read, it's how we are informed the socket has closed early
	eg, groupCtx := errgroup.WithContext(helloCtx)

	eg.Go(func() error {
		err := wsjson.Read(groupCtx, sock, &helloMessage)
		if err != nil {
			return err
		} else if helloMessage.Protocol != "1" {
			return websocket.CloseError{
				Code:   SocketCodeUnknownProtocol,
				Reason: fmt.Sprintf("unknown protocol: %v", 1),
			}
		}
		return nil
	})

	eg.Go(func() error {
		var err error
		init, err = ch.CallHandler.Init(groupCtx, req)
		return err
	})

	err := eg.Wait()
	if err != nil {
		return err
	}

	// wait for hello msg

	// send initial response to hello
	var responseMessage helloResponseMessage[Init]
	responseMessage.Ok = true
	responseMessage.Init = init
	responseMessage.Limit.Call = ch.CallLimit
	responseMessage.Limit.Packet = ch.PacketLimit

	err = wsjson.Write(helloCtx, sock, responseMessage)
	if err != nil {
		return err
	}

	session := &activeSession[Init]{
		ctx:           ctx,
		init:          init,
		ch:            ch,
		conn:          sock,
		callLimit:     buildLimiter(ch.CallLimit, ch.ExtraLimit),
		packetLimit:   buildLimiter(ch.PacketLimit, ch.ExtraLimit),
		outgoingQueue: queue.New[controlMessage](),
		calls:         map[int]*activeCall{},
	}

	return session.runProtocol1Socket(ctx)
}

type controlMessage struct {
	CallId int     `json:"c"`
	Stop   *string `json:"stop,omitzero"`
	Packet any     `json:"-"`
}

type activeCall struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	q        queue.Queue[[]byte]
	listener queue.Listener[[]byte]
	send     func(v any)
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
	a.send(v)
	return nil
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
		timeout := testNoopTimeout
		if timeout <= 0 {
			timeout = noopTimeout
		}
		t := time.AfterFunc(timeout, func() { as.outgoingQueue.Push(controlMessage{}) })

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
	var startLock sync.Mutex

	// run outgoing queue (ignore err)
	go session.runOutgoing(session.outgoingQueue.Join(ctx))

	// handle incoming stuff
	lastIncomingId := -1
	lastNewCall := 0
	for {
		typ, b, err := session.conn.Read(ctx)
		if err != nil {
			return err
		}
		if !session.packetLimit.Allow() {
			// drop; sending too many packets
			return websocket.CloseError{Code: SocketCodeExcessTraffic}
		}

		if typ == websocket.MessageBinary || len(b) == 0 || b[0] != ':' {
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
			// can't re-use IDs (also picks up -ve IDs)
			return websocket.CloseError{Code: SocketCodeBadCallID}
		}

		if !session.callLimit.Allow() {
			// drop; sending too many calls
			return websocket.CloseError{Code: SocketCodeExcessTraffic}
		}

		callCtx, cancel := context.WithCancelCause(ctx)

		q := queue.New[[]byte]()
		l := q.Join(callCtx)

		active = &activeCall{
			ctx:      callCtx,
			cancel:   cancel,
			q:        q,
			listener: l,
			send: func(v any) {
				session.outgoingQueue.Push(controlMessage{
					CallId: c.CallId,
					Packet: v,
				})
			},
		}
		session.updateCall(c.CallId, active)

		go func() {
			startLock.Lock()
			var unlocked bool
			unlockOnce := func() {
				if !unlocked {
					unlocked = true
					startLock.Unlock()
				}
			}

			err := session.ch.CallHandler.Call(active, session.init, unlockOnce)
			unlockOnce() // in case the handler never ran it
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
