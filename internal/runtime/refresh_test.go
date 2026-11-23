package runtime

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestRefreshLoopCoalescesBurstsAndCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		changes := make(chan struct{}, 1)
		type event struct {
			dirty bool
			at    time.Time
		}
		var events []event
		done := make(chan struct{})
		go func() {
			defer close(done)
			RunRefreshLoop(ctx, changes, func(dirty bool, at time.Time) { events = append(events, event{dirty, at}) })
		}()
		synctest.Wait()
		start := time.Now()
		for i := 0; i < 5; i++ {
			changes <- struct{}{}
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(649 * time.Millisecond)
		synctest.Wait()
		if len(events) != 0 {
			t.Fatal("burst was not debounced")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if len(events) != 1 || !events[0].dirty || events[0].at.Sub(start) != 1150*time.Millisecond {
			t.Fatalf("coalesced events: %+v", events)
		}
		time.Sleep(30*time.Second - time.Since(start))
		synctest.Wait()
		if len(events) != 2 || events[1].dirty {
			t.Fatal("scheduling pulse incorrectly requested IO")
		}
		cancel()
		synctest.Wait()
		<-done
		changes <- struct{}{}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(events) != 2 {
			t.Fatal("callback after shutdown")
		}
	})
}

func TestRefreshLoopBoundsContinuousBursts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		changes := make(chan struct{}, 1)
		calls := 0
		go RunRefreshLoop(ctx, changes, func(dirty bool, _ time.Time) {
			if dirty {
				calls++
			}
		})
		synctest.Wait()
		for i := 0; i < 7; i++ {
			changes <- struct{}{}
			time.Sleep(500 * time.Millisecond)
		}
		synctest.Wait()
		if calls != 1 {
			t.Fatalf("continuous hints starved refresh: %d", calls)
		}
	})
}
