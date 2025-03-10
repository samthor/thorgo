package future

import (
	"context"
	"errors"
	"testing"
)

func TestFuture(t *testing.T) {
	ctx := t.Context()

	nestedCtx, cancel := context.WithCancel(ctx)
	f, resolve := New[int]()

	cancel()
	<-nestedCtx.Done() // TODO: sometimes we need to block for this???

	_, err := f.Wait(nestedCtx)
	if err != context.Canceled {
		t.Errorf("expected Canceled to pass through, was: %v", err)
	}

	resolve(123, nil)
	_, err = f.Wait(nestedCtx)
	if err != context.Canceled {
		t.Errorf("expected Canceled to be triggered first, was: %v", err)
	}

	val, err := f.Wait(ctx)
	if err != nil {
		t.Errorf("expected nil err, was: %v", err)
	}
	if val != 123 {
		t.Errorf("value was not expected, got: %v", val)
	}

	resolve(456, errors.New("lol"))
	res, err, ok := f.Sync()
	if !ok || err != nil || res != 123 {
		t.Errorf("second resolve should have no effect")
	}
}
