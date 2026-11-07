package decision

import (
	"testing"
	"time"
)

func TestDiffDocumentsReportsAddedRemovedChanged(t *testing.T) {
	a := Document{Table: Table{Rows: []Row{
		{ID: "r1", Conditions: []Condition{{PointID: "soc", Operator: "gt", Value: 50}}, Actions: []Action{{ActionID: "pcs", Value: 100}}},
		{ID: "r2", Conditions: []Condition{{PointID: "grid", Operator: "gt", Value: 10}}, Actions: []Action{{ActionID: "pcs", Value: 50}}},
		{ID: "default"},
	}}}
	b := Document{Table: Table{Rows: []Row{
		{ID: "r1", Conditions: []Condition{{PointID: "soc", Operator: "gt", Value: 80}}, Actions: []Action{{ActionID: "pcs", Value: 100}}},
		{ID: "r3", Conditions: []Condition{{PointID: "grid", Operator: "gt", Value: 20}}, Actions: []Action{{ActionID: "pcs", Value: 70}}},
		{ID: "default"},
	}}}
	diffs := DiffDocuments(a, b)
	kinds := make(map[string]RowDiff, len(diffs))
	for _, diff := range diffs {
		kinds[diff.RowID] = diff
	}
	if kinds["r1"].Kind != "changed" || len(kinds["r1"].Detail) == 0 {
		t.Errorf("r1 should be changed with detail, got %+v", kinds["r1"])
	}
	if kinds["r2"].Kind != "removed" || kinds["r2"].From == "" {
		t.Errorf("r2 should be removed with a summary, got %+v", kinds["r2"])
	}
	if kinds["r3"].Kind != "added" || kinds["r3"].To == "" {
		t.Errorf("r3 should be added with a summary, got %+v", kinds["r3"])
	}
	if _, ok := kinds["default"]; ok {
		t.Errorf("unchanged default row should not appear in the diff")
	}
}

func TestDiffDocumentsUnchanged(t *testing.T) {
	doc := Document{Table: Table{Rows: []Row{
		{ID: "r1", Conditions: []Condition{{PointID: "soc", Operator: "gt", Value: 50}}, Actions: []Action{{ActionID: "pcs", Value: 100}}},
	}}}
	if diffs := DiffDocuments(doc, doc); len(diffs) != 0 {
		t.Fatalf("identical documents should diff to empty, got %+v", diffs)
	}
}

func TestReplayVersionAtTime(t *testing.T) {
	doc := Document{
		Table: Table{Rows: []Row{
			{ID: "night", Conditions: []Condition{{Kind: "time", Start: "22:00", End: "06:00", Weekdays: 127}}, Actions: []Action{{ActionID: "pcs", Value: -100}}},
			{ID: "default", Actions: []Action{{ActionID: "pcs", Value: 0}}},
		}},
		Bindings: BindingSet{Actions: map[string]Target{"pcs": {NodeID: "pcs-node"}}},
	}
	at, err := ParseSimulationTime("2026-01-06T23:30", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := doc.Simulate(nil, nil, at)
	if err != nil {
		t.Fatal(err)
	}
	if result.RowID != "night" {
		t.Fatalf("replay at 23:30 should match night row, got %q", result.RowID)
	}
}

func TestBuildDependencyGraph(t *testing.T) {
	doc := Document{Table: Table{Rows: []Row{
		{ID: "r1", Conditions: []Condition{{PointID: "soc", Operator: "gt", Value: 50}, {Kind: "time", Weekdays: 127}}, Actions: []Action{{ActionID: "pcs", Value: 100}}},
	}}}
	graph := BuildDependencyGraph(doc)
	if len(graph.Nodes) != 3 {
		t.Fatalf("want 3 nodes (input, output, row), got %+v", graph.Nodes)
	}
	if len(graph.Edges) != 2 {
		t.Fatalf("want 2 edges (soc→row, row→pcs), got %+v", graph.Edges)
	}
	kinds := map[string]string{}
	for _, node := range graph.Nodes {
		kinds[node.ID] = node.Kind
	}
	if kinds["input:soc"] != "input" || kinds["output:pcs"] != "output" || kinds["row:r1"] != "row" {
		t.Fatalf("unexpected node kinds: %+v", graph.Nodes)
	}
}
