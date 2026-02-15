package lifecycle

import (
	"context"
	"errors"
	"iter"
	"testing"
)

func TestFoo(t *testing.T) {
	dataCh := make(chan int, 5)
	expectedErr := errors.New("lol")
	dataCh <- 1
	dataCh <- 2
	close(dataCh)

	f := RunFoo(t.Context(), dataCh, func(ctx context.Context, events iter.Seq[int]) (err error) {
		return expectedErr
	})

	err := <-f.Done()
	if err != expectedErr {
		t.Errorf("bad err: %v", err)
	}

	select {
	case <-f.Ready():
		t.Errorf("was never ready: never read iterator")
	default:
	}
}
