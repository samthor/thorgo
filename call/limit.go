package call

import (
	"golang.org/x/time/rate"
)

type LimitConfig struct {
	Burst int        `json:"b"`
	Rate  rate.Limit `json:"r"`
}

func buildLimiter(lc *LimitConfig, extra float64) *rate.Limiter {
	ratio := max(1.0, 1.0+extra)

	if lc == nil {
		return rate.NewLimiter(rate.Inf, 0)
	}
	return rate.NewLimiter(rate.Limit(float64(lc.Rate)*ratio), int(float64(lc.Burst)*ratio))
}
