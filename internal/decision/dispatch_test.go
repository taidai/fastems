package decision

import "testing"

func TestDispatchCapability(t *testing.T) {
	values := CapabilityValues{
		"eu-01": {FieldSOC: 95, FieldAvailablePower: 200, FieldRatedPower: 250},
	}
	cabinets := map[string][]DeviceCapability{
		"eu-01": {
			{DeviceID: "pcs-01", RatedPowerKW: 100, AvailablePowerKW: 100},
			{DeviceID: "pcs-02", RatedPowerKW: 100, AvailablePowerKW: 100},
		},
	}

	rule := CapabilityRule{
		ID: "discharge-150",
		Conditions: []CapabilityCondition{
			{Ref: CapabilityRef{EnergyUnitID: "eu-01", Field: FieldSOC}, Operator: "gt", Value: 50},
		},
		Actions: []CapabilityAction{
			{EnergyUnitID: "eu-01", RequestKW: 150},
		},
	}

	t.Run("dispatches power to devices when rule matches", func(t *testing.T) {
		dispatches := DispatchCapability(rule, values, cabinets)
		if len(dispatches) != 2 {
			t.Fatalf("expected 2 device dispatches, got %d", len(dispatches))
		}
		total := 0.0
		for _, dispatch := range dispatches {
			total += dispatch.SetpointKW
		}
		if total <= 0 {
			t.Fatalf("expected positive total setpoint, got %.1f", total)
		}
	})

	t.Run("no dispatch when rule does not match", func(t *testing.T) {
		failing := rule
		failing.Conditions[0].Value = 99
		if dispatches := DispatchCapability(failing, values, cabinets); len(dispatches) != 0 {
			t.Fatalf("expected no dispatches, got %+v", dispatches)
		}
	})
}
