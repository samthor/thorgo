package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTGroupBasic(t *testing.T) {
	tg := NewTGroup[string]()
	ctx1, cancel1 := context.WithCancel(t.Context())
	defer cancel1()

	tg.Provide("a", ctx1)

	// Access with nil check should pass if any token exists
	accessCtx := tg.Access(t.Context(), nil)
	select {
	case <-accessCtx.Done():
		t.Fatal("Access should be valid")
	default:
	}

	// Revoke should cancel access
	tg.Revoke("a")
	select {
	case <-accessCtx.Done():
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Access should be revoked")
	}
}

func TestTGroupAccessCheck(t *testing.T) {
	tg := NewTGroup[string]()
	tg.Provide("a", t.Context())
	tg.Provide("b", t.Context())

	check := func(ctx context.Context, token string) error {
		if token == "b" {
			return nil
		}
		return context.Canceled
	}

	accessCtx := tg.Access(t.Context(), check)
	select {
	case <-accessCtx.Done():
		t.Fatal("Access should be valid for token b")
	default:
	}

	tg.Revoke("b")
	select {
	case <-accessCtx.Done():
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Access should be revoked after b is gone")
	}
}

func TestTGroupExpiration(t *testing.T) {
	tg := NewTGroup[string]()
	ctx, cancel := context.WithCancel(t.Context())

	tg.Provide("a", ctx)
	accessCtx := tg.Access(t.Context(), nil)

	cancel()
	select {
	case <-accessCtx.Done():
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Access should be revoked after context expires")
	}
}

func TestTGroupMultipleTokens(t *testing.T) {
	tg := NewTGroup[string]()
	tg.Provide("a", t.Context())
	tg.Provide("b", t.Context())

	accessCtx := tg.Access(t.Context(), nil)

	tg.Revoke("a")
	select {
	case <-accessCtx.Done():
		t.Fatal("Access should still be valid (b exists)")
	case <-time.After(time.Millisecond * 50):
	}

	tg.Revoke("b")
	select {
	case <-accessCtx.Done():
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Access should be revoked now")
	}
}

func TestTGroupUnion(t *testing.T) {
	tg := NewTGroup[string]()
	ctx1, cancel1 := context.WithCancel(t.Context())
	ctx2, cancel2 := context.WithCancel(t.Context())

	tg.Provide("a", ctx1)
	tg.Provide("a", ctx2)

	accessCtx := tg.Access(t.Context(), nil)

	cancel2()
	// If union works, accessCtx should still be valid because ctx1 is still alive.
	select {
	case <-accessCtx.Done():
		t.Fatal("Access should still be valid (ctx1 is alive)")
	case <-time.After(time.Millisecond * 50):
	}

	cancel1()
	select {
	case <-accessCtx.Done():
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Access should be revoked now")
	}
}

func TestTGroupFail(t *testing.T) {
	tg := NewTGroup[string]()

	// Check that we get an immediately failed context with no tokens.
	c1 := tg.Access(t.Context(), nil)
	if c1.Err() == nil {
		t.Errorf("should have never been active")
	}

	// Check that the context is failed with a token that doesn't pass the test.
	tg.Provide("a", t.Context())
	var invoked int
	c2 := tg.Access(t.Context(), func(ctx context.Context, token string) (err error) {
		invoked++
		return context.Canceled
	})
	if c2.Err() == nil {
		t.Errorf("should have never been active")
	}
	if invoked != 1 {
		t.Errorf("invoked should have been called once, was: %d", invoked)
	}

	// Check that we fail with the parent context if it's already failed.
	expectedErr := errors.New("expected")
	failedCtx, fail := context.WithCancelCause(t.Context())
	fail(expectedErr)

	c3 := tg.Access(failedCtx, nil)
	if context.Cause(c3) != expectedErr {
		t.Errorf("expected known err, was: %v (wanted=%v)", c3.Err(), expectedErr)
	}
}
