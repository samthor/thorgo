package queue

import (
	"context"
	"iter"
	"time"
)

type Queue[X any] interface {
	// Push adds more events to the queue.
	// All subscribers currently waiting will recieve at least one event before this method returns.
	// Returns true if any subscribers woke up.
	Push(all ...X) (awoke bool)

	// Join returns a listener that provides all events passed with Push after this call completes.
	// If the context is cancelled, the listener becomes invalid and returns no/empty values.
	Join(ctx context.Context) (l Listener[X])

	// Pull builds a new PullFn, bound by the given context,  for this Queue.
	Pull(ctx context.Context) (fn PullFn[X])
}

// PullFn is a simple method which pulls events from the Queue within the duration.
// If the duration is negative, waits forever (or until the context dies).
// If the duration expires, returns zero but ok.
// If the internal context has failed, returns non-ok and a nil array.
type PullFn[X any] func(d time.Duration) (more []X, ok bool)

type Listener[X any] interface {
	// Peek determines if there's a pending queue event, returning it if available.
	// This returns the zero X and false if there is no event or this listener is invalid.
	// It does not consume the event.
	Peek() (x X, has bool)

	// Next waits for and returns the next queue event.
	// It returns the zero X and false if this listener is invalid/cancelled context.
	Next() (x X, ok bool)

	// Batch waits for and returns a slice of all available queue events.
	// If the returned slice is nil or has zero length, this listener is invalid/cancelled context.
	Batch() (all []X)

	// Iter returns an iterator that internally calls Next.
	Iter() (it iter.Seq[X])

	// BatchIter returns an iterator that internally calls Batch.
	BatchIter() (it iter.Seq[[]X])

	// Context returns the context that this Listener was created with.
	Context() (ctx context.Context)
}
