package cgroup

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWithContext(t *testing.T) {
	cg := New()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cg.Add(ctx)

	cgCtx := cg.Start()
	select {
	case <-cgCtx.Done():
		t.Fatalf("context should not be done")
	default:
	}

	cancel()

	if cg.Start() != cgCtx {
		t.Errorf("should get same context for Start")
	}
	select {
	case <-cgCtx.Done():
	case <-time.After(time.Millisecond):
		t.Fatalf("context should not be alive")
	}

	if context.Cause(cgCtx) != context.Canceled {
		t.Errorf("bad cause")
	}
}

func TestNoContext(t *testing.T) {
	err := fmt.Errorf("lol")

	cg := NewCause(err)
	cgCtx := cg.Start()
	select {
	case <-cgCtx.Done():
	default:
		t.Fatalf("context should not be alive")
	}

	if context.Cause(cgCtx) != err {
		t.Errorf("bad cause")
	}
}
