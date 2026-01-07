package transport

import (
	"context"
	"testing"
	"time"
)

func TestPair(t *testing.T) {
	left, right := NewBufferPair(t.Context(), 1)

	left.WriteJSON("hello")

	var out string
	right.ReadJSON(&out)

	if out != "hello" {
		t.Errorf("bad send")
	}
}

func TestPairClose(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	left, right := NewBufferPair(ctx, 1)
	left.WriteJSON("hello")

	hasSent := make(chan struct{})
	go func() {
		left.WriteJSON("there")
		close(hasSent)
	}()

	select {
	case <-time.After(time.Millisecond):
	case <-hasSent:
		t.Fatalf("should not send")
	}

	var out string
	right.ReadJSON(&out)
	if out != "hello" {
		t.Errorf("bad send")
	}

	cancel()
	<-time.After(time.Millisecond) // let cancelation handler run
	err := right.ReadJSON(&out)
	if err == nil {
		t.Errorf("expected closed, was: out=%v %v", out, err)
	}
}
