package lcm

import (
	"context"
	"time"
)

// Manager is a "life-cycle manager", which allows keyed creation -> use -> shutdown.
type Manager[Key comparable, Object any] interface {

	// Run creates/joins the lifecycle of an object with the given key.
	// Returns an error if it could not be created - this is "sticky" while interested.
	// Otherwise, returns the object and its run context (detached).
	Run(context.Context, Key) (Object, context.Context, error)
	SetTimeout(time.Duration)
}

// Build is passed to BuildFn for build operations.
type Build[Key comparable] struct {
	Key    Key
	C      context.Context
	Cancel context.CancelCauseFunc
}

// BuildFn can build a resulting done-able object or fail.
type BuildFn[Key comparable, Object any] func(Build[Key]) (Object, error)

// Objects used within Manager can optionally implement HasShutdown.
// If so, Shutdown is called on normal shutdown (no-one is interested in object) before the context is cancelled.
// This method is not called in a failure mode (listen to the context yourself).
type HasShutdown interface {
	Shutdown()
}

// Objects used within Manager can optionally implement HasShutdownError.
// If so, Shutdown is called on normal shutdown (no-one is interested in object) before the context is cancelled.
// The returned error is used to cancel the context (nil just means cancelled normally).
// This method is not called in a failure mode (listen to the context yourself).
type HasShutdownError interface {
	Shutdown() error
}
