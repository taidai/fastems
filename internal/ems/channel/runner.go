package channel

import (
	"context"
	"errors"
	"time"
)

// Runner polls one channel on a fixed period until the context is cancelled.
// One channel has one runner; different channels run independently.
type Runner struct {
	Channel *Channel
	Period  time.Duration
	OnCycle func(now time.Time, results []Result, writes []WriteResult)
}

// Run executes one cycle immediately, then one per period.
func (r *Runner) Run(ctx context.Context) error {
	if r.Channel == nil {
		return errors.New("runner has no channel")
	}
	period := r.Period
	if period <= 0 {
		period = time.Second
	}
	cycle := func() {
		now := time.Now()
		results, writes := r.Channel.RunOnce(ctx, now)
		if r.OnCycle != nil {
			r.OnCycle(now, results, writes)
		}
	}
	cycle()
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			cycle()
		}
	}
}
