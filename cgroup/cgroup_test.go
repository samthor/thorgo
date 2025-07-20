package cgroup

import (
	"context"
	"fmt"
	"sync/atomic"
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

func TestGo(t *testing.T) {
	cg := New()

	var started bool
	var run atomic.Int32

	cg.Go(func(c context.Context) error {
		if started == false {
			t.Errorf("should not have been started")
		}
		run.Add(1)
		return nil
	})

	time.Sleep(time.Millisecond)
	if run.Load() != 0 {
		t.Errorf("should not have run")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cg.Add(ctx)

	time.Sleep(time.Millisecond)
	if run.Load() != 0 {
		t.Errorf("should not have run")
	}

	started = true
	cg.Start()
	time.Sleep(time.Millisecond)
	if run.Load() != 1 {
		t.Errorf("should have run")
	}
}
