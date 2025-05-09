package rope

import (
	"math/rand/v2"
)

func randomHeight() int {
	height := 1
	r := rand.Int32()

	for r&1 == 1 {
		r <<= 1
		height++
	}

	return height
}
