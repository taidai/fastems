package decision

import (
	"testing"

	"fastems/internal/ems/alloc"
)

func resolved(id string, discharge, charge float64) resolvedMember {
	return resolvedMember{member: alloc.Member{
		ID: id, Online: true,
		MaxDischarge: discharge, MaxCharge: charge,
		AvailableDischarge: discharge, AvailableCharge: charge,
	}}
}

func shareByIDTest(t *testing.T, result alloc.Result, id string) alloc.Share {
	t.Helper()
	for _, share := range result.Shares {
		if share.MemberID == id {
			return share
		}
	}
	t.Fatalf("member %s missing from result: %+v", id, result.Shares)
	return alloc.Share{}
}

func TestDistributeProportionalSplit(t *testing.T) {
	result := distribute(275, []resolvedMember{
		resolved("A", 200, 200), resolved("B", 200, 200), resolved("C", 150, 150),
	})
	for id, want := range map[string]float64{"A": 100, "B": 100, "C": 75} {
		if got := shareByIDTest(t, result, id).Setpoint; got != want {
			t.Errorf("%s setpoint = %v, want %v", id, got, want)
		}
	}
	if result.Shortfall != 0 {
		t.Errorf("unexpected shortfall %v", result.Shortfall)
	}
}

func TestDistributeExcludesOfflineAndReallocates(t *testing.T) {
	offline := resolved("D", 200, 200)
	offline.member.Online = false
	result := distribute(300, []resolvedMember{resolved("A", 200, 200), resolved("B", 200, 200), offline})
	if got := shareByIDTest(t, result, "D"); !got.Excluded || got.Reason != "offline" {
		t.Errorf("offline member not excluded: %+v", got)
	}
	if a, b := shareByIDTest(t, result, "A").Setpoint, shareByIDTest(t, result, "B").Setpoint; a != 150 || b != 150 {
		t.Errorf("expected 150/150, got %v/%v", a, b)
	}
}

func TestDistributePriorityFillsHigherFirst(t *testing.T) {
	low := resolved("low", 100, 100)
	high := resolved("high", 100, 100)
	high.priority = 1
	result := distribute(150, []resolvedMember{low, high})
	if got := shareByIDTest(t, result, "high").Setpoint; got != 100 {
		t.Errorf("high setpoint = %v, want 100", got)
	}
	if got := shareByIDTest(t, result, "low").Setpoint; got != 50 {
		t.Errorf("low setpoint = %v, want 50", got)
	}
}

func TestDistributeWeightBiasesShare(t *testing.T) {
	a := resolved("A", 100, 100)
	a.weight = 2
	b := resolved("B", 100, 100)
	b.weight = 1
	result := distribute(150, []resolvedMember{a, b})
	if got := shareByIDTest(t, result, "A").Setpoint; got != 100 {
		t.Errorf("A setpoint = %v, want 100", got)
	}
	if got := shareByIDTest(t, result, "B").Setpoint; got != 50 {
		t.Errorf("B setpoint = %v, want 50", got)
	}
}

func TestDistributeWeightCappedAtCapacity(t *testing.T) {
	a := resolved("A", 100, 100)
	a.weight = 2
	b := resolved("B", 100, 100)
	b.weight = 1
	result := distribute(180, []resolvedMember{a, b})
	if got := shareByIDTest(t, result, "A").Setpoint; got != 100 {
		t.Errorf("A setpoint = %v, want 100 (capped at capacity)", got)
	}
	if got := shareByIDTest(t, result, "B").Setpoint; got != 80 {
		t.Errorf("B setpoint = %v, want 80", got)
	}
}

func TestDistributeMinPowerExcludesSmallShare(t *testing.T) {
	big := resolved("big", 1000, 1000)
	small := resolved("small", 10, 10)
	small.member.MinPower = 50
	result := distribute(100, []resolvedMember{big, small})
	if got := shareByIDTest(t, result, "small"); !got.Excluded || got.Reason == "" {
		t.Errorf("small member should be excluded, got %+v", got)
	}
	if got := shareByIDTest(t, result, "big").Setpoint; got != 100 {
		t.Errorf("big setpoint = %v, want 100", got)
	}
}

func TestDistributeConstraintExcludesWithReason(t *testing.T) {
	m := resolved("A", 100, 100)
	m.member.Disabled = true
	m.reasons = []string{"SOC 95 exceeds 90, excluded"}
	result := distribute(100, []resolvedMember{m})
	share := shareByIDTest(t, result, "A")
	if !share.Excluded || share.Reason != "SOC 95 exceeds 90, excluded" {
		t.Errorf("constraint reason lost: %+v", share)
	}
}

func TestDistributeDerateZeroesOneDirection(t *testing.T) {
	m := resolved("A", 100, 100)
	m.member.AvailableCharge = 0
	m.reasons = []string{"SOC 95 exceeds 90, derated"}
	result := distribute(-100, []resolvedMember{m})
	if got := shareByIDTest(t, result, "A"); !got.Excluded || got.Reason != "SOC 95 exceeds 90, derated" {
		t.Errorf("derated charge member should be excluded with reason, got %+v", got)
	}
	// Discharge is still available.
	result = distribute(100, []resolvedMember{m})
	if got := shareByIDTest(t, result, "A"); got.Excluded || got.Setpoint != 100 {
		t.Errorf("discharge should still work, got %+v", got)
	}
}

func TestWithinDeadband(t *testing.T) {
	if !withinDeadband(100, 102, 5) {
		t.Error("102 is within deadband 5 of 100")
	}
	if withinDeadband(100, 102, 1) {
		t.Error("102 is outside deadband 1 of 100")
	}
	if withinDeadband(100, 100, 0) {
		t.Error("deadband 0 must disable the check")
	}
	if withinDeadband(-100, -102, 5) != true {
		t.Error("signed magnitudes should compare by absolute difference")
	}
}
