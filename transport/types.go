package transport

import (
	"context"
)

type Transport interface {
	Context() context.Context

	// ReadJSON reads JSON for the transport into the given pointer.
	ReadJSON(v any) error

	// WriteJSON writes JSON from the given data to the transport.
	WriteJSON(v any) error
}
