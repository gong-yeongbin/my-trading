package kis

import (
	"context"
	"sync"
	"time"
)

// limiter 는 호출 사이에 최소 간격을 강제한다 (초당 rps 건).
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// newLimiter 는 초당 rps 건으로 호출 간격을 띄운다. rps 가 0 이하면 제한하지 않는다.
func newLimiter(rps float64) *limiter {
	if rps <= 0 {
		return &limiter{}
	}
	return &limiter{interval: time.Duration(float64(time.Second) / rps)}
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	var delay time.Duration
	if l.next.After(now) {
		delay = l.next.Sub(now)
		l.next = l.next.Add(l.interval)
	} else {
		l.next = now.Add(l.interval)
	}
	l.mu.Unlock()
	if delay == 0 {
		return ctx.Err()
	}
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
