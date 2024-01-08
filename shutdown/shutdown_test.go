package shutdown

import (
	"testing"
	"time"
)

func TestLazyShutdown(t *testing.T) {
	g := New(time.Millisecond * 5)

	g.Lock()
	time.Sleep(time.Millisecond * 5)

	if g.IsDone() {
		t.Errorf("should not be done")
	}

	g.Unlock()
	time.Sleep(time.Millisecond * 10)
	if !g.IsDone() {
		t.Errorf("should be done")
	}
}
