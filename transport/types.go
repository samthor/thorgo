package transport

import (
	"context"
	"fmt"
)

type Transport interface {
	Context() context.Context

	// ReadJSON reads JSON for the transport into the given pointer.
	ReadJSON(v any) error

	// WriteJSON writes JSON from the given data to the transport.
	WriteJSON(v any) error
}

type TransportError struct {
	Code   int    // codes should be 0-1000
	Reason string // string reason
}

func (te TransportError) Error() string {
	return fmt.Sprintf("status = %v and reason = %q", te.Code, te.Reason)
}
