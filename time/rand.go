package time

import (
	rand "math/rand/v2"
	"time"
)

// DurationRange gets a random time interval between these two values: [low,high).
func DurationRange(low time.Duration, high time.Duration) time.Duration {
	delta := int64(high - low)
	mid := time.Duration(rand.Int64N(int64(delta)))

	return low + mid
}

// DurationRatio returns the value +/- the floating point value. For instance, for -/+ 5%, pass 0.05.
func DurationRatio(value time.Duration, by float64) time.Duration {
	i := time.Duration(float64(value) * by)
	return DurationRange(value-i, value+i)
}
