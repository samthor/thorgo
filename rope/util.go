package rope

import (
	"math/bits"
	"math/rand/v2"
)

// randomHeight picks a height from 1-minHeight, inclusive.
// the odds of 2 is 50%, 3 is 25%, 4 is 12.5%... down to 32 at ~0.00000005%
func randomHeight() int {
	return min(maxHeight, 1+bits.LeadingZeros64(rand.Uint64()))
}
