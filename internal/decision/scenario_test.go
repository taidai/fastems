package decision

import (
	"math"
	"testing"
	"time"
)

func scenarioDocument(scenarios ...Scenario) Document {
	document := typedDocument(Condition{PointID: "state", Operator: "gt", Value: 30})
	document.Scenarios = scenarios
	return document
}

func TestRunScenariosVerdicts(t *testing.T) {
	document := scenarioDocument(
		Scenario{ID: "hit", Inputs: map[string]float64{"state": 35}, ExpectRow: "row-1",
			ExpectActions: []Action{{ActionID: "set", Value: 1}}},
		Scenario{ID: "no-match", Inputs: map[string]float64{"state": 20}},
		Scenario{ID: "wrong-row", Inputs: map[string]float64{"state": 35}},
		Scenario{ID: "wrong-action", Inputs: map[string]float64{"state": 35}, ExpectRow: "row-1",
			ExpectActions: []Action{{ActionID: "set", Value: 2}}},
	)
	if err := document.Validate(); err != nil {
		t.Fatal(err)
	}
	results := document.RunScenarios(time.Now())
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	expect := map[string]struct {
		passed bool
		reason string
	}{
		"hit":          {true, ""},
		"no-match":     {true, ""},
		"wrong-row":    {false, "row_mismatch"},
		"wrong-action": {false, "action_mismatch"},
	}
	for _, result := range results {
		want := expect[result.ScenarioID]
		if result.Passed != want.passed || result.Reason != want.reason {
			t.Fatalf("scenario %s: got passed=%v reason=%q, want %+v", result.ScenarioID, result.Passed, result.Reason, want)
		}
	}
}

func TestRunScenariosAtSimulatedTime(t *testing.T) {
	document := timeDocument(Condition{Kind: "time", Start: "22:00", End: "06:00", Weekdays: maskMonToFri})
	document.Scenarios = []Scenario{
		{ID: "night", Inputs: map[string]float64{"temp": 35}, At: "2026-01-05T23:30", ExpectRow: "night"},
		{ID: "after-midnight", Inputs: map[string]float64{"temp": 35}, At: "2026-01-06T03:00", ExpectRow: "night"},
		{ID: "daytime", Inputs: map[string]float64{"temp": 35}, At: "2026-01-05T12:00"},
		{ID: "weekend", Inputs: map[string]float64{"temp": 35}, At: "2026-01-03T23:30"},
	}
	for _, result := range document.RunScenarios(time.Now()) {
		if !result.Passed {
			t.Fatalf("scenario %s failed: reason=%s detail=%s row=%s", result.ScenarioID, result.Reason, result.Detail, result.RowID)
		}
	}
}

func TestScenarioValidation(t *testing.T) {
	for _, document := range []Document{
		scenarioDocument(Scenario{ID: ""}),
		scenarioDocument(Scenario{ID: "a"}, Scenario{ID: "a"}),
		scenarioDocument(Scenario{ID: "a", At: "tomorrow"}),
		scenarioDocument(Scenario{ID: "a", Inputs: map[string]float64{"state": math.Inf(1)}}),
		scenarioDocument(Scenario{ID: "a", ExpectActions: []Action{{ActionID: ""}}}),
	} {
		if err := document.Validate(); err == nil {
			t.Fatalf("expected invalid scenarios to fail: %+v", document.Scenarios)
		}
	}
}

func TestParseSimulationTime(t *testing.T) {
	fallback := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	if at, err := ParseSimulationTime("", fallback); err != nil || !at.Equal(fallback) {
		t.Fatalf("empty should return fallback: %v %v", at, err)
	}
	local, err := ParseSimulationTime("2026-01-05T23:30", fallback)
	if err != nil || local.Hour() != 23 || local.Minute() != 30 || local.Weekday() != time.Monday {
		t.Fatalf("local format: %v %v", local, err)
	}
	if _, err := ParseSimulationTime("2026-01-05T23:30:00+08:00", fallback); err != nil {
		t.Fatalf("RFC3339 rejected: %v", err)
	}
}
