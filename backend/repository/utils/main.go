package utils

import (
	"context"
	"math/rand/v2"
	"time"
)

// jitter ttl up to 30s
func JitterTtl(baseTTL time.Duration) time.Duration {
	return baseTTL + time.Duration(rand.IntN(30))*time.Second
}

func GetRemainingDeadline(ctx context.Context) (time.Duration, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0, ErrMissingDeadline
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0, context.DeadlineExceeded
	}
	return remaining, nil
}
