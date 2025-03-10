package lgroup

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStartLGroup(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())

	g, start := NewLGroup[string](cancel)
	select {
	case <-g.Done():
		t.Errorf("should not be immediately released")
	default:
	}

	start()

	select {
	case <-g.Done():
	default:
		t.Errorf("should die immediately")
	}
	select {
	case <-ctx.Done():
		t.Errorf("context should not cancel, was non-nil err")
	case <-time.After(time.Millisecond):
	}
}

func TestRegisterError(t *testing.T) {
	expectedErr := errors.New("lol")

	ctx, cancel := context.WithCancelCause(t.Context())

	g, start := NewLGroup[string](cancel)
	select {
	case <-g.Done():
		t.Errorf("should not be immediately released")
	default:
	}

	userCtx, userCtxCancel := context.WithTimeout(t.Context(), time.Second)
	defer userCtxCancel()
	g.Join(userCtx, "hello")
	start()

	g.Register(func(ctx context.Context, s string) error {
		return expectedErr
	})

	select {
	case <-ctx.Done():
		if context.Cause(ctx) != expectedErr {
			t.Errorf("unexpected err: %v", context.Cause(ctx))
		}
	case <-g.Done():
		t.Errorf("should fail ctx first")
	}
}

func TestLGroup(t *testing.T) {
	_, cancel := context.WithCancelCause(t.Context())

	g, start := NewLGroup[string](cancel)

	userCtx, userCtxCancel := context.WithTimeout(t.Context(), time.Second)
	defer userCtxCancel()
	g.Join(userCtx, "hello")
	start()

	select {
	case <-g.Done():
		t.Errorf("should not be released with joined handler")
	default:
	}

	userCtxCancel()
	select {
	case <-g.Done():
	case <-time.After(time.Millisecond * 4): // not immediate
		t.Errorf("should be done after userCtx cancelled")
	}
}

func TestRegister(t *testing.T) {
	_, cancel := context.WithCancelCause(t.Context())
	g, start := NewLGroup[string](cancel)

	firstRegisterCh := make(chan struct{})
	g.Register(func(ctx context.Context, s string) error {
		close(firstRegisterCh)
		return nil
	})

	select {
	case <-time.After(time.Millisecond * 2):
	case <-firstRegisterCh:
		t.Errorf("should not trigger - no-one registered")
	}

	userCtx, userCtxCancel := context.WithTimeout(t.Context(), time.Second)
	defer userCtxCancel()
	g.Join(userCtx, "hello")
	start()

	select {
	case <-time.After(time.Millisecond * 4):
		t.Errorf("should be active")
	case <-firstRegisterCh:
	}

	secondRegisterCh := make(chan struct{})
	g.Register(func(ctx context.Context, s string) error {
		close(secondRegisterCh)
		return nil
	})
	select {
	case <-time.After(time.Millisecond * 4):
		t.Errorf("should be active")
	case <-secondRegisterCh:
	}
}
