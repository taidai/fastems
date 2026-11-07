package decision

import (
	"reflect"
	"testing"
)

func TestEvaluateOrderedTable(t *testing.T) {
	table := Table{Rows: []Row{
		{
			ID: "hot-and-occupied",
			Conditions: []Condition{
				{PointID: "temperature", Operator: "gt", Value: 30},
				{PointID: "occupied", Operator: "eq", Value: 1},
			},
			Actions: []Action{
				{ActionID: "fan", Value: 1},
				{ActionID: "damper", Value: 80},
			},
		},
		{
			ID:         "hot",
			Conditions: []Condition{{PointID: "temperature", Operator: "gt", Value: 30}},
			Actions:    []Action{{ActionID: "fan", Value: 0}},
		},
		{ID: "default"},
	}}

	result, err := Evaluate(table, map[string]float64{"temperature": 35, "occupied": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowID != "hot-and-occupied" {
		t.Fatalf("matched %q, want first row", result.RowID)
	}
	wantActions := []Action{{ActionID: "fan", Value: 1}, {ActionID: "damper", Value: 80}}
	if !reflect.DeepEqual(result.Actions, wantActions) {
		t.Fatalf("actions = %#v, want %#v", result.Actions, wantActions)
	}

	table.Rows[0], table.Rows[1] = table.Rows[1], table.Rows[0]
	result, err = Evaluate(table, map[string]float64{"temperature": 35, "occupied": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowID != "hot" || len(result.Actions) != 1 || result.Actions[0].Value != 0 {
		t.Fatalf("reordered result = %#v, want hot row only", result)
	}

	result, err = Evaluate(table, map[string]float64{"temperature": 20, "occupied": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowID != "default" || len(result.Actions) != 0 {
		t.Fatalf("default result = %#v", result)
	}
}

func TestEvaluateMissingInputAndInvalidConfig(t *testing.T) {
	table := Table{Rows: []Row{{
		ID:         "needs-input",
		Conditions: []Condition{{PointID: "temperature", Operator: "gt", Value: 30}},
		Actions:    []Action{{ActionID: "fan", Value: 1}},
	}}}
	result, err := Evaluate(table, nil)
	if err != nil || result.RowID != "" {
		t.Fatalf("missing input: result=%#v err=%v", result, err)
	}

	table.Rows[0].Conditions[0].Operator = "approximately"
	if _, err := Evaluate(table, map[string]float64{"temperature": 35}); err == nil {
		t.Fatal("invalid operator was accepted")
	}
}
