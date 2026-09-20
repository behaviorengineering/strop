package concurrency

import "time"

const hardMaxWorkers = 32

// Config controls adaptive or fixed in-flight limits for RunPool.
type Config struct {
	Min           int
	Max           int
	SlowThreshold time.Duration
	SuccessBurst  int
	Cooldown      time.Duration
	Fixed         int // >0 disables adaptive adjustment
}

// DefaultConfig returns conservative adaptive defaults.
func DefaultConfig() Config {
	return Config{
		Min:           1,
		Max:           8,
		SlowThreshold: 30 * time.Second,
		SuccessBurst:  4,
		Cooldown:      5 * time.Second,
	}
}

// Normalize clamps bounds and fills zero durations/counts.
func (c Config) Normalize() Config {
	out := c
	if out.Min <= 0 {
		out.Min = 1
	}
	if out.Max <= 0 {
		out.Max = 8
	}
	if out.Max > hardMaxWorkers {
		out.Max = hardMaxWorkers
	}
	if out.Min > out.Max {
		out.Min = out.Max
	}
	if out.Fixed > 0 {
		if out.Fixed > hardMaxWorkers {
			out.Fixed = hardMaxWorkers
		}
		out.Min = out.Fixed
		out.Max = out.Fixed
	}
	if out.SlowThreshold <= 0 {
		out.SlowThreshold = 30 * time.Second
	}
	if out.SuccessBurst <= 0 {
		out.SuccessBurst = 4
	}
	if out.Cooldown <= 0 {
		out.Cooldown = 5 * time.Second
	}
	return out
}

func initialLimit(cfg Config) int {
	if cfg.Fixed > 0 {
		return cfg.Fixed
	}
	n := 2
	if cfg.Max < n {
		n = cfg.Max
	}
	if n < cfg.Min {
		n = cfg.Min
	}
	return n
}
