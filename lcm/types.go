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

	// SetTimeout sets the future timeout, once no callers are interested, for objects created here.
	SetTimeout(time.Duration)
}

type Status interface {
	Context() context.Context

	// Task creates an immediate task that is started in a goroutine after the object is successfully created.
	// It will be deferred until the builder function first returns (or not called if this errors).
	// If/when it returns a non-nil error, this managed object will be cancelled.
	// After cleanups wait for all tasks to return before starting.
	Task(TaskFunc)

	// After registers a shutdown function to run after this Status completes.
	// These functions will only be called in normal shutdown; if the runnable [context.Context] is cancelled, they will not be called.
	// Additionally, if any function returns a non-nil error, no further shutdown functions will run.
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

type TaskFunc func(stop <-chan struct{}) error
