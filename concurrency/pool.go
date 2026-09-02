package concurrency

import (
	"context"
	"sync"
	"time"
)

// RunPool executes fn for each item with adaptive in-flight control.
// The first error cancels ctx; partial completion is the caller's responsibility.
func RunPool[T any](ctx context.Context, items []T, cfg Config, fn func(context.Context, T) error) error {
	if len(items) == 0 {
		return nil
	}
	cfg = cfg.Normalize()
	lim := NewLimiter(cfg)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		firstErr error
		errOnce  sync.Once
	)

	for _, item := range items {
		if err := lim.Acquire(ctx); err != nil {
			wg.Wait()
			if firstErr != nil {
				return firstErr
			}
			return err
		}
		wg.Add(1)
		go func(item T) {
			defer wg.Done()
			defer lim.Release()
			start := time.Now()
			err := fn(ctx, item)
			lim.Record(time.Since(start), err)
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}(item)
	}
	wg.Wait()
	return firstErr
}

// RunPoolUsingLimiter runs fn with an existing limiter (for progress Limit() during work).
func RunPoolUsingLimiter[T any](ctx context.Context, items []T, lim *Limiter, fn func(context.Context, T) error) error {
	if len(items) == 0 {
		return nil
	}
	if lim == nil {
		lim = NewLimiter(DefaultConfig())
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		firstErr error
		errOnce  sync.Once
	)

	for _, item := range items {
		if err := lim.Acquire(ctx); err != nil {
			wg.Wait()
			if firstErr != nil {
				return firstErr
			}
			return err
		}
		wg.Add(1)
		go func(item T) {
			defer wg.Done()
			defer lim.Release()
			start := time.Now()
			err := fn(ctx, item)
			lim.Record(time.Since(start), err)
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}(item)
	}
	wg.Wait()
	return firstErr
}

// RunPoolWithLimiter is RunPool but exposes the limiter for final Limit() reporting.
func RunPoolWithLimiter[T any](ctx context.Context, items []T, cfg Config, fn func(context.Context, T) error) (*Limiter, error) {
	if len(items) == 0 {
		return NewLimiter(cfg.Normalize()), nil
	}
	cfg = cfg.Normalize()
	lim := NewLimiter(cfg)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		firstErr error
		errOnce  sync.Once
	)

	for _, item := range items {
		if err := lim.Acquire(ctx); err != nil {
			wg.Wait()
			if firstErr != nil {
				return lim, firstErr
			}
			return lim, err
		}
		wg.Add(1)
		go func(item T) {
			defer wg.Done()
			defer lim.Release()
			start := time.Now()
			err := fn(ctx, item)
			lim.Record(time.Since(start), err)
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}(item)
	}
	wg.Wait()
	return lim, firstErr
}
