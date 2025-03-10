// Package transport deals with packet-like connections between endpoints.
// It includes helpers to create them from sockets or as derived concepts.
package transport

import (
	"context"
)

type Transport interface {
	// Read reads the next message available into the given target, e.g., by decoding.
	Read(any) error

	// Send sends the given message.
	// This may block, or not, and may always return nil (e.g., message put into queue rather than sent directly).
	Send(any) error

	// Context returns a context which is Done when the underlying connection has closed.
	Context() context.Context
}
