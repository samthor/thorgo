package guard

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestGuard(t *testing.T) {
	var checkCalls atomic.Int32

	g := New(func(ctx context.Context, key string, all []string) (use []string, err error) {
		checkCalls.Add(1)
		return all, nil
	})

	// #1: check zero tokens case
	s, err := g.RunSession(t.Context(), "butt")
	if err != nil {
		t.Errorf("zero tokens must not err")
	}
	defer s.Stop()

	select {
	case _, ok := <-s.UpdateCh():
		if ok {
			t.Errorf("no token must be available")
		}
	default:
		t.Errorf("tokenCh must be closed")
	}
	if checkCalls.Load() != 1 {
		t.Errorf("expected single check call (even with no tokens)")
	}

	// #2: check two tokens + migrate to third
	t1 := make(chan struct{})
	g.ProvideToken("t1", t1)

	t2 := make(chan struct{})
	g.ProvideToken("t2", t2)

	s, err = g.RunSession(t.Context(), "butt")
	if err != nil {
		t.Errorf("must not err")
	}
	defer s.Stop()
	if checkCalls.Load() != 2 {
		t.Errorf("expected one check call")
	}
	select {
	case <-s.UpdateCh():
	default:
		t.Errorf("expected initial tokens")
	}
	select {
	case <-s.UpdateCh():
		t.Errorf("should not be more tokens at start")
	default:
	}

	// remove single token
	close(t1)
	if checkCalls.Load() != 2 {
		t.Errorf("expected one check call")
	}

	// provide new token, close second
	t3 := make(chan struct{})
	g.ProvideToken("t3", t3)
	if actual := checkCalls.Load(); actual != 2 {
		t.Errorf("expected no more check calls: %v", actual)
	}

	close(t2)
	time.Sleep(time.Millisecond)

	if actual := checkCalls.Load(); actual != 3 {
		t.Errorf("expected another check call, was: %v", actual)
	}

	select {
	case <-s.UpdateCh():
		actual := s.Tokens()
		if !reflect.DeepEqual(actual, []string{"t3"}) {
			t.Errorf("expected token t3, was: %s", actual)
		}
	default:
		t.Errorf("should be a new token (but no tokens available)")
	}

	close(t3)

	select {
	case _, ok := <-s.UpdateCh():
		if ok {
			t.Errorf("expected token ch close")
		}
	case <-time.After(time.Millisecond):
		t.Errorf("ch should be closed")
	}
}
