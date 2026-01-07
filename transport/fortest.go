package transport

import (
	"context"
	"encoding/json"
)

// NewBufferPair constructs two Transport interfaces that are connected to each other.
// Pass a buffer size, or zero for blocking.
// Internally uses channels, so write/read has those semantics.
func NewBufferPair(ctx context.Context, size int) (Transport, Transport) {
	done := make(chan struct{})

	ch1 := make(chan json.RawMessage, size)
	ch2 := make(chan json.RawMessage, size)

	l := &bufferTransport{ctx: ctx, readCh: ch1, writeCh: ch2, doneCh: done}
	r := &bufferTransport{ctx: ctx, readCh: ch2, writeCh: ch1, doneCh: done}

	// close immediately if already done (don't wait for goroutine)
	select {
	case <-ctx.Done():
		close(done)
	default:
		context.AfterFunc(ctx, func() { close(done) })
	}

	return l, r
}

type bufferTransport struct {
	ctx     context.Context
	readCh  <-chan json.RawMessage
	writeCh chan<- json.RawMessage
	doneCh  <-chan struct{}
}

func (t *bufferTransport) Context() context.Context {
	return t.ctx
}

func (t *bufferTransport) ReadJSON(v any) (err error) {
	select {
	case <-t.doneCh:
		return context.Cause(t.ctx)
	default:
	}

	select {
	case <-t.doneCh:
		return context.Cause(t.ctx)
	case raw := <-t.readCh:
		return json.Unmarshal(raw, v)
	}
}

func (t *bufferTransport) WriteJSON(v any) (err error) {
	select {
	case <-t.doneCh:
		return context.Cause(t.ctx)
	default:
	}

	var b []byte
	b, err = json.Marshal(v)
	if err != nil {
		return err
	}

	select {
	case t.writeCh <- b:
		return nil // ok, sent!
	case <-t.doneCh:
		return context.Cause(t.ctx)
	}
}
