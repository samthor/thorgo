package guard

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGuard(t *testing.T) {
	var checkCalls atomic.Int32

	g := NewGuard(t.Context(), func(ctx context.Context, key string, all []string) (use []string, err error) {
		checkCalls.Add(1)
		return all, nil
	})

	// #1: check zero tokens case
	s, err := g.RunSession("butt")
	if err != nil {
		t.Errorf("zero tokens must not err")
	}
	select {
	case _, ok := <-s.TokenCh():
		if ok {
			t.Errorf("no token must be available")
		}
	default:
		t.Errorf("tokenCh must be closed")
	}
	if checkCalls.Load() != 0 {
		t.Errorf("expected no check calls (no tokens!)")
	}

	// #2: check two tokens + migrate to third
	t1 := make(chan struct{})
	g.ProvideToken("t1", t1)

	t2 := make(chan struct{})
	g.ProvideToken("t2", t2)

	s, err = g.RunSession("butt")
	if err != nil {
		t.Errorf("must not err")
	}
	if checkCalls.Load() != 1 {
		t.Errorf("expected one check call")
	}
	for range 2 {
		select {
		case <-s.TokenCh():
		default:
			t.Errorf("expected 2 initial tokens")
		}
	}
	select {
	case <-s.TokenCh():
		t.Errorf("should not be more tokens at start")
	default:
	}

	// remove single token
	close(t1)
	if checkCalls.Load() != 1 {
		t.Errorf("expected one check call")
	}

	// provide new token, close second
	t3 := make(chan struct{})
	g.ProvideToken("t3", t3)
	if actual := checkCalls.Load(); actual != 1 {
		t.Errorf("expected no more check calls: %v", actual)
	}

	close(t2)
	time.Sleep(time.Millisecond)

	if actual := checkCalls.Load(); actual != 2 {
		t.Errorf("expected another check call, was: %v", actual)
	}

	select {
	case actual := <-s.TokenCh():
		if actual != "t3" {
			t.Errorf("expected token t3, was: %s", actual)
		}
	default:
		t.Errorf("should be a new token (but no tokens available)")
	}

	close(t3)

	select {
	case _, ok := <-s.TokenCh():
		if ok {
			t.Errorf("expected token ch close")
		}
	case <-time.After(time.Millisecond):
		t.Errorf("ch should be closed")
	}
}
