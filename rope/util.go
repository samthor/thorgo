package rope

import (
	"math/bits"
	"math/rand/v2"
)

// randomHeight picks a height from 1-32.
// the odds of 2 is 50%, 3 is 25%, 4 is 12.5%...
func randomHeight() int {
	// min zero, with every extra height at 0.5x chance
	return min(32, 1+bits.LeadingZeros64(rand.Uint64()))
}
