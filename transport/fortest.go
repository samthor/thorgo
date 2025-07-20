package transport

import (
	"context"
	"encoding/json"

	"github.com/samthor/thorgo/queue"
)

// NewPair constructs two Transport interfaces that are connected to each other and have an infinite buffer.
// This is likely for testing.
func NewPair(ctx context.Context) (Transport, Transport) {
	l := &testTransport{
		ctx: ctx,
		q:   queue.New[json.RawMessage](),
	}
	r := &testTransport{
		ctx: ctx,
		q:   queue.New[json.RawMessage](),
	}

	l.l = r.q.Join(ctx)
	r.l = l.q.Join(ctx)

	return l, r
}

type testTransport struct {
	ctx context.Context
	q   queue.Queue[json.RawMessage]
	l   queue.Listener[json.RawMessage]
}

func (t *testTransport) Context() context.Context {
	return t.ctx
}

func (t *testTransport) ReadJSON(v any) error {
	raw, ok := t.l.Next()
	if !ok {
		return context.Cause(t.ctx)
	}
	return json.Unmarshal(raw, v)
}

func (t *testTransport) WriteJSON(v any) error {
	select {
	case <-t.ctx.Done():
		return context.Cause(t.ctx)
	default:
	}

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	t.q.Push(b)
	return nil
}
