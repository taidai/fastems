package safety

import (
	"fmt"

	"easyems/internal/ems/signal"
)

// ConditionConfig is the wire format for one safety condition. PointID is the
// source node id; it is intentionally not a business id so the safety layer
// keeps working when a device is replaced.
type ConditionConfig struct {
	PointID  string  `json:"point_id"`
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
	Kind     string  `json:"kind,omitempty"`
	Text     string  `json:"text,omitempty"`
}

// Config is the per-output safety envelope stored on the target node.
type Config struct {
	Min          *float64          `json:"min,omitempty"`
	Max          *float64          `json:"max,omitempty"`
	MaxDischarge *float64          `json:"max_discharge,omitempty"`
	MaxCharge    *float64          `json:"max_charge,omitempty"`
	// RampPerSecond limits how fast a numeric setpoint moves. Zero/absent
	// means the target is written directly.
	RampPerSecond *float64          `json:"ramp_per_second,omitempty"`
	Conditions    []ConditionConfig `json:"conditions,omitempty"`
}

// Output converts and validates a stored config into the runtime envelope.
func (c Config) Output() (Output, error) {
	out := Output{Kind: signal.Number}
	if c.Min != nil && c.Max != nil {
		if *c.Min >= *c.Max {
			return out, fmt.Errorf("safety range min must be below max")
		}
		out.HasRange, out.Min, out.Max = true, *c.Min, *c.Max
	} else if c.Min != nil || c.Max != nil {
		return out, fmt.Errorf("safety range needs both min and max")
	}
	if c.MaxDischarge != nil {
		if *c.MaxDischarge < 0 {
			return out, fmt.Errorf("max discharge must not be negative")
		}
		out.CapPositive = *c.MaxDischarge
	}
	if c.MaxCharge != nil {
		if *c.MaxCharge < 0 {
			return out, fmt.Errorf("max charge must not be negative")
		}
		out.CapNegative = *c.MaxCharge
	}
	if c.RampPerSecond != nil {
		if *c.RampPerSecond < 0 {
			return out, fmt.Errorf("ramp must not be negative")
		}
		out.Ramp = *c.RampPerSecond
	}
	for _, condition := range c.Conditions {
		if condition.PointID == "" {
			return out, fmt.Errorf("safety condition without point")
		}
		value := signal.Num(condition.Value)
		switch condition.Kind {
		case "", "number":
			switch condition.Operator {
			case "eq", "ne", "gt", "ge", "lt", "le":
			default:
				return out, fmt.Errorf("unsupported safety operator %q", condition.Operator)
			}
		case "bool":
			if condition.Value != 0 && condition.Value != 1 {
				return out, fmt.Errorf("bool safety value must be 0 or 1")
			}
			if condition.Operator != "eq" && condition.Operator != "ne" {
				return out, fmt.Errorf("bool safety only supports eq/ne")
			}
			value = signal.Flag(condition.Value != 0)
		case "enum":
			if condition.Text == "" {
				return out, fmt.Errorf("enum safety condition needs a label")
			}
			if condition.Operator != "eq" && condition.Operator != "ne" {
				return out, fmt.Errorf("enum safety only supports eq/ne")
			}
			value = signal.Label(condition.Text)
		default:
			return out, fmt.Errorf("unsupported safety kind %q", condition.Kind)
		}
		out.Conditions = append(out.Conditions, Condition{Input: condition.PointID, Op: condition.Operator, Value: value})
	}
	return out, nil
}
