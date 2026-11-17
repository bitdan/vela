package runtime

import (
	"context"
	"time"
)

// RunRefreshLoop coalesces hints, bounds continuous bursts and provides a cheap
// scheduling pulse. A pulse alone never means repository IO is needed.
func RunRefreshLoop(ctx context.Context, changes <-chan struct{}, notify func(bool, time.Time)) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var timer *time.Timer
	var ready <-chan time.Time
	var first time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			now := time.Now()
			if first.IsZero() {
				first = now
			}
			delay := min(750*time.Millisecond, max(time.Duration(0), 3*time.Second-now.Sub(first)))
			if timer == nil {
				timer = time.NewTimer(delay)
			} else {
				timer.Reset(delay)
			}
			ready = timer.C
		case now := <-ready:
			ready, first = nil, time.Time{}
			notify(true, now)
		case now := <-ticker.C:
			notify(false, now)
		}
	}
}
