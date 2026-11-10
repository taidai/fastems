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

func TestAllocateEnergyDirectionalCapabilityAndStatus(t *testing.T) {
	caps := CapabilityValues{
		"eu": {
			FieldRatedPower: 100, FieldAvailableCharge: 30, FieldAvailableDischarge: 80,
			FieldStatus: 1, FieldEnergyType: EnergyStorageBidir,
		},
	}
	members := []EnergyMember{{EnergyUnitID: "eu"}}
	discharge, err := AllocateEnergy(100, members, caps)
	if err != nil || len(discharge) != 1 || discharge[0].SetpointKW != 80 {
		t.Fatalf("放电应受 80 kW 上限约束：%+v err=%v", discharge, err)
	}
	charge, err := AllocateEnergy(-100, members, caps)
	if err != nil || len(charge) != 1 || charge[0].SetpointKW != -30 {
		t.Fatalf("充电应受 30 kW 上限约束：%+v err=%v", charge, err)
	}
	caps["eu"][FieldStatus] = 0
	offline, err := AllocateEnergy(50, members, caps)
	if err != nil || len(offline) != 1 || !offline[0].Excluded {
		t.Fatalf("离线/互锁成员必须剔除：%+v err=%v", offline, err)
	}
}
