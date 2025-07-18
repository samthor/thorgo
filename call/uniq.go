package call

import (
	"math/rand/v2"

	"github.com/taylorza/go-lfsr"
)

// newIDGenerator returns IDs in the range (0,2^31].
func newIDGenerator() <-chan int {
	gen := lfsr.NewLfsr32(rand.Uint32())
	out := make(chan int)

	go func() {
		for {
			id, restarted := gen.Next()
			if restarted {
				panic("generated ~32 bits of IDs")
			}

			if id == 0 || id&0x80000000 == 0x80000000 {
				continue // don't allow zero or anything with top bit
			}

			out <- int(id)
		}
	}()

	return out
}
