package decision

import (
	"fmt"
	"math"
)

// MemberConstraint guards one direction of a member's power capability. When
// the input reading violates the limit the member is derated in that direction
// (capacity zeroed) or removed from the allocation entirely (direction "both"),
// and the reason is recorded so the operator can see why.
type MemberConstraint struct {
	// Kind names the limit in the explanation, e.g. "SOC", "temperature", "grid".
	Kind string `json:"kind"`
	// Target is the input point that carries the current reading.
	Target Target `json:"target"`
	// Operator is one of gt, ge, lt, le.
	Operator string `json:"operator"`
	Limit    float64 `json:"limit"`
	// Direction is "charge", "discharge" or "both" (default). "both" removes the
	// member entirely; a single direction zeroes that direction's capability.
	Direction string `json:"direction,omitempty"`
}

func (c MemberConstraint) validate() error {
	if c.Kind == "" {
		return fmt.Errorf("constraint kind is empty")
	}
	if c.Target.NodeID == "" {
		return fmt.Errorf("constraint %s has no input node", c.Kind)
	}
	switch c.Operator {
	case "gt", "ge", "lt", "le":
	default:
		return fmt.Errorf("constraint %s: unsupported operator %q", c.Kind, c.Operator)
	}
	if math.IsNaN(c.Limit) || math.IsInf(c.Limit, 0) {
		return fmt.Errorf("constraint %s: limit must be finite", c.Kind)
	}
	switch c.Direction {
	case "", "charge", "discharge", "both":
	default:
		return fmt.Errorf("constraint %s: direction must be charge, discharge or both", c.Kind)
	}
	return nil
}

func (c MemberConstraint) direction() string {
	if c.Direction == "" {
		return "both"
	}
	return c.Direction
}

// ConstraintViolation is one violated member limit and its explanation.
type ConstraintViolation struct {
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	Reason    string `json:"reason"`
}

// EvaluateConstraints reports which limits a member's current readings violate.
// read is injectable so the decision is pure and unit-testable. A missing or
// unreadable input does not violate a constraint: a failed sensor must not
// silently derate a healthy member, and the member's own fault handling already
// covers unavailable capability sources.
func EvaluateConstraints(constraints []MemberConstraint, read func(Target) (float64, bool)) []ConstraintViolation {
	violations := make([]ConstraintViolation, 0, len(constraints))
	for _, c := range constraints {
		value, found := read(c.Target)
		if !found {
			continue
		}
		if !compare(value, c.Operator, c.Limit) {
			continue
		}
		direction := c.direction()
		effect := "derated"
		if direction == "both" {
			effect = "excluded"
		}
		violations = append(violations, ConstraintViolation{
			Kind: c.Kind, Direction: direction,
			Reason: fmt.Sprintf("%s %.1f %s %.1f, %s", c.Kind, value, operatorWord(c.Operator), c.Limit, effect),
		})
	}
	return violations
}

func operatorWord(op string) string {
	switch op {
	case "gt":
		return "exceeds"
	case "ge":
		return "meets or exceeds"
	case "lt":
		return "is below"
	case "le":
		return "is at or below"
	}
	return op
}
