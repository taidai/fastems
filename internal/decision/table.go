package decision

import (
	"fmt"
	"math"
)

type Table struct {
	Rows []Row `json:"rows"`
}

type Row struct {
	ID         string      `json:"row_id"`
	Conditions []Condition `json:"conditions"`
	Actions    []Action    `json:"actions"`
	// HoldMS delays firing until the conditions have held this long.
	HoldMS int `json:"hold_ms,omitempty"`
	// CooldownMS blocks re-firing the row for this long after it fired.
	CooldownMS int `json:"cooldown_ms,omitempty"`
}

type Condition struct {
	PointID  string  `json:"point_id"`
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
	// Kind is "number" (default), "bool" or "enum". Bool uses Value 0/1;
	// enum compares Text as a label.
	Kind string `json:"kind,omitempty"`
	Text string `json:"text,omitempty"`
}

type Action struct {
	ActionID string  `json:"action_id"`
	Value    float64 `json:"value"`
}

type Result struct {
	RowID   string   `json:"row_id,omitempty"`
	Actions []Action `json:"actions,omitempty"`
}

func (d Document) Validate() error {
	if err := d.Table.Validate(); err != nil {
		return err
	}
	for _, row := range d.Table.Rows {
		for _, condition := range row.Conditions {
			target, ok := d.Bindings.Points[condition.PointID]
			if !ok || target.NodeID == "" {
				return fmt.Errorf("row %s: point %s is not bound", row.ID, condition.PointID)
			}
		}
		for _, action := range row.Actions {
			target, ok := d.Bindings.Actions[action.ActionID]
			if !ok || target.NodeID == "" {
				return fmt.Errorf("row %s: action %s is not bound", row.ID, action.ActionID)
			}
		}
	}
	return nil
}

func (t Table) Validate() error {
	rows := make(map[string]bool, len(t.Rows))
	for i, row := range t.Rows {
		if row.ID == "" {
			return fmt.Errorf("row %d: missing row_id", i+1)
		}
		if rows[row.ID] {
			return fmt.Errorf("row %d: duplicate row_id", i+1)
		}
		rows[row.ID] = true
		if row.HoldMS < 0 || row.CooldownMS < 0 {
			return fmt.Errorf("row %s: hold and cooldown must not be negative", row.ID)
		}
		for _, condition := range row.Conditions {
			if condition.PointID == "" {
				return fmt.Errorf("row %s: missing point_id", row.ID)
			}
			switch condition.Kind {
			case "", "number":
				if !finite(condition.Value) {
					return fmt.Errorf("row %s: condition value must be finite", row.ID)
				}
				switch condition.Operator {
				case "eq", "ne", "gt", "ge", "lt", "le":
				default:
					return fmt.Errorf("row %s: unsupported operator %q", row.ID, condition.Operator)
				}
			case "bool":
				if condition.Value != 0 && condition.Value != 1 {
					return fmt.Errorf("row %s: bool condition value must be 0 or 1", row.ID)
				}
				if condition.Operator != "eq" && condition.Operator != "ne" {
					return fmt.Errorf("row %s: bool condition only supports eq/ne", row.ID)
				}
			case "enum":
				if condition.Text == "" {
					return fmt.Errorf("row %s: enum condition needs a label", row.ID)
				}
				if condition.Operator != "eq" && condition.Operator != "ne" {
					return fmt.Errorf("row %s: enum condition only supports eq/ne", row.ID)
				}
			default:
				return fmt.Errorf("row %s: unsupported condition kind %q", row.ID, condition.Kind)
			}
		}
		actions := make(map[string]bool, len(row.Actions))
		for _, action := range row.Actions {
			if action.ActionID == "" {
				return fmt.Errorf("row %s: missing action_id", row.ID)
			}
			if actions[action.ActionID] {
				return fmt.Errorf("row %s: duplicate action_id", row.ID)
			}
			actions[action.ActionID] = true
			if !finite(action.Value) {
				return fmt.Errorf("row %s: action value must be finite", row.ID)
			}
		}
	}
	return nil
}

// Evaluate returns the first matching row. A row with no conditions is the
// default row; missing inputs never match.
func Evaluate(t Table, values map[string]float64) (Result, error) {
	if err := t.Validate(); err != nil {
		return Result{}, err
	}
	for _, row := range t.Rows {
		matched := true
		for _, condition := range row.Conditions {
			value, ok := values[condition.PointID]
			if !ok || !finite(value) {
				matched = false
				break
			}
			if !compare(value, condition.Operator, condition.Value) {
				matched = false
				break
			}
		}
		if matched {
			return Result{RowID: row.ID, Actions: row.Actions}, nil
		}
	}
	return Result{}, nil
}

func compare(value float64, operator string, target float64) bool {
	switch operator {
	case "eq":
		return value == target
	case "ne":
		return value != target
	case "gt":
		return value > target
	case "ge":
		return value >= target
	case "lt":
		return value < target
	case "le":
		return value <= target
	default:
		return false
	}
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
