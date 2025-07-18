package call

import (
	"golang.org/x/time/rate"
)

type LimitConfig struct {
	Burst int        `json:"b"`
	Rate  rate.Limit `json:"r"`
}

func buildLimiter(lc *LimitConfig) *rate.Limiter {
	if lc == nil {
		return rate.NewLimiter(rate.Inf, 0)
	}
	return rate.NewLimiter(lc.Rate, lc.Burst)
}
