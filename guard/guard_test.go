package guard

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestGuard(t *testing.T) {
	var checkCalls atomic.Int32

	g := NewGuard(t.Context(), func(ctx context.Context, key string, all []string) (use []string, err error) {
		checkCalls.Add(1)
		return all, nil
	})

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

}
