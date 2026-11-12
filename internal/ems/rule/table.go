// Package rule implements the ordered decision table: typed conditions, AND
// within a row, first matching row from top to bottom, hold time, cooldown,
// input snapshots and explainable limiting.
package rule

import (
	"fmt"
	"time"

	"fastiot/internal/ems/signal"
)

// Condition compares one input with a typed value.
type Condition struct {
	Input string
	Op    string
	Value signal.Value
}

// Action requests a value on one output. Outputs are point targets or
// allocator targets; the runtime maps them to device writes.
type Action struct {
	Output string
	Value  signal.Value
}

// Row is one rule: all conditions must hold, then the actions are requested.
// A row with no conditions is the default row and must be last.
type Row struct {
	ID         string
	Name       string
	Conditions []Condition
	Hold       time.Duration
	Cooldown   time.Duration
	Actions    []Action
}

// Table is one versioned decision table.
type Table struct {
	ID      string
	Version int
	// MaxAge rejects inputs older than this at evaluation time. Zero disables
	// the age check, but quality and finiteness are always enforced.
	MaxAge time.Duration
	Rows   []Row
}

// Validate rejects ambiguous or unsafe tables.
func (t Table) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("table id is empty")
	}
	if t.MaxAge < 0 {
		return fmt.Errorf("table %s: max age is negative", t.ID)
	}
	if len(t.Rows) == 0 {
		return fmt.Errorf("table %s: no rows", t.ID)
	}
	seen := make(map[string]bool, len(t.Rows))
	defaultRows := 0
	for i, row := range t.Rows {
		if row.ID == "" {
			return fmt.Errorf("table %s: row %d has no id", t.ID, i+1)
		}
		if seen[row.ID] {
			return fmt.Errorf("table %s: duplicate row id %q", t.ID, row.ID)
		}
		seen[row.ID] = true
		if row.Hold < 0 || row.Cooldown < 0 {
			return fmt.Errorf("row %s: hold and cooldown must not be negative", row.ID)
		}
		if len(row.Conditions) == 0 {
			defaultRows++
			if i != len(t.Rows)-1 {
				return fmt.Errorf("row %s: the default row must be last", row.ID)
			}
		}
		for _, c := range row.Conditions {
			if c.Input == "" {
				return fmt.Errorf("row %s: condition without input", row.ID)
			}
			if !c.Value.Valid() {
				return fmt.Errorf("row %s: condition value for %s is invalid", row.ID, c.Input)
			}
			if !c.Value.ValidOperator(c.Op) {
				return fmt.Errorf("row %s: operator %q is not defined for %s", row.ID, c.Op, c.Value.Kind)
			}
		}
		outputs := make(map[string]bool, len(row.Actions))
		for _, a := range row.Actions {
			if a.Output == "" {
				return fmt.Errorf("row %s: action without output", row.ID)
			}
			if !a.Value.Valid() {
				return fmt.Errorf("row %s: action value for %s is invalid", row.ID, a.Output)
			}
			if outputs[a.Output] {
				return fmt.Errorf("row %s: duplicate action on %s", row.ID, a.Output)
			}
			outputs[a.Output] = true
		}
	}
	if defaultRows > 1 {
		return fmt.Errorf("table %s: more than one default row", t.ID)
	}
	return nil
}

// ValidateOutputs rejects actions on outputs this rule does not own. A nil
// owned set means ownership is not checked (used by pure simulation).
func (t Table) ValidateOutputs(owned map[string]bool) error {
	if owned == nil {
		return nil
	}
	for _, row := range t.Rows {
		for _, a := range row.Actions {
			if !owned[a.Output] {
				return fmt.Errorf("row %s: output %s is not owned by this table", row.ID, a.Output)
			}
		}
	}
	return nil
}
