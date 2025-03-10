package lcm

import (
	"context"
)

// Manager is a "life-cycle manager", which allows keyed creation -> use -> shutdown.
type Manager[Key comparable, Object, Init any] interface {
	// Run creates/joins the lifecycle of an object with the given key.
	// Returns an error if it could not be created - this is "sticky" while interested.
	// Otherwise, returns the object and its run context (detached).
	Run(context.Context, Key, Init) (Object, context.Context, error)
}

type Status[Init any] interface {
	Context() context.Context

	// JoinTask registers a join function to handle join lifecycle.
	// This is run in its own goroutine and blocks shutdown until completion; a normal use is to wait on [context.Context.Done] and do cleanup tasks once done.
	// All registered methods may run in parallel.
	// Notably this ensures the managed object stays alive during context shutdown: [context.AfterFunc] doesn't do this on its own.
	JoinTask(func(context.Context, Init) error)

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
type BuildFunc[Key comparable, Object, Init any] func(Key, Status[Init]) (Object, error)

// TaskFunc runs for the overall runtime of the status object.
type TaskFunc func(stop <-chan struct{}) error
