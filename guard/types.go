package guard

import (
	"context"
	"time"
)

// CheckFunc is called when a session needs to be validated (either new or refresh: it shouldn't matter).
// This must return the subset of tokens which allow access to this credential, and must be at least one.
type CheckFunc[Token comparable, Key any] func(ctx context.Context, key Key, all []Token) (use []Token, err error)

type Guard[Token comparable, Key any] interface {
	// ProvideToken provides a token which expires when the provided channel is closed.
	ProvideToken(t Token, shutdown <-chan struct{})

	// ProvideTokenExpiry provides a token which expires in the future.
	// Calls with a matching token are allowed, and the updated expiry will always be used.
	// It internally calls ProvideToken.
	ProvideTokenExpiry(t Token, expiry time.Time)

	// RunSession creates a session which is dependent on at least one token being valid.
	// Key is just passed to CheckFunc; it is not actually a map key, many sessions can start for the same key.
	// The channel within Session[Token] must be read for token updates until closed, or the Guard will deadlock.
	// It is closed when there are no more valid tokens (including immediately).
	RunSession(ctx context.Context, key Key) (Session[Token], error)
}

type Session[Token any] interface {
	// Context provides a derived context which cancels when tokens are no longer valid.
	Context() context.Context

	// Stop shuts down this session and cancels the related context.
	Stop()

	// UpdateCh returns a chan written in a non-blocking way when tokens change.
	UpdateCh() <-chan struct{}

	// Tokens returns the current tokens used for this Session.
	Tokens() []Token
}
