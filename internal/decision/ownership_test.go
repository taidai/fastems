package decision

import "testing"

func TestOutputTargetsNormalizeAndDeduplicate(t *testing.T) {
	document := Document{
		Table: Table{Rows: []Row{{Actions: []Action{
			{ActionID: "a"}, {ActionID: "b"}, {ActionID: "c"}, {ActionID: "missing"},
		}}}},
		Bindings: BindingSet{Actions: map[string]Target{
			"a": {NodeID: "pcs-1", PointType: "valueSet"},
			"b": {NodeID: "pcs-1", PointType: "value"},
			"c": {NodeID: "pcs-1", PointType: "valueSet"},
		}},
	}
	targets := outputTargets(document)
	if len(targets) != 2 {
		t.Fatalf("expected two distinct targets, got %+v", targets)
	}
	if targets[0] != (outputTarget{nodeID: "pcs-1", pointType: "valueSet"}) {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
	if targets[1] != (outputTarget{nodeID: "pcs-1", pointType: "value"}) {
		t.Fatalf("unexpected second target: %+v", targets[1])
	}
}

func TestOutputTargetsEmptyMeansNoOwnership(t *testing.T) {
	document := Document{
		Table:    Table{Rows: []Row{{Actions: []Action{{ActionID: "a"}}}}},
		Bindings: BindingSet{Actions: map[string]Target{"a": {}}},
	}
	if got := outputTargets(document); len(got) != 0 {
		t.Fatalf("unbound target should not own anything, got %+v", got)
	}
}
