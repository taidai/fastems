package decision

import "testing"

func TestAllocateEnergy(t *testing.T) {
	caps := CapabilityValues{
		"eu-01": {FieldRatedPower: 250, FieldAvailablePower: 250},
		"eu-02": {FieldRatedPower: 250, FieldAvailablePower: 200},
		"eu-03": {FieldRatedPower: 250, FieldAvailablePower: 0},
	}

	t.Run("fills by capability", func(t *testing.T) {
		allocations, err := AllocateEnergy(400, []EnergyMember{
			{EnergyUnitID: "eu-01"}, {EnergyUnitID: "eu-02"}, {EnergyUnitID: "eu-03"},
		}, caps)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]EnergyAllocation{}
		for _, allocation := range allocations {
			byID[allocation.EnergyUnitID] = allocation
		}
		if byID["eu-01"].SetpointKW <= 0 || byID["eu-02"].SetpointKW <= 0 {
			t.Fatalf("expected positive allocations, got %+v", byID)
		}
		if !byID["eu-03"].Excluded || byID["eu-03"].Reason == "" {
			t.Fatalf("zero-capacity member must be excluded with a reason: %+v", byID["eu-03"])
		}
	})

	t.Run("shortfall when request exceeds capability", func(t *testing.T) {
		allocations, err := AllocateEnergy(600, []EnergyMember{
			{EnergyUnitID: "eu-01"}, {EnergyUnitID: "eu-02"},
		}, caps)
		if err != nil {
			t.Fatal(err)
		}
		total := 0.0
		for _, allocation := range allocations {
			total += allocation.SetpointKW
		}
		if total >= 600 {
			t.Fatalf("expected shortfall below 600, got %.1f", total)
		}
	})

	t.Run("missing energy unit capacity", func(t *testing.T) {
		allocations, err := AllocateEnergy(100, []EnergyMember{{EnergyUnitID: "eu-missing"}}, caps)
		if err != nil {
			t.Fatal(err)
		}
		if len(allocations) != 1 || !allocations[0].Excluded {
			t.Fatalf("missing unit must be excluded: %+v", allocations)
		}
	})
}
