package decision

import "testing"

func TestAntiReverse(t *testing.T) {
	if reduction := AntiReverse(100); reduction != 0 {
		t.Fatalf("no reverse flow must not reduce, got %.1f", reduction)
	}
	if reduction := AntiReverse(-80); reduction != 80 {
		t.Fatalf("80 kW reverse must reduce 80, got %.1f", reduction)
	}
}

func TestApplyAntiReverse(t *testing.T) {
	caps := CapabilityValues{
		"eu-01": {FieldRatedPower: 250, FieldAvailablePower: 250},
		"eu-02": {FieldRatedPower: 250, FieldAvailablePower: 200},
	}
	allocations, err := ApplyAntiReverse(300, []EnergyMember{{EnergyUnitID: "eu-01"}, {EnergyUnitID: "eu-02"}}, caps)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, allocation := range allocations {
		total += allocation.SetpointKW
	}
	// 削减 300，分配为负（充电方向），完全满足（能力 450）。
	if total >= 0 || total < -300 {
		t.Fatalf("unexpected anti-reverse total %.1f", total)
	}
}

func TestPeakShaving(t *testing.T) {
	if shave := PeakShaving(800, 500); shave != 300 {
		t.Fatalf("expected 300 kW shave, got %.1f", shave)
	}
	if shave := PeakShaving(400, 500); shave != 0 {
		t.Fatalf("below limit must not shave, got %.1f", shave)
	}
}

func TestApplyPeakShaving(t *testing.T) {
	caps := CapabilityValues{
		"eu-01": {FieldRatedPower: 250, FieldAvailablePower: 250},
		"eu-02": {FieldRatedPower: 250, FieldAvailablePower: 150},
	}
	allocations, err := ApplyPeakShaving(300, []EnergyMember{{EnergyUnitID: "eu-01"}, {EnergyUnitID: "eu-02"}}, caps)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, allocation := range allocations {
		total += allocation.SetpointKW
	}
	// 削峰 300，能力 400，完全满足。
	if total <= 0 || total > 300 {
		t.Fatalf("unexpected peak-shaving total %.1f", total)
	}
}
