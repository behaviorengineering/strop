package concurrency

import (
	"context"
	"sync"
	"time"
)

// Limiter controls adaptive in-flight concurrency.
type Limiter struct {
	mu            sync.Mutex
	cond          *sync.Cond
	cfg           Config
	limit         int
	inFlight      int
	fastStreak    int
	cooldownUntil time.Time
}

// NewLimiter builds a limiter from normalized config.
func NewLimiter(cfg Config) *Limiter {
	cfg = cfg.Normalize()
	l := &Limiter{
		cfg:   cfg,
		limit: initialLimit(cfg),
	}
	l.cond = sync.NewCond(&l.mu)
	return l
}

// Limit returns the current in-flight cap (for progress logging).
func (l *Limiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// Acquire blocks until a slot is available or ctx is cancelled.
func (l *Limiter) Acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for l.inFlight >= l.limit {
		if err := l.wait(ctx); err != nil {
			return err
		}
	}
	l.inFlight++
	return nil
}

// Release frees one in-flight slot.
func (l *Limiter) Release() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight > 0 {
		l.inFlight--
	}
	l.cond.Broadcast()
}

// Record applies adaptive policy after one unit completes.
func (l *Limiter) Record(latency time.Duration, err error) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cfg.Fixed > 0 {
		l.cond.Broadcast()
		return
	}

	outcome := ClassifyError(err)
	if err == nil && latency > l.cfg.SlowThreshold {
		outcome = OutcomeSlow
	}

	switch outcome {
	case OutcomeTrip:
		l.limit = l.limit / 2
		if l.limit < l.cfg.Min {
			l.limit = l.cfg.Min
		}
		l.fastStreak = 0
		l.cooldownUntil = time.Now().Add(l.cfg.Cooldown)
	case OutcomeSlow:
		if l.limit > l.cfg.Min {
			l.limit--
		}
		l.fastStreak = 0
	case OutcomeOK:
		if time.Now().Before(l.cooldownUntil) {
			break
		}
		l.fastStreak++
		if l.fastStreak >= l.cfg.SuccessBurst && l.limit < l.cfg.Max {
			l.limit++
			l.fastStreak = 0
		}
	}
	l.cond.Broadcast()
}

func (l *Limiter) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() {
		l.cond.Broadcast()
	})
	defer stop()
	l.cond.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
