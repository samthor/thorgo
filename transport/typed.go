package transport

import (
	"context"
)

type TypeTransport[X any] interface {
	// Context returns the underlying Context for the physical transport, e.g., of the HTTP connection.
	Context() (ctx context.Context)

	// Read reads from this TypeTransport.
	Read() (x X, err error)

	// Write writes into this TypeTransport.
	Write(x X) (err error)
}

// NewTyped wraps the given Transport with JSON serialization to provide a typed interface.
func NewTyped[X any](tr Transport) (tt TypeTransport[X]) {
	return &typeTransport[X]{tr: tr}
}

type typeTransport[X any] struct {
	tr Transport
}

func (t *typeTransport[X]) Context() (ctx context.Context) {
	return t.tr.Context()
}

func (t *typeTransport[X]) Read() (x X, err error) {
	err = t.tr.ReadJSON(&x)
	return
}

func (t *typeTransport[X]) Write(x X) (err error) {
	return t.tr.WriteJSON(x)
}
