package alarm

import (
	"testing"
	"time"
)

// raiseWithPoint raises an alarm with an explicit trigger point so tests can
// control the derived root cause independently of the device.
func raiseWithPoint(t *testing.T, engine *Engine, at time.Time, key, deviceID, pointID string) Alarm {
	t.Helper()
	alarm, err := engine.Raise(at, key, Warning, "测试报警", "detail", deviceID, pointID)
	if err != nil {
		t.Fatalf("raise %s: %v", key, err)
	}
	return alarm
}

func TestRootCauseDerivedFromPointThenDevice(t *testing.T) {
	engine := New()
	byPoint := raiseWithPoint(t, engine, t0, "k1", "d1", "p1")
	if byPoint.RootCause != "p1" {
		t.Fatalf("root cause = %q, want p1", byPoint.RootCause)
	}
	byDevice := raiseWithPoint(t, engine, t0, "k2", "d2", "")
	if byDevice.RootCause != "d2" {
		t.Fatalf("root cause = %q, want d2", byDevice.RootCause)
	}
}

func TestGroupingByRootCause(t *testing.T) {
	engine := New()
	first := raiseWithPoint(t, engine, t0, "a", "d1", "p1")
	if first.GroupID != "" {
		t.Fatalf("first alarm for a root cause must be a primary, group_id = %q", first.GroupID)
	}
	second := raiseWithPoint(t, engine, t0.Add(time.Second), "b", "d1", "p1")
	if second.GroupID != "a" {
		t.Fatalf("second alarm with same root cause must group under first, group_id = %q", second.GroupID)
	}
	unrelated := raiseWithPoint(t, engine, t0.Add(2*time.Second), "c", "d1", "p2")
	if unrelated.GroupID != "" {
		t.Fatalf("alarm with a different root cause must stay top-level, group_id = %q", unrelated.GroupID)
	}
	if len(engine.List()) != 3 {
		t.Fatalf("expected 3 alarms, got %d", len(engine.List()))
	}
}

func TestFloodSuppressionUpdatesCountNotRecord(t *testing.T) {
	engine := New()
	raised := 0
	engine.OnChange = func(_ Alarm, transition string) {
		if transition == "raised" {
			raised++
		}
	}

	first := raiseWithPoint(t, engine, t0, "k", "d1", "p1")
	if first.Count != 1 {
		t.Fatalf("count = %d, want 1", first.Count)
	}
	if _, err := engine.Clear(t0.Add(time.Second), "k"); err != nil {
		t.Fatal(err)
	}
	flapped := raiseWithPoint(t, engine, t0.Add(2*time.Second), "k", "d1", "p1")
	if !flapped.Active || flapped.Count != 2 {
		t.Fatalf("flapped alarm = %+v", flapped)
	}
	if !flapped.CreatedAt.Equal(t0) {
		t.Fatalf("flood-suppressed alarm must keep original creation time, got %v", flapped.CreatedAt)
	}
	if raised != 1 {
		t.Fatalf("suppressed flap must not fire a raised transition, raised events = %d", raised)
	}
	if engine.Active() != 1 || len(engine.List()) != 1 {
		t.Fatalf("flood suppression must not create new records: active=%d list=%d", engine.Active(), len(engine.List()))
	}
}

func TestGenuineRecurrenceAfterFloodWindow(t *testing.T) {
	engine := New()
	raised := 0
	engine.OnChange = func(_ Alarm, transition string) {
		if transition == "raised" {
			raised++
		}
	}

	raiseWithPoint(t, engine, t0, "k", "d1", "p1")
	if _, err := engine.Clear(t0.Add(time.Second), "k"); err != nil {
		t.Fatal(err)
	}
	again := raiseWithPoint(t, engine, t0.Add(61*time.Second), "k", "d1", "p1")
	if again.Count != 1 || !again.CreatedAt.Equal(t0.Add(61*time.Second)) {
		t.Fatalf("recurrence after the window must be a new record: %+v", again)
	}
	if raised != 2 {
		t.Fatalf("genuine recurrence must fire a raised transition, raised events = %d", raised)
	}
}

func TestCountAccumulatesAcrossRepeatedFlaps(t *testing.T) {
	engine := New()
	raiseWithPoint(t, engine, t0, "k", "d1", "p1")
	for i := 1; i <= 3; i++ {
		if _, err := engine.Clear(t0.Add(time.Duration(i)*time.Second), "k"); err != nil {
			t.Fatal(err)
		}
		at := t0.Add(time.Duration(i)*time.Second + 500*time.Millisecond)
		alarm := raiseWithPoint(t, engine, at, "k", "d1", "p1")
		if alarm.Count != i+1 {
			t.Fatalf("count = %d, want %d", alarm.Count, i+1)
		}
	}
}
