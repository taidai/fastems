package decision

import (
	"testing"
	"time"

	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/ems/signal"
)

func typedDocument(condition Condition) Document {
	return Document{
		Table: Table{Rows: []Row{{
			ID:         "row-1",
			Conditions: []Condition{condition},
			Actions:    []Action{{ActionID: "set", Value: 1}},
		}}},
		Bindings: BindingSet{
			Points:  map[string]Target{"state": {NodeID: "device-1", PointType: "value"}},
			Actions: map[string]Target{"set": {NodeID: "device-1", PointType: "valueSet"}},
		},
	}
}

func TestRuleTableKeepsTypedConditions(t *testing.T) {
	for _, step := range []struct {
		condition Condition
		kind      signal.Kind
		text      string
		flag      bool
	}{
		{Condition{PointID: "state", Operator: "eq", Value: 3}, signal.Number, "", false},
		{Condition{PointID: "state", Operator: "eq", Kind: "bool", Value: 1}, signal.Bool, "", true},
		{Condition{PointID: "state", Operator: "eq", Kind: "enum", Text: "grid"}, signal.Enum, "grid", false},
	} {
		table, err := typedDocument(step.condition).RuleTable()
		if err != nil {
			t.Fatal(err)
		}
		value := table.Rows[0].Conditions[0].Value
		if value.Kind != step.kind || value.Enum != step.text || value.Bool != step.flag {
			t.Fatalf("condition %+v became %+v", step.condition, value)
		}
	}
}

func TestSimulateEnumCondition(t *testing.T) {
	document := typedDocument(Condition{PointID: "state", Operator: "eq", Kind: "enum", Text: "grid"})
	result, err := document.Simulate(nil, map[string]string{"state": "grid"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.RowID != "row-1" {
		t.Fatalf("expected enum match, got %+v", result)
	}
	miss, err := document.Simulate(nil, map[string]string{"state": "island"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if miss.RowID != "" {
		t.Fatalf("expected no match, got %+v", miss)
	}
}

func TestEmptyTextDoesNotOverwriteNumber(t *testing.T) {
	samples := SamplesFromValues(map[string]float64{"temp": 35}, map[string]string{"temp": ""}, time.Now())
	if samples["temp"].Value.Kind != signal.Number || samples["temp"].Value.Num != 35 {
		t.Fatalf("empty text overwrote numeric sample: %+v", samples["temp"])
	}
}

func TestPointSignalKeepsText(t *testing.T) {
	value, err := pointSignal(data.NewPointString("value", "", "grid"))
	if err != nil {
		t.Fatal(err)
	}
	if value.Kind != signal.Enum || value.Enum != "grid" {
		t.Fatalf("string point became %+v", value)
	}
	number, err := pointSignal(data.NewPointFloat("value", "", 12.5))
	if err != nil {
		t.Fatal(err)
	}
	if number.Kind != signal.Number || number.Num != 12.5 {
		t.Fatalf("numeric point became %+v", number)
	}
}

func TestTypedConditionValidation(t *testing.T) {
	for _, condition := range []Condition{
		{PointID: "state", Operator: "eq", Kind: "bool", Value: 2},
		{PointID: "state", Operator: "gt", Kind: "bool", Value: 1},
		{PointID: "state", Operator: "eq", Kind: "enum"},
		{PointID: "state", Operator: "eq", Kind: "enum", Text: "grid", Value: 1},
	} {
		document := typedDocument(condition)
		if condition.Kind == "enum" && condition.Text == "" {
			if err := document.Validate(); err == nil {
				t.Fatalf("expected enum without label to fail: %+v", condition)
			}
			continue
		}
		// Enum with value is allowed because Value is ignored; the other cases must fail.
		if condition.Kind == "enum" {
			continue
		}
		if err := document.Validate(); err == nil {
			t.Fatalf("expected invalid condition to fail: %+v", condition)
		}
	}
}
