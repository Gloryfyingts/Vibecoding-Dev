package client

import (
	"context"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	limiter *rate.Limiter
}

func NewRateLimiter(weightPerMinute int) *RateLimiter {
	perSecond := rate.Limit(weightPerMinute) / 60.0
	burst := weightPerMinute / 6
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		limiter: rate.NewLimiter(perSecond, burst),
	}
}

func (r *RateLimiter) Wait(ctx context.Context, weight int) error {
	return r.limiter.WaitN(ctx, weight)
}
