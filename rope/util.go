package rope

import (
	"math/bits"
	"math/rand/v2"
)

func randomHeight() int {
	// min zero, with every extra height at 0.5x chance
	return 1 + bits.LeadingZeros64(rand.Uint64())
}
