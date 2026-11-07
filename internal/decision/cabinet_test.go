package decision

import "testing"

func TestDispatchCabinet(t *testing.T) {
	devices := []DeviceCapability{
		{DeviceID: "pcs-01", RatedPowerKW: 50, AvailablePowerKW: 50},
		{DeviceID: "pcs-02", RatedPowerKW: 50, AvailablePowerKW: 30},
		{DeviceID: "pcs-03", RatedPowerKW: 50, AvailablePowerKW: 0},
	}

	t.Run("splits by device capability", func(t *testing.T) {
		dispatches, err := DispatchCabinet(80, devices)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]DeviceDispatch{}
		for _, dispatch := range dispatches {
			byID[dispatch.DeviceID] = dispatch
		}
		if byID["pcs-01"].SetpointKW <= 0 || byID["pcs-02"].SetpointKW <= 0 {
			t.Fatalf("expected positive setpoints, got %+v", byID)
		}
		if !byID["pcs-03"].Excluded {
			t.Fatalf("zero-capacity device must be excluded: %+v", byID["pcs-03"])
		}
	})

	t.Run("shortfall beyond cabinet capability", func(t *testing.T) {
		dispatches, err := DispatchCabinet(100, devices[:2])
		if err != nil {
			t.Fatal(err)
		}
		total := 0.0
		for _, dispatch := range dispatches {
			total += dispatch.SetpointKW
		}
		if total >= 100 {
			t.Fatalf("expected shortfall below 100, got %.1f", total)
		}
	})
}
