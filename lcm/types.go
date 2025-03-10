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

type Status interface {
	// After registers a shutdown function to run after this Status completes.
	// In normal operation, the context provided when the Manager built this object will still be alive.
	// The returned stop function has the same semantics as [context.AfterFunc].
	// The passed function will block a manager recreating the managed object; be sure not to deadlock.
	After(func() error) (stop func() bool)

	// Check passes through the given error. If it is non-nil, it cancels this managed object.
	Check(error) error

	// CheckWrap passes through the given error. If it is non-nil, it cancels this managed object with a wrapped error ("%s: %w").
	CheckWrap(string, error) error
}

// BuildFunc can build a managed object, or fail immediately.
type BuildFunc[Key comparable, Object any] func(Key, Status) (Object, error)
