package alloc

import (
	"math"
	"testing"
	"time"
)

func member(id string, discharge, charge float64) Member {
	return Member{
		ID:                 id,
		Online:             true,
		MaxDischarge:       discharge,
		MaxCharge:          charge,
		AvailableDischarge: discharge,
		AvailableCharge:    charge,
	}
}

func shareOf(t *testing.T, result Result, id string) Share {
	t.Helper()
	for _, share := range result.Shares {
		if share.MemberID == id {
			return share
		}
	}
	t.Fatalf("member %s missing from result", id)
	return Share{}
}

func TestProportionalSplit(t *testing.T) {
	result, err := Allocate(275, []Member{member("A", 200, 200), member("B", 200, 200), member("C", 150, 150)})
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]float64{"A": 100, "B": 100, "C": 75} {
		if got := shareOf(t, result, id).Setpoint; got != want {
			t.Errorf("%s setpoint = %v, want %v", id, got, want)
		}
	}
	if result.Shortfall != 0 {
		t.Errorf("unexpected shortfall %v", result.Shortfall)
	}
}

func TestExcludesOfflineAndReallocates(t *testing.T) {
	offline := member("D", 200, 200)
	offline.Online = false
	result, _ := Allocate(300, []Member{member("A", 200, 200), member("B", 200, 200), offline})
	if got := shareOf(t, result, "D"); !got.Excluded || got.Setpoint != 0 {
		t.Errorf("offline member not excluded: %+v", got)
	}
	if a, b := shareOf(t, result, "A").Setpoint, shareOf(t, result, "B").Setpoint; a != 150 || b != 150 {
		t.Errorf("expected 150/150, got %v/%v", a, b)
	}
}

func TestSaturatesAtCapacityAndReportsShortfall(t *testing.T) {
	offline := member("D", 200, 200)
	offline.Online = false
	result, _ := Allocate(800, []Member{member("A", 200, 200), member("B", 200, 200), member("C", 150, 150), offline})
	for id, want := range map[string]float64{"A": 200, "B": 200, "C": 150} {
		if got := shareOf(t, result, id).Setpoint; got != want {
			t.Errorf("%s setpoint = %v, want %v", id, got, want)
		}
	}
	if result.Delivered != 550 || result.Shortfall != 250 {
		t.Errorf("delivered/shortfall = %v/%v, want 550/250", result.Delivered, result.Shortfall)
	}
	if len(result.Notes) == 0 {
		t.Error("expected shortfall note")
	}
}

func TestChargeDirectionUsesChargeCapacity(t *testing.T) {
	a := Member{ID: "A", Online: true, MaxDischarge: 500, MaxCharge: 200, AvailableDischarge: 500, AvailableCharge: 200}
	b := Member{ID: "B", Online: true, MaxDischarge: 500, MaxCharge: 400, AvailableDischarge: 500, AvailableCharge: 400}
	result, _ := Allocate(-300, []Member{a, b})
	if got := shareOf(t, result, "A").Setpoint; got != -100 {
		t.Errorf("A setpoint = %v, want -100", got)
	}
	if got := shareOf(t, result, "B").Setpoint; got != -200 {
		t.Errorf("B setpoint = %v, want -200", got)
	}
	if result.Delivered != -300 {
		t.Errorf("delivered = %v, want -300", result.Delivered)
	}
}

func TestMinimumPowerExcludesSmallShare(t *testing.T) {
	big := member("big", 1000, 1000)
	small := member("small", 10, 10)
	small.MinPower = 50
	result, _ := Allocate(100, []Member{big, small})
	if got := shareOf(t, result, "small"); !got.Excluded || got.Reason == "" {
		t.Errorf("small member should be excluded, got %+v", got)
	}
	if got := shareOf(t, result, "big").Setpoint; got != 100 {
		t.Errorf("big setpoint = %v, want 100", got)
	}
}

func TestFaultedAndDisabledAreExcluded(t *testing.T) {
	faulted := member("F", 100, 100)
	faulted.Faulted = true
	disabled := member("X", 100, 100)
	disabled.Disabled = true
	result, _ := Allocate(100, []Member{member("A", 100, 100), faulted, disabled})
	if !shareOf(t, result, "F").Excluded || !shareOf(t, result, "X").Excluded {
		t.Error("faulted/disabled must be excluded")
	}
	if got := shareOf(t, result, "A").Setpoint; got != 100 {
		t.Errorf("A setpoint = %v, want 100", got)
	}
}

func TestAllocateRejectsInvalidRequest(t *testing.T) {
	if _, err := Allocate(math.NaN(), nil); err == nil {
		t.Fatal("expected error for NaN")
	}
}

func TestTrackerHoldsThroughGraceThenReallocates(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	tracker := NewTracker(10 * time.Second)
	a := member("A", 200, 200)
	first, _ := tracker.Apply(now, 100, []Member{a})
	if got := shareOf(t, first, "A").Setpoint; got != 100 {
		t.Fatalf("initial setpoint = %v, want 100", got)
	}

	a.Online = false
	heldResult, _ := tracker.Apply(now.Add(5*time.Second), 100, []Member{a})
	held := shareOf(t, heldResult, "A")
	if !held.Held || held.Setpoint != 100 {
		t.Fatalf("expected held setpoint 100, got %+v", held)
	}
	if heldResult.Delivered != 100 {
		t.Fatalf("held value should count as delivered, got %v", heldResult.Delivered)
	}

	expired, _ := tracker.Apply(now.Add(16*time.Second), 100, []Member{a})
	if got := shareOf(t, expired, "A"); !got.Excluded || got.Setpoint != 0 {
		t.Fatalf("expected exclusion after grace, got %+v", got)
	}
	if expired.Shortfall != 100 {
		t.Fatalf("expected 100 shortfall, got %+v", expired)
	}

	b := member("B", 200, 200)
	reallocated, _ := tracker.Apply(now.Add(17*time.Second), 100, []Member{a, b})
	if got := shareOf(t, reallocated, "B").Setpoint; got != 100 {
		t.Fatalf("expected B to take the request, got %v", got)
	}
}

func TestTrackerDropsHeldValueWhenDirectionFlips(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	tracker := NewTracker(10 * time.Second)
	a := member("A", 200, 200)
	tracker.Apply(now, 100, []Member{a})
	a.Online = false
	result, _ := tracker.Apply(now.Add(2*time.Second), -100, []Member{a})
	if got := shareOf(t, result, "A"); got.Held || got.Setpoint != 0 {
		t.Fatalf("held value must not survive a direction change: %+v", got)
	}
}
