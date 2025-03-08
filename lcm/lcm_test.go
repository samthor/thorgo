package lcm

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"testing"
	"time"
)

type FakeObject struct {
	shutdownCount *atomic.Int32
	name          string
}

func (fo FakeObject) Shutdown() {
	fo.shutdownCount.Add(1)
}

func TestManager(t *testing.T) {
	var activeCount atomic.Int32
	var shutdownCount atomic.Int32

	m := New(t.Context(), func(b Build[string]) (FakeObject, error) {
		if b.Key == "" {
			return FakeObject{}, errors.New("empty name")
		}

		fo := FakeObject{
			shutdownCount: &shutdownCount,
			name:          b.Key,
		}

		activeCount.Add(1)
		time.Sleep(time.Microsecond)

		context.AfterFunc(b.C, func() {
			activeCount.Add(-1)
		})

		return fo, nil
	})

	userCtx, cancel := context.WithCancel(t.Context())
	fo, _, err := m.Run(userCtx, "butt")
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
	_, runCtx, err := m.Run(userCtx, "")
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

func (rs RaceShutdown) Shutdown() error {
	log.Printf("waiting for ch to be done inst=%v", rs.inst)
	<-rs.releaseShutdownCh
	return nil
}

func TestManagerShutdownRace(t *testing.T) {
	var inst int

	m := New(t.Context(), func(b Build[string]) (RaceShutdown, error) {
		inst++
		return RaceShutdown{
			releaseShutdownCh: make(chan struct{}),
			inst:              inst,
		}, nil
	})

	userCtx1, cancel1 := context.WithCancel(t.Context())
	rs1, _, _ := m.Run(userCtx1, "foo")
	if rs1.inst != 1 {
		t.Errorf("unexpected seq, wanted 1 was=%v", rs1.inst)
	}
	cancel1()
	time.Sleep(time.Millisecond * 10)

	userCtx2, cancel2 := context.WithCancelCause(t.Context())
	go func() {
		time.Sleep(time.Millisecond * 100)
		cancel2(nil)
	}()
	// the below line should block because we're waiting for shutdown
	_, _, err := m.Run(userCtx2, "foo")
	if err == nil {
		t.Errorf("should have timed out join, err was=%v", err)
	}
	close(rs1.releaseShutdownCh)

	userCtx3, cancel3 := context.WithCancel(t.Context())
	rs2, _, _ := m.Run(userCtx3, "foo")
	if rs2.inst != 2 {
		t.Errorf("unexpected seq, wanted 2 was=%v", rs2.inst)
	}
	cancel3()

	close(rs2.releaseShutdownCh)
	time.Sleep(time.Millisecond) // mostly ensures logs are assigned to this test properly
}
