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

// BuildFn can build a resulting done-able object or fail.
type BuildFn[Key comparable, Object any] func(Key, Status) (Object, error)

type Status interface {
	// After registers a shutdown function to run after this Status completes.
	// In normal operation, it is called with a still-valid [context.Context].
	// The returned stop function has the same semantics as [context.AfterFunc].
	After(func() error) (stop func() bool)

	// Check passes through the given error. If it is non-nil, it cancels this managed object.
	Check(error) error

	// CheckWrap passes through the given error. If it is non-nil, it cancels this managed object with a wrapped error ("%s: %w").
	CheckWrap(string, error) error
}
