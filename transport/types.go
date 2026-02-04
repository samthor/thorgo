package transport

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// TransportError is a custom error that is sent to the client as a JSON object
// inside the close reason, with a specific status code.
type TransportError struct {
	Code   int
	Reason string
}

// Encode encodes this TransportError to string.
func (e TransportError) Encode() (out string) {
	return fmt.Sprintf("%d/%s", e.Code, e.Reason)
}

// DecodeTransportError decodes a previously encoded TransportError.
func DecodeTransportError(raw string) (err TransportError) {
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 {
		return
	}
	err.Code, _ = strconv.Atoi(parts[0])
	err.Reason = parts[1]
	return
}

func (e TransportError) Error() (reason string) {
	return e.Encode()
}

type Transport interface {
	// Context returns the underlying Context for the physical transport, e.g., of the HTTP connection.
	Context() (ctx context.Context)

	// ReadJSON reads JSON from the transport into the given target.
	// It will also return an error if the underlying Transport has shut down.
	ReadJSON(v any) (err error)

	// WriteJSON writes JSON from the given data to the transport.
	// Implementations may queue sends and never return an error.
	WriteJSON(v any) (err error)
}

// Handler runs a Transport.
type Handler func(tr Transport) (err error)
