package decision

import "testing"

func TestEvaluateCapability(t *testing.T) {
	values := CapabilityValues{
		"eu-01": {
			FieldSOC:            95,
			FieldAvailablePower: 200,
			FieldRatedPower:     250,
		},
	}

	t.Run("match", func(t *testing.T) {
		condition := CapabilityCondition{Ref: CapabilityRef{EnergyUnitID: "eu-01", Field: FieldSOC}, Operator: "gt", Value: 90}
		if !EvaluateCapability(condition, values) {
			t.Fatal("expected match")
		}
	})

	t.Run("no match", func(t *testing.T) {
		condition := CapabilityCondition{Ref: CapabilityRef{EnergyUnitID: "eu-01", Field: FieldSOC}, Operator: "lt", Value: 10}
		if EvaluateCapability(condition, values) {
			t.Fatal("expected no match")
		}
	})

	t.Run("missing energy unit", func(t *testing.T) {
		condition := CapabilityCondition{Ref: CapabilityRef{EnergyUnitID: "eu-02", Field: FieldSOC}, Operator: "gt", Value: 0}
		if EvaluateCapability(condition, values) {
			t.Fatal("missing unit must not match")
		}
	})

	t.Run("missing field", func(t *testing.T) {
		condition := CapabilityCondition{Ref: CapabilityRef{EnergyUnitID: "eu-01", Field: FieldStatus}, Operator: "eq", Value: 0}
		if EvaluateCapability(condition, values) {
			t.Fatal("missing field must not match")
		}
	})
}
