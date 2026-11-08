package decision

import "testing"

func TestAllocateSite(t *testing.T) {
	plants := []SitePlant{
		{PlantID: "plant-a", RatedPowerKW: 500, AvailablePowerKW: 400},
		{PlantID: "plant-b", RatedPowerKW: 500, AvailablePowerKW: 300},
		{PlantID: "plant-c", RatedPowerKW: 500, AvailablePowerKW: 0},
	}

	t.Run("splits by plant capability", func(t *testing.T) {
		allocations, err := AllocateSite(600, plants)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]SiteAllocation{}
		for _, allocation := range allocations {
			byID[allocation.PlantID] = allocation
		}
		if byID["plant-a"].SetpointKW <= 0 || byID["plant-b"].SetpointKW <= 0 {
			t.Fatalf("expected positive allocations, got %+v", byID)
		}
		if !byID["plant-c"].Excluded {
			t.Fatalf("zero-capacity plant must be excluded: %+v", byID["plant-c"])
		}
	})

	t.Run("shortfall beyond site capability", func(t *testing.T) {
		allocations, err := AllocateSite(1000, plants[:2])
		if err != nil {
			t.Fatal(err)
		}
		total := 0.0
		for _, allocation := range allocations {
			total += allocation.SetpointKW
		}
		if total >= 1000 {
			t.Fatalf("expected shortfall below 1000, got %.1f", total)
		}
	})
}
