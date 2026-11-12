package state

import (
	"testing"
	"time"

	"fastiot/internal/ems/channel"
	"fastiot/internal/ems/signal"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func result(deviceID, pointID string, value float64) channel.Result {
	return channel.Result{
		DeviceID: deviceID, PointID: pointID,
		Value: signal.Num(value), Quality: signal.Good, SourceTime: t0,
	}
}

func TestApplyAndGet(t *testing.T) {
	store := New()
	store.Apply(t0, []channel.Result{result("d1", "temp", 35.1)})
	value, ok := store.Get(t0, "d1", "temp", 10*time.Second)
	if !ok || value.Quality != signal.Good || value.Value.Num != 35.1 {
		t.Fatalf("unexpected value %+v ok=%v", value, ok)
	}
	if _, ok := store.Get(t0, "d1", "missing", time.Second); ok {
		t.Fatal("missing point must not be found")
	}
}

func TestValueGoesStaleByAge(t *testing.T) {
	store := New()
	store.Apply(t0, []channel.Result{result("d1", "temp", 35.1)})
	value, _ := store.Get(t0.Add(11*time.Second), "d1", "temp", 10*time.Second)
	if value.Quality != signal.Stale {
		t.Fatalf("quality = %s, want stale", value.Quality)
	}
	if value.Value.Num != 35.1 {
		t.Fatal("stale values keep the last reading")
	}
}

func TestOfflineDeviceMarksPointsStale(t *testing.T) {
	store := New()
	store.Apply(t0, []channel.Result{result("d1", "temp", 35.1)})
	store.SetHealth("d1", channel.HealthOffline, t0)
	value, _ := store.Get(t0, "d1", "temp", time.Minute)
	if value.Quality != signal.Stale {
		t.Fatalf("offline device quality = %s, want stale", value.Quality)
	}
	if store.Health("d1") != channel.HealthOffline {
		t.Fatal("health not stored")
	}
}

func TestSamplesAndSnapshotAreDeterministic(t *testing.T) {
	store := New()
	store.Apply(t0, []channel.Result{
		result("d2", "b", 2), result("d1", "b", 2), result("d1", "a", 1),
	})
	samples := store.Samples("d1", t0, time.Minute)
	if len(samples) != 2 || samples["a"].Value.Num != 1 {
		t.Fatalf("unexpected samples %+v", samples)
	}
	snapshot := store.Snapshot(t0, time.Minute)
	if len(snapshot) != 3 || snapshot[0].DeviceID != "d1" || snapshot[0].PointID != "a" {
		t.Fatalf("snapshot not sorted: %+v", snapshot)
	}
}
