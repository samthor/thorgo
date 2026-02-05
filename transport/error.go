package transport

import (
	"context"
)

type errorTransport struct {
	ctx context.Context
}

func (e *errorTransport) Context() (ctx context.Context) {
	return e.ctx
}

func (e *errorTransport) ReadJSON(v any) (err error) {
	return context.Cause(e.ctx)
}

func (e *errorTransport) WriteJSON(v any) (err error) {
	return context.Cause(e.ctx)
}
