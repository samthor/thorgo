package lifecycle

import (
	"context"
)

// IsDone is a helper which checks <-ctx.Done().
func IsDone(ctx context.Context) (done bool) {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
