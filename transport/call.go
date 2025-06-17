package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/samthor/thorgo/queue"
)

// CallServer runs a new socket server with an internal simple keyed protocol.
// Invokes the handler for every new bidirectional call.
// This is designed to be similar-ish to WebTransport so we can use that one day.
func CallServer(tr Transport, handle func(Transport) error) error {
	var sendLock sync.Mutex
	conns := map[int]*activeConn{}
	key := -1
	lastKeySent := -1

	for {
		var data json.RawMessage
		err := tr.Read(&data)
		if err != nil {
			return err
		}

		var ok, stop bool
		var cause string
		key, data, stop, cause, ok = readCallPacket(key, data)
		if !ok {
			continue
		}

		func() {
			sendLock.Lock()
			defer sendLock.Unlock()

			active := conns[key]
			if active != nil {
				if stop {
					active.cancel(fmt.Errorf("client: %s", cause))
					delete(conns, key)
				} else {
					active.q.Push(data)
				}
				return
			} else if stop {
				return
			}

			// setup new call
			localKey := key
			ctx, cancel := context.WithCancelCause(tr.Context())
			active = &activeConn{
				ctx:    ctx,
				cancel: cancel,
				q:      queue.New[json.RawMessage](),
				send: func(v json.RawMessage) {
					select {
					case <-ctx.Done():
						return
					default:
					}

					sendLock.Lock()
					defer sendLock.Unlock()

					if lastKeySent != localKey {
						lastKeySent = localKey
						tr.Send([]any{localKey, v})
					} else {
						tr.Send(v)
					}
				},
			}
			active.listener = active.q.Join(ctx)
			conns[key] = active
			active.q.Push(data)

			go func() {
				err := handle(active)

				// send before cancel
				if err != nil {
					s := err.Error()
					msg, _ := json.Marshal(s)
					active.send(msg)
				} else {
					active.send([]byte("true"))
				}

				sendLock.Lock()
				defer sendLock.Unlock()
				delete(conns, key)
				cancel(err)
			}()
		}()
	}
}

type activeConn struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	q        queue.Queue[json.RawMessage]
	listener queue.Listener[json.RawMessage]
	send     func(v json.RawMessage)
}

func (a *activeConn) Context() context.Context {
	return a.ctx
}

func (a *activeConn) Read(v any) error {
	next, ok := a.listener.Next()
	if !ok {
		return context.Cause(a.ctx)
	}
	return json.Unmarshal(next, v)
}

func (a *activeConn) Send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.send(b)
	return nil
}

func readCallPacket(lastKey int, raw json.RawMessage) (key int, data json.RawMessage, stop bool, cause string, ok bool) {
	if len(raw) == 0 {
		return
	}

	if raw[0] == '[' {
		var arr []json.RawMessage
		json.Unmarshal(raw, &arr)
		if len(arr) != 2 {
			return
		}

		json.Unmarshal(arr[0], &key)
		raw = arr[1]
	} else {
		key = lastKey
	}

	if key <= 0 {
		return
	}

	if raw[0] == 't' || raw[0] == '"' {
		json.Unmarshal(raw, &cause)
		stop = true
		ok = true
		return
	}

	if raw[0] == '{' {
		// direct object ref
		ok = true
		data = raw
		return
	}

	return
}
