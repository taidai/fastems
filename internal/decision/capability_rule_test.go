package decision

import "testing"

func TestEvaluateCapabilityRule(t *testing.T) {
	values := CapabilityValues{
		"eu-01": {FieldSOC: 95, FieldAvailablePower: 200, FieldRatedPower: 250},
	}

	rule := CapabilityRule{
		ID: "soc-protect",
		Conditions: []CapabilityCondition{
			{Ref: CapabilityRef{EnergyUnitID: "eu-01", Field: FieldSOC}, Operator: "gt", Value: 90},
		},
		Actions: []CapabilityAction{
			{EnergyUnitID: "eu-01", RequestKW: 0},
		},
	}

	t.Run("matches when conditions hold", func(t *testing.T) {
		actions := EvaluateCapabilityRule(rule, values)
		if len(actions) != 1 || actions[0].EnergyUnitID != "eu-01" {
			t.Fatalf("expected matched action, got %+v", actions)
		}
	})

	t.Run("no match when condition fails", func(t *testing.T) {
		failing := rule
		failing.Conditions[0].Value = 99
		if actions := EvaluateCapabilityRule(failing, values); actions != nil {
			t.Fatalf("expected nil, got %+v", actions)
		}
	})

	t.Run("no match on missing value", func(t *testing.T) {
		missing := rule
		missing.Conditions[0].Ref.EnergyUnitID = "eu-missing"
		if actions := EvaluateCapabilityRule(missing, values); actions != nil {
			t.Fatalf("expected nil for missing unit, got %+v", actions)
		}
	})
}
