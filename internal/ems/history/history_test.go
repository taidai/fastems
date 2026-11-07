package history

import (
	"testing"
	"time"

	"easyems/internal/ems/signal"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func sample(at time.Time, value float64) Sample {
	return Sample{
		DeviceID: "d1", PointID: "temp",
		Value: signal.Num(value), Quality: signal.Good,
		SourceTime: at, ReceivedTime: at,
	}
}

func TestQueryRangeAndLimit(t *testing.T) {
	store := New(100)
	store.Record([]Sample{sample(t0, 1), sample(t0.Add(time.Second), 2), sample(t0.Add(2*time.Second), 3)})

	got := store.Query("d1", "temp", t0.Add(time.Second), t0.Add(2*time.Second), 0)
	if len(got) != 2 || got[0].Value.Num != 2 || got[1].Value.Num != 3 {
		t.Fatalf("range query = %+v", got)
	}

	limited := store.Query("d1", "temp", time.Time{}, time.Time{}, 1)
	if len(limited) != 1 || limited[0].Value.Num != 3 {
		t.Fatalf("limit must keep the most recent sample, got %+v", limited)
	}
}

func TestCapDropsOldest(t *testing.T) {
	store := New(2)
	store.Record([]Sample{sample(t0, 1), sample(t0.Add(time.Second), 2), sample(t0.Add(2*time.Second), 3)})
	got := store.Query("d1", "temp", time.Time{}, time.Time{}, 0)
	if len(got) != 2 || got[0].Value.Num != 2 || got[1].Value.Num != 3 {
		t.Fatalf("cap = %+v", got)
	}
	if store.Len("d1", "temp") != 2 {
		t.Fatal("len should reflect the cap")
	}
}

func TestPointsAreListed(t *testing.T) {
	store := New(10)
	store.Record([]Sample{sample(t0, 1)})
	points := store.Points()
	if len(points) != 1 || points[0] != "d1\x00temp" {
		t.Fatalf("points = %v", points)
	}
}

func TestSetLimitTrimsExistingSamples(t *testing.T) {
	store := New(10)
	now := time.Now()
	for i := 0; i < 5; i++ {
		store.Record([]Sample{{DeviceID: "d", PointID: "p", Value: signal.Num(float64(i)), SourceTime: now.Add(time.Duration(i) * time.Second)}})
	}
	store.SetLimit(2)
	if got := store.Len("d", "p"); got != 2 {
		t.Fatalf("len = %d", got)
	}
	samples := store.Query("d", "p", time.Time{}, time.Time{}, 10)
	if len(samples) != 2 || samples[0].Value.Num != 3 || samples[1].Value.Num != 4 {
		t.Fatalf("samples = %+v", samples)
	}
}
