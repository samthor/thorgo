package context

import (
	"context"
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {

	g := NewTimeoutGroup(time.Millisecond * 5)
	if g.IsDone() {
		t.Errorf("expected immediate alive")
	}

	time.Sleep(time.Millisecond * 2)
	if g.IsDone() {
		t.Errorf("expected 2ms alive")
	}

	ctx, cancel := context.WithCancel(context.Background())
	if !g.Add(ctx) {
		t.Errorf("expected successful add")
	}

	time.Sleep(time.Millisecond * 4)
	if g.IsDone() {
		t.Errorf("expected timeout cancelled")
	}

	cancel()
	if g.IsDone() {
		t.Errorf("expected still alive")
	}

	time.Sleep(time.Millisecond * 10)
	if !g.IsDone() {
		t.Errorf("expected group DONE!")
	}
}
