package lcm

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type FakeObject struct {
	shutdownCount *atomic.Int32
	name          string
}

func TestManager(t *testing.T) {
	var activeCount atomic.Int32
	var shutdownCount atomic.Int32

	m := NewWithContext(t.Context(), func(b string, s Status[any]) (FakeObject, error) {
		if b == "" {
			return FakeObject{}, errors.New("empty name")
		}

		fo := FakeObject{
			shutdownCount: &shutdownCount,
			name:          b,
		}

		activeCount.Add(1)
		time.Sleep(time.Microsecond)

		s.After(func() error {
			activeCount.Add(-1)
			return nil
		})

		s.After(func() error {
			shutdownCount.Add(1)
			return nil
		})

		// check that cleanup works by removing it immediately
		cleanup := s.After(func() error {
			activeCount.Add(-100)
			return nil
		})
		cleanup()

		return fo, nil
	})

	userCtx, cancel := context.WithCancel(t.Context())
	fo, _, err := m.Run(userCtx, "butt", nil)
	if err != nil {
		t.Errorf("got err creating obj: %v", err)
	}
	if fo.name != "butt" {
		t.Errorf("did not get key (default value?)")
	}
	if v := activeCount.Load(); v != 1 {
		t.Errorf("should have one active now, was: %d", v)
	}
	cancel()

	time.Sleep(time.Millisecond)

	if v := activeCount.Load(); v != 0 {
		t.Errorf("should have zero active now, was: %d", v)
	}
	if v := shutdownCount.Load(); v != 1 {
		t.Errorf("should have one shutdown now, was: %d", v)
	}

	// check failure mode
	userCtx, cancel = context.WithCancel(t.Context())
	_, runCtx, err := m.Run(userCtx, "", nil)
	if err == nil {
		t.Errorf("expected non-nil err: %v", err)
	}
	if context.Cause(runCtx) != err {
		t.Errorf("expected ctx to match err: runCtx.Err()=%v err=%+v", runCtx.Err(), err)
	}
	cancel()
}

type RaceShutdown struct {
	releaseShutdownCh chan struct{}
	inst              int
}

func TestManagerShutdownRace(t *testing.T) {
	var inst int

	m := NewWithContext(t.Context(), func(b string, s Status[any]) (*RaceShutdown, error) {
		inst++

		releaseShutdownCh := make(chan struct{})
		s.After(func() error {
			<-releaseShutdownCh
			return nil
		})

		return &RaceShutdown{
			releaseShutdownCh: releaseShutdownCh,
			inst:              inst,
		}, nil
	})

	//
	userCtx1, cancel1 := context.WithCancel(t.Context())
	rs1, runCtx1, _ := m.Run(userCtx1, "foo", nil)
	if rs1.inst != 1 {
		t.Errorf("unexpected seq, wanted 1 was=%v", rs1.inst)
	}
	cancel1()
	time.Sleep(time.Millisecond * 10)

	userCtx2, cancel2 := context.WithTimeout(t.Context(), time.Millisecond*100)
	defer cancel2()

	// we won't be able to join here: userCtx2 expires in 100ms _but_ we need to close the shutdownCh first
	rs1a, _, err := m.Run(userCtx2, "foo", nil)
	if err == nil || rs1a != nil {
		t.Errorf("should have timed out join, err was=%v", err)
	}
	close(rs1.releaseShutdownCh)

	userCtx3, cancel3 := context.WithCancel(t.Context())
	rs2, _, _ := m.Run(userCtx3, "foo", nil)
	if rs2.inst != 2 {
		t.Fatalf("rs2 not valid, should have inst=2: %v", rs2.inst)
	}
	cancel3()

	close(rs2.releaseShutdownCh)
	time.Sleep(time.Millisecond) // mostly ensures logs are assigned to this test properly

	// ensure runCtx is actually cancelled (ages ago)
	select {
	case <-runCtx1.Done():
	case <-time.NewTimer(time.Second).C:
		t.Errorf("could not wait until ctx was done: %v", runCtx1.Err())
	}
}

func TestManagerDie(t *testing.T) {
	expectedErr := fmt.Errorf("lol error")

	m := NewWithContext(t.Context(), func(b string, s Status[any]) (string, error) {
		go func() {
			time.Sleep(time.Millisecond * 10)
			s.Check(expectedErr)
		}()

		s.After(func() error {
			t.Errorf("s.After should not be called")
			return nil
		})

		return "_" + b, nil
	})

	out, ctx, err := m.Run(context.Background(), "x", nil)
	if out != "_x" || err != nil {
		t.Errorf("could not run object")
	}

	select {
	case <-ctx.Done():
	case <-time.NewTimer(time.Second).C:
		t.Errorf("ctx did not shutdown in time")
	}
	if context.Cause(ctx) != expectedErr {
		t.Errorf("bad err returned: %v", context.Cause(ctx))
	}
}

func TestTask(t *testing.T) {
	var taskWaitingToStop bool
	var afterCalled bool
	releaseCh := make(chan struct{})

	m := NewWithContext(t.Context(), func(b string, s Status[any]) (string, error) {
		s.After(func() error {
			afterCalled = true
			return nil
		})

		s.Task(func(stop <-chan struct{}) error {
			<-stop
			taskWaitingToStop = true
			<-releaseCh
			return nil
		})

		return "_" + b, nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	m.Run(ctx, "x", nil)
	cancel()

	// TODO: timeout tests are bad juju but they work for now

	time.Sleep(time.Millisecond * 4)
	if !taskWaitingToStop || afterCalled {
		t.Errorf("should be waiting but not done")
	}

	close(releaseCh)
	time.Sleep(time.Millisecond * 4)
	if !afterCalled {
		t.Errorf("should be done")
	}
}

func TestJoinTask(t *testing.T) {
	errExpected := errors.New("expected")
	failCh := make(chan struct{})

	m := NewWithContext(t.Context(), func(b string, s Status[any]) (string, error) {
		s.JoinTask(func(ctx context.Context, ch <-chan bool, a any) error {
			t.Errorf("should not join; err in setup")
			<-failCh
			return nil
		})
		return "", errExpected
	})

	_, _, err := m.Run(t.Context(), "x", nil)
	if err != errExpected {
		t.Errorf("got unexpected err: %v", err)
	}

	select {
	case <-failCh:
		t.Errorf("got failCh")
	case <-time.After(time.Millisecond * 2):
	}

	// now try outer err before inner shutdown

	dieCh := make(chan struct{})
	shutdownRedirectCh := make(chan struct{})
	awakeLatchCh := make(chan struct{})

	m = NewWithContext(t.Context(), func(key string, s Status[any]) (string, error) {
		s.JoinTask(func(ctx context.Context, shutdownCh <-chan bool, arg any) error {
			close(awakeLatchCh)
			for range shutdownCh {
				t.Errorf("should never get 'normal' shutdown")
			}
			close(shutdownRedirectCh)
			return nil
		})

		s.Task(func(stop <-chan struct{}) error {
			select {
			case <-stop:
				return nil
			case <-dieCh:
				return errExpected
			}
		})

		return key, nil
	})

	out, runCtx, err := m.Run(t.Context(), "x", nil)
	if err != nil || out != "x" {
		t.Errorf("could not join valid lcm, err=%v key=%v", err, out)
	}
	<-awakeLatchCh

	close(dieCh)
	<-runCtx.Done()

	select {
	case _, ok := <-shutdownRedirectCh:
		if ok {
			t.Errorf("expected shutdownRedirectCh close, was ok=%v", ok)
		}
	case <-time.After(time.Millisecond):
		t.Errorf("expected shutdownRedirectCh close")
	}
}
