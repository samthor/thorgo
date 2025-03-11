package lcm

import (
	"context"
	"time"
)

// Option can be applied when building a new Manager.
type Option struct {
	joinTask func(ctx context.Context) error
}

// TimeoutOption makes the manager shutdown a given duration after the last context has been cancelled.
func TimeoutOption(t time.Duration) *Option {
	return &Option{
		joinTask: func(ctx context.Context) error {
			<-ctx.Done()
			time.Sleep(t)
			return nil
		},
	}
}
