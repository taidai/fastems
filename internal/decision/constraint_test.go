package decision

import "testing"

func TestEvaluateConstraintsReportsViolations(t *testing.T) {
	read := func(target Target) (float64, bool) {
		switch target.NodeID {
		case "soc":
			return 95, true
		case "temp":
			return 57, true
		case "grid":
			return -120, true
		case "missing":
			return 0, false
		}
		return 0, false
	}
	constraints := []MemberConstraint{
		{Kind: "SOC", Target: Target{NodeID: "soc"}, Operator: "gt", Limit: 90, Direction: "charge"},
		{Kind: "temperature", Target: Target{NodeID: "temp"}, Operator: "gt", Limit: 55, Direction: "both"},
		{Kind: "grid", Target: Target{NodeID: "grid"}, Operator: "lt", Limit: -100, Direction: "discharge"},
		{Kind: "ignored", Target: Target{NodeID: "missing"}, Operator: "gt", Limit: 1},
	}
	violations := EvaluateConstraints(constraints, read)
	if len(violations) != 3 {
		t.Fatalf("want 3 violations, got %d: %+v", len(violations), violations)
	}
	if violations[0].Kind != "SOC" || violations[0].Direction != "charge" || violations[0].Reason == "" {
		t.Errorf("unexpected SOC violation: %+v", violations[0])
	}
	if violations[1].Kind != "temperature" || violations[1].Direction != "both" {
		t.Errorf("unexpected temperature violation: %+v", violations[1])
	}
	if violations[2].Kind != "grid" || violations[2].Direction != "discharge" {
		t.Errorf("unexpected grid violation: %+v", violations[2])
	}
}

func TestEvaluateConstraintsNoViolation(t *testing.T) {
	read := func(target Target) (float64, bool) {
		if target.NodeID == "soc" {
			return 80, true
		}
		return 0, false
	}
	violations := EvaluateConstraints([]MemberConstraint{
		{Kind: "SOC", Target: Target{NodeID: "soc"}, Operator: "gt", Limit: 90, Direction: "charge"},
	}, read)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %+v", violations)
	}
}

func TestConstraintDefaultDirectionIsBoth(t *testing.T) {
	c := MemberConstraint{Kind: "temp", Target: Target{NodeID: "t"}, Operator: "gt", Limit: 55}
	if c.direction() != "both" {
		t.Fatalf("default direction = %q, want both", c.direction())
	}
}

func TestConstraintValidation(t *testing.T) {
	valid := MemberConstraint{Kind: "SOC", Target: Target{NodeID: "soc"}, Operator: "gt", Limit: 90, Direction: "charge"}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid constraint rejected: %v", err)
	}
	for name, constraint := range map[string]MemberConstraint{
		"empty kind":     {Kind: "", Target: Target{NodeID: "soc"}, Operator: "gt", Limit: 1},
		"missing node":   {Kind: "SOC", Operator: "gt", Limit: 1},
		"bad operator":   {Kind: "SOC", Target: Target{NodeID: "soc"}, Operator: "eq", Limit: 1},
		"bad direction":  {Kind: "SOC", Target: Target{NodeID: "soc"}, Operator: "gt", Limit: 1, Direction: "sideways"},
	} {
		if err := constraint.validate(); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
