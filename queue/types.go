package queue

import (
	"context"
)

type Queue[X any] interface {
	// Push adds more events to the queue.
	// All subscribers currently waiting will recieve at least one event before this method returns.
	// Returns true if any subscribers woke up.
	Push(all ...X) bool

	// Join returns a listener that provides all events passed with Push after this call completes.
	// If the context is cancelled, the listener becomes invalid and returns no/empty values.
	Join(ctx context.Context) Listener[X]
}

type Listener[X any] interface {
	// Peek determines if there's a pending queue event, returning it if available.
	// This returns the zero X and false if there is no event or this listener is invalid.
	// It does not consume the event.
	Peek() (X, bool)

	// Next waits for and returns the next queue event.
	// It returns the zero X and false if this listener is invalid/cancelled context.
	Next() (X, bool)

	// Batch waits for and returns a slice of all available queue events.
	// If the returned slice is nil or has zero length, this listener is invalid/cancelled context.
	Batch() []X
}
