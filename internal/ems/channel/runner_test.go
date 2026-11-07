package channel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerCyclesUntilCancelled(t *testing.T) {
	bus := newFakeBus()
	bus.holding[10] = 351
	ch, err := New("c1", bus, []Device{device("d1", 1, tempPoint(10))})
	if err != nil {
		t.Fatal(err)
	}
	var cycles int32
	runner := &Runner{
		Channel: ch,
		Period:  5 * time.Millisecond,
		OnCycle: func(time.Time, []Result, []WriteResult) {
			atomic.AddInt32(&cycles, 1)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if err := runner.Run(ctx); err != context.DeadlineExceeded {
		t.Fatalf("runner returned %v, want deadline exceeded", err)
	}
	if atomic.LoadInt32(&cycles) < 2 {
		t.Fatalf("cycles = %d, want at least 2", cycles)
	}
	if stats := ch.Stats("d1"); stats.Requests == 0 {
		t.Fatal("runner did not poll")
	}
}
