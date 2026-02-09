package transport

import (
	"context"
	"testing"
)

func TestTypeTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, r := NewBufferPair(ctx, 1)

	type myData struct {
		Foo string `json:"foo"`
		Bar int    `json:"bar"`
	}

	lt := NewTyped[myData](l)
	rt := NewTyped[myData](r)

	expected := myData{Foo: "hello", Bar: 123}

	go func() {
		if err := lt.Write(expected); err != nil {
			t.Errorf("failed to write: %v", err)
		}
	}()

	got, err := rt.Read()
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	if got != expected {
		t.Errorf("expected %+v, got %+v", expected, got)
	}
}
