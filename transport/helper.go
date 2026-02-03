package transport

import (
	"math/rand/v2"
)

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

const (
	skewBy = 0.20
)

func randomSkew() (skew float64) {
	return 1.0 - skewBy + rand.Float64()*(skewBy*2)
}
