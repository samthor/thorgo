package lcm

import (
	"context"
	"time"
)

// Option can be applied when building a new Manager.
type Option struct {
	joinTask func(ctx context.Context, shutdownCh <-chan bool) error
}

// TimeoutOption makes the manager shutdown a given duration after the last context has been cancelled.
func TimeoutOption(t time.Duration) *Option {
	return &Option{
		joinTask: func(ctx context.Context, shutdownCh <-chan bool) error {
			result := <-shutdownCh
			if result {
				// if normal shutdown, wait for time OR irregular shutdown
				select {
				case <-time.After(t):
				case <-shutdownCh:
				}
			}
			return nil
		},
	}
}
