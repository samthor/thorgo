package lifecycle

import (
	"context"
	"errors"
	"iter"
	"testing"
)

func TestWorker(t *testing.T) {
	dataCh := make(chan int, 5)
	expectedErr := errors.New("lol")
	dataCh <- 1
	dataCh <- 2
	close(dataCh)

	w := Worker(t.Context(), dataCh, func(ctx context.Context, events iter.Seq[int]) (err error) {
		return expectedErr
	})

	err := <-w.Done()
	if err != expectedErr {
		t.Errorf("bad err: %v", err)
	}

	select {
	case <-w.Ready():
		t.Errorf("was never ready: never read iterator")
	default:
	}
}

func TestWorkerOrder(t *testing.T) {
	dataCh := make(chan int)
	proceed := make(chan struct{})

	w := Worker(t.Context(), dataCh, func(ctx context.Context, events iter.Seq[int]) (err error) {
		for range events {
			<-proceed
			return nil
		}
		return nil
	})

	// 1. Trigger start
	dataCh <- 1

	// 2. Wait for Ready
	<-w.Ready()

	// 3. Ensure not Idle/Done
	idleCh := w.Idle()
	select {
	case <-idleCh:
		t.Fatal("Idle closed too early")
	case <-w.Done():
		t.Fatal("Done closed too early")
	default:
	}

	// 4. Finish
	close(proceed)

	// 5. Idle should fire
	if val := <-idleCh; !val {
		t.Errorf("Idle=false, want true")
	}

	// 6. Done should fire
	if err := <-w.Done(); err != nil {
		t.Errorf("Done err=%v, want nil", err)
	}
}

func TestWorkerUnused(t *testing.T) {
	dataCh := make(chan int)
	w := Worker(t.Context(), dataCh, func(ctx context.Context, events iter.Seq[int]) (err error) {
		return nil // never use events
	})

	// Done should fire eventually
	if err := <-w.Done(); err != nil {
		t.Errorf("Done err=%v, want nil", err)
	}

	// Ready should never have fired
	select {
	case <-w.Ready():
		t.Error("Ready should not be closed")
	default:
	}

	// Idle should fire and be false
	if wasReady := <-w.Idle(); wasReady {
		t.Error("Idle should be false (never ready)")
	}
}
