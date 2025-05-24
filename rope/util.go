package rope

import (
	"math/bits"
	"math/rand/v2"
)

// randomHeight picks a height in the range [1,31], inclusive.
// the odds of 2 is 50%, 3 is 25%, 4 is 12.5%... down to 32 at ~0.00000005%
func randomHeight() int {
	// always set bits 1+2, so this can at most return 30 plus our min 1 => [1,31].
	return 1 + bits.TrailingZeros32(rand.Uint32()|3)
}
