package decision

import (
	"fmt"
	"time"
)

// Scenario is one saved simulation: an input snapshot, an optional simulated
// moment and the expected outcome. Scenarios live in the draft document and
// never touch a device or the published behaviour.
type Scenario struct {
	ID         string             `json:"scenario_id"`
	Name       string             `json:"name,omitempty"`
	Inputs     map[string]float64 `json:"inputs,omitempty"`
	TextInputs map[string]string  `json:"text_inputs,omitempty"`
	// At is the simulated moment ("2006-01-02T15:04" local or RFC3339);
	// empty means the moment the scenarios are run.
	At string `json:"at,omitempty"`
	// ExpectRow is the row that must match; empty expects no match.
	ExpectRow string `json:"expect_row,omitempty"`
	// ExpectActions, when present, must equal the simulated actions in order.
	ExpectActions []Action `json:"expect_actions,omitempty"`
}

type ScenarioResult struct {
	ScenarioID string             `json:"scenario_id"`
	Name       string             `json:"name,omitempty"`
	Passed     bool               `json:"passed"`
	RowID      string             `json:"row_id,omitempty"`
	Actions    []SimulationAction `json:"actions,omitempty"`
	// Reason is "row_mismatch", "action_mismatch" or "error" when not passed.
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (d Document) validateScenarios() error {
	seen := make(map[string]bool, len(d.Scenarios))
	for i, scenario := range d.Scenarios {
		if scenario.ID == "" {
			return fmt.Errorf("scenario %d: missing scenario_id", i+1)
		}
		if seen[scenario.ID] {
			return fmt.Errorf("scenario %d: duplicate scenario_id", i+1)
		}
		seen[scenario.ID] = true
		if _, err := ParseSimulationTime(scenario.At, time.Time{}); err != nil {
			return fmt.Errorf("scenario %s: %w", scenario.ID, err)
		}
		for input, value := range scenario.Inputs {
			if !finite(value) {
				return fmt.Errorf("scenario %s: input %s must be finite", scenario.ID, input)
			}
		}
		for _, action := range scenario.ExpectActions {
			if action.ActionID == "" || !finite(action.Value) {
				return fmt.Errorf("scenario %s: invalid expected action", scenario.ID)
			}
		}
	}
	return nil
}

// ParseSimulationTime accepts "2006-01-02T15:04" (site-local, optional
// seconds) or RFC3339; empty returns fallback.
func ParseSimulationTime(at string, fallback time.Time) (time.Time, error) {
	if at == "" {
		return fallback, nil
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, at, time.Local); err == nil {
			return t, nil
		}
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return time.Time{}, fmt.Errorf("time %q must be YYYY-MM-DDTHH:MM", at)
	}
	return t, nil
}

// RunScenarios simulates every scenario against this document. It is pure:
// no state, no commands. A failing scenario carries a machine-readable reason.
func (d Document) RunScenarios(now time.Time) []ScenarioResult {
	results := make([]ScenarioResult, 0, len(d.Scenarios))
	for _, scenario := range d.Scenarios {
		result := ScenarioResult{ScenarioID: scenario.ID, Name: scenario.Name}
		at, err := ParseSimulationTime(scenario.At, now)
		if err == nil {
			var simulation SimulationResult
			simulation, err = d.Simulate(scenario.Inputs, scenario.TextInputs, at)
			if err == nil {
				result.RowID = simulation.RowID
				result.Actions = simulation.Actions
				result.Passed, result.Reason = scenarioVerdict(scenario, simulation)
			}
		}
		if err != nil {
			result.Reason = "error"
			result.Detail = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func scenarioVerdict(scenario Scenario, simulation SimulationResult) (bool, string) {
	if simulation.RowID != scenario.ExpectRow {
		return false, "row_mismatch"
	}
	if len(scenario.ExpectActions) == 0 {
		return true, ""
	}
	if len(simulation.Actions) != len(scenario.ExpectActions) {
		return false, "action_mismatch"
	}
	for i, expected := range scenario.ExpectActions {
		actual := simulation.Actions[i]
		if actual.ActionID != expected.ActionID || actual.Value != expected.Value {
			return false, "action_mismatch"
		}
	}
	return true, ""
}
