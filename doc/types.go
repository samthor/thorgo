package doc

import (
	"context"
	"iter"
	"time"
)

type Config[K comparable, V any] struct {
	// Create controls creating this V.
	// This is passed a cancel function which causes a shutdown of the context wrapping the V.
	// Use this for errors e.g., in the background task of this V.
	Create func(ctx context.Context, cancel context.CancelCauseFunc, key K) (inst V, err error)

	// Destroy controls destroying this V.
	// This is called when there is no hope of restoring the object.
	Destroy func(ctx context.Context, key K, inst V) (err error)

	// ShutdownDelay controls how long to keep the V alive after all clients disappear.
	ShutdownDelay time.Duration
}

// Holder describes a keyed storage of instance objects.
type Holder[K comparable, V any] interface {
	// For joins the instance with the given comparable Key while the given context is active.
	For(ctx context.Context, key K) (inst V, done <-chan error, err error)

	// Active returns an iterator which yields document changes over time.
	// The first yield will always contain the current set of active documents, even if that set is empty.
	Active(ctx context.Context, filter func(key K) (include bool)) (i iter.Seq[Action[K]])
}

// Action describes docs that are loaded (true) or unloaded (false).
type Action[K comparable] map[K]bool
