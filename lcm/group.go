package lcm

import (
	"context"
	"sync"
	"time"
)

type groupTimer struct {
	timeout time.Duration
	doneCh  chan struct{}

	lock      sync.Mutex
	active    int
	seq       int
	stopTimer func() bool
}

type GroupTimer interface {
	// Join returns true if a change was made, or false if the timer had already expired.
	Join(context.Context) bool
	// Done returns a channel which is closed when the the last context is done and the timer has expired.
	Done() <-chan struct{}
}

// NewGroupTimer builds a timer that expires in the passed duration after the last context joined is done.
func NewGroupTimer(t time.Duration, ctx context.Context) GroupTimer {
	doneCh := make(chan struct{})

	gt := &groupTimer{
		timeout:   t,
		doneCh:    doneCh,
		stopTimer: func() bool { return false },
	}

	gt.Join(ctx)
	return gt
}

func (gt *groupTimer) Done() <-chan struct{} {
	return gt.doneCh
}

func (gt *groupTimer) Join(ctx context.Context) bool {
	gt.lock.Lock()
	defer gt.lock.Unlock()

	select {
	case <-gt.doneCh:
		return false
	default:
	}

	gt.active++

	gt.stopTimer()
	context.AfterFunc(ctx, gt.contextDone)
	return true
}

func (gt *groupTimer) contextDone() {
	gt.lock.Lock()
	defer gt.lock.Unlock()

	gt.active--
	if gt.active < 0 {
		panic("got -ve group count")
	} else if gt.active > 0 {
		return // nothing to do
	}

	gt.seq++
	localSeq := gt.seq

	timer := time.AfterFunc(gt.timeout, func() {
		gt.lock.Lock()
		defer gt.lock.Unlock()
		if localSeq != gt.seq {
			return // sanity check in case something weird happened
		}

		close(gt.doneCh)
	})
	gt.stopTimer = timer.Stop
}
