package doc

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHolder(t *testing.T) {
	var created, destroyed atomic.Int32

	conf := Config[string, *int]{
		Create: func(ctx context.Context, cancel context.CancelCauseFunc, key string) (*int, error) {
			created.Add(1)
			val := int(created.Load())
			return &val, nil
		},
		Destroy: func(ctx context.Context, key string, inst *int) error {
			destroyed.Add(1)
			return nil
		},
		ShutdownDelay: time.Millisecond * 10,
	}

	h := New(conf)

	// 1. First caller
	ctx1, cancel1 := context.WithCancel(t.Context())
	inst1, done1, err := h.For(ctx1, "foo")
	if err != nil {
		t.Fatalf("For failed: %v", err)
	}
	if *inst1 != 1 {
		t.Errorf("expected instance 1, got %d", *inst1)
	}

	// 2. Second caller (same key)
	ctx2, cancel2 := context.WithCancel(t.Context())
	inst2, done2, err := h.For(ctx2, "foo")
	if err != nil {
		t.Fatalf("For failed: %v", err)
	}
	if inst1 != inst2 {
		t.Errorf("expected same instance")
	}

	// 3. Cancel first caller, nothing should happen
	cancel1()
	select {
	case <-done1:
		// expected, as this caller is done (Wait, done1 is tied to active.halting?)
		// Let's check doc.go:
		// doneCh := make(chan error, 1)
		// go func() {
		// 	<-active.halting
		// 	doneCh <- active.err
		// }()
		// active.halting is closed when the *instance* is halting, not when the caller is done.
		// Wait, `For` returns `inst`, `done`, `err`.
		// If I cancel my context, `For` has already returned.
		// `done` channel signals when the instance is going away?
		t.Errorf("instance should not halt yet")
	case <-time.After(time.Millisecond * 20):
	}

	// 4. Cancel second caller, instance should halt after delay
	cancel2()
	select {
	case <-done2:
	case <-time.After(time.Millisecond * 100):
		t.Errorf("instance should halt")
	}

	if created.Load() != 1 {
		t.Errorf("expected 1 creation, got %d", created.Load())
	}
	if destroyed.Load() != 1 {
		t.Errorf("expected 1 destruction, got %d", destroyed.Load())
	}

	// 5. New caller, should create new instance
	ctx3, cancel3 := context.WithCancel(t.Context())
	inst3, _, err := h.For(ctx3, "foo")
	if err != nil {
		t.Fatalf("For failed: %v", err)
	}
	if *inst3 != 2 {
		t.Errorf("expected instance 2, got %d", *inst3)
	}
	cancel3()
}

func TestResurrection(t *testing.T) {
	var created, destroyed atomic.Int32

	conf := Config[string, *int]{
		Create: func(ctx context.Context, cancel context.CancelCauseFunc, key string) (*int, error) {
			created.Add(1)
			val := int(created.Load())
			return &val, nil
		},
		Destroy: func(ctx context.Context, key string, inst *int) error {
			destroyed.Add(1)
			return nil
		},
		ShutdownDelay: time.Millisecond * 50,
	}

	h := New(conf)

	ctx1, cancel1 := context.WithCancel(t.Context())
	inst1, done1, err := h.For(ctx1, "bar")
	if err != nil {
		t.Fatalf("For failed: %v", err)
	}

	// Cancel, triggers shutdown delay
	cancel1()

	// Immediately try to join again
	ctx2, cancel2 := context.WithCancel(t.Context())
	inst2, done2, err := h.For(ctx2, "bar")
	if err != nil {
		t.Fatalf("For failed: %v", err)
	}

	if inst1 != inst2 {
		t.Errorf("expected same instance (resurrected)")
	}

	// Wait for longer than shutdown delay to ensure it didn't die
	select {
	case <-done1:
		// done1 might fire because it was tied to the *previous* lifecycle attempt?
		// or is it tied to the active.halting?
		// In doc.go:
		// active.halting is closed when the instance is halting.
		// If we resurrected, the instance is NOT halting.
		// However, wait.
		// The `active.halting` channel is created when `activeDoc` is created.
		// If we resurrect, we reuse `activeDoc`.
		// But `activeDoc` has `halting` channel.
		// If `shutdown` was called...
		// In `doc.go`:
		// shutdown := func() { ... close(active.halting) ... }
		// And the CGroup Halt handler calls `shutdown()` AFTER the delay.
		// If we resumed `active.group.Halt`, the `shutdown()` is NOT called if we get `resume` signal.
		// So `active.halting` should NOT be closed.
		// So `done1` should NOT fire.
	case <-time.After(time.Millisecond * 100):
	}

	// Now cancel 2
	cancel2()
	select {
	case <-done2:
	case <-time.After(time.Millisecond * 100):
		t.Errorf("instance should halt")
	}

	if created.Load() != 1 {
		t.Errorf("expected 1 creation, got %d", created.Load())
	}
	if destroyed.Load() != 1 {
		t.Errorf("expected 1 destruction, got %d", destroyed.Load())
	}
}

func TestActiveIterator(t *testing.T) {
	h := New(Config[string, int]{
		Create: func(ctx context.Context, cancel context.CancelCauseFunc, key string) (int, error) {
			return 0, nil
		},
		Destroy: func(ctx context.Context, key string, inst int) error {
			return nil
		},
		ShutdownDelay: time.Millisecond * 10,
	})

	// Start iterator
	iterCtx, iterCancel := context.WithCancel(t.Context())
	defer iterCancel()

	next, stop := iter.Pull(h.Active(iterCtx, nil))
	defer stop()

	// Initial set should be empty
	change, ok := next()
	if !ok || len(change) != 0 {
		t.Errorf("expected empty initial set")
	}

	// Create a doc
	subCtx, subCancel := context.WithCancel(t.Context())
	_, _, err := h.For(subCtx, "foo")
	if err != nil {
		t.Fatal(err)
	}

	// Should see "foo": true
	change, ok = next()
	if !ok || !change["foo"] {
		t.Errorf("expected foo: true, got %v", change)
	}

	// Destroy doc
	subCancel()

	// Should see "foo": false
	change, ok = next()
	if !ok || change["foo"] {
		t.Errorf("expected foo: false, got %v", change)
	}
}

func TestErrorCreate(t *testing.T) {
	myErr := errors.New("my error")
	h := New(Config[string, int]{
		Create: func(ctx context.Context, cancel context.CancelCauseFunc, key string) (int, error) {
			return 0, myErr
		},
		Destroy: func(ctx context.Context, key string, inst int) error {
			return nil
		},
	})

	_, _, err := h.For(t.Context(), "fail")
	if !errors.Is(err, myErr) {
		t.Errorf("expected error %v, got %v", myErr, err)
	}
}

func TestSlowDestroy(t *testing.T) {
	var created atomic.Int32
	var once sync.Once
	inDestroy := make(chan struct{})

	conf := Config[string, *int]{
		Create: func(ctx context.Context, cancel context.CancelCauseFunc, key string) (*int, error) {
			created.Add(1)
			val := int(created.Load())
			return &val, nil
		},
		Destroy: func(ctx context.Context, key string, inst *int) error {
			once.Do(func() {
				close(inDestroy)
			})
			time.Sleep(time.Millisecond * 50)
			return nil
		},
		ShutdownDelay: 0,
	}

	h := New(conf)

	// 1. Create first instance
	ctx1, cancel1 := context.WithCancel(t.Context())
	inst1, _, err := h.For(ctx1, "slow")
	if err != nil {
		t.Fatalf("For failed: %v", err)
	}

	// 2. Trigger shutdown
	cancel1()

	// 3. Wait for Destroy to start
	select {
	case <-inDestroy:
	case <-time.After(time.Millisecond * 100):
		t.Fatal("timed out waiting for Destroy to start")
	}

	// 4. Immediately try to join again. This should block until Destroy finishes and then create a NEW instance.
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()

	inst2, _, err := h.For(ctx2, "slow")
	if err != nil {
		t.Fatalf("For failed: %v", err)
	}

	if inst1 == inst2 {
		t.Errorf("expected different instances, got same pointer %p", inst1)
	}
	if *inst2 != 2 {
		t.Errorf("expected instance 2, got %d", *inst2)
	}
}

func TestSelfCancel(t *testing.T) {
	connCancel := make(chan context.CancelCauseFunc, 1)

	h := New(Config[string, int]{
		Create: func(ctx context.Context, cancel context.CancelCauseFunc, key string) (int, error) {
			connCancel <- cancel
			return 123, nil
		},
		Destroy: func(ctx context.Context, key string, inst int) error {
			return nil
		},
	})

	ctx, cancelCtx := context.WithCancel(t.Context())
	defer cancelCtx()

	_, done, err := h.For(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}

	cancel := <-connCancel
	myErr := errors.New("foobar")
	cancel(myErr)

	select {
	case err := <-done:
		if !errors.Is(err, myErr) {
			t.Errorf("expected error %v, got %v", myErr, err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for done")
	}
}

func TestSlowCreateCancel(t *testing.T) {
	conf := Config[string, *int]{
		Create: func(ctx context.Context, cancel context.CancelCauseFunc, key string) (*int, error) {
			select {
			case <-time.After(time.Second):
				val := 1
				return &val, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		Destroy: func(ctx context.Context, key string, inst *int) error {
			return nil
		},
	}
	h := New(conf)

	ctx, cancel := context.WithCancel(t.Context())

	// Start For in a goroutine because it blocks
	errCh := make(chan error)
	go func() {
		_, _, err := h.For(ctx, "slow")
		errCh <- err
	}()

	time.Sleep(10 * time.Millisecond) // Give it time to enter Create
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error, got nil")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for For to return")
	}
}
