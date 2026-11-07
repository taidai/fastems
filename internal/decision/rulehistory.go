package decision

import (
	"fmt"
	"sort"
	"strings"
)

// RowDiff is one row-level difference between two published documents.
type RowDiff struct {
	RowID string `json:"row_id"`
	// Kind is "added", "removed" or "changed".
	Kind string `json:"kind"`
	// From/To carry a one-line description of the removed/added row.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Detail lists field-level changes for a changed row.
	Detail []string `json:"detail,omitempty"`
}

// DiffDocuments compares two documents row by row, matching on row_id so a
// reordered table still reports content changes rather than spurious removes.
func DiffDocuments(a, b Document) []RowDiff {
	aRows := make(map[string]Row, len(a.Table.Rows))
	aOrder := make([]string, 0, len(a.Table.Rows))
	for _, row := range a.Table.Rows {
		aRows[row.ID] = row
		aOrder = append(aOrder, row.ID)
	}
	bRows := make(map[string]Row, len(b.Table.Rows))
	bOrder := make([]string, 0, len(b.Table.Rows))
	for _, row := range b.Table.Rows {
		bRows[row.ID] = row
		bOrder = append(bOrder, row.ID)
	}

	diffs := make([]RowDiff, 0)
	for _, id := range aOrder {
		if _, ok := bRows[id]; !ok {
			diffs = append(diffs, RowDiff{RowID: id, Kind: "removed", From: describeRow(aRows[id])})
		}
	}
	for _, id := range bOrder {
		if _, ok := aRows[id]; !ok {
			diffs = append(diffs, RowDiff{RowID: id, Kind: "added", To: describeRow(bRows[id])})
		}
	}
	for _, id := range aOrder {
		before, aok := aRows[id]
		after, bok := bRows[id]
		if !aok || !bok {
			continue
		}
		if detail := diffRow(before, after); len(detail) > 0 {
			diffs = append(diffs, RowDiff{RowID: id, Kind: "changed", Detail: detail})
		}
	}
	return diffs
}

func describeRow(row Row) string {
	if len(row.Conditions) == 0 {
		return "默认行"
	}
	parts := make([]string, 0, len(row.Conditions))
	for _, condition := range row.Conditions {
		parts = append(parts, conditionText(condition))
	}
	summary := strings.Join(parts, " 且 ")
	if len(row.Actions) > 0 {
		actions := make([]string, 0, len(row.Actions))
		for _, action := range row.Actions {
			actions = append(actions, actionText(action))
		}
		summary += " → " + strings.Join(actions, "，")
	}
	return summary
}

func conditionText(c Condition) string {
	if c.Kind == "time" {
		return fmt.Sprintf("时间 %s %s-%s", weekdayText(c.Weekdays), c.Start, c.End)
	}
	value := fmt.Sprintf("%g", c.Value)
	if c.Kind == "bool" {
		if c.Value != 0 {
			value = "是"
		} else {
			value = "否"
		}
	} else if c.Kind == "enum" {
		value = c.Text
	}
	return fmt.Sprintf("%s %s %s", c.PointID, c.Operator, value)
}

func actionText(a Action) string {
	return fmt.Sprintf("%s = %g", a.ActionID, a.Value)
}

func diffRow(a, b Row) []string {
	detail := make([]string, 0)
	aCond := make([]string, 0, len(a.Conditions))
	bCond := make([]string, 0, len(b.Conditions))
	for _, c := range a.Conditions {
		aCond = append(aCond, conditionText(c))
	}
	for _, c := range b.Conditions {
		bCond = append(bCond, conditionText(c))
	}
	if strings.Join(aCond, " 且 ") != strings.Join(bCond, " 且 ") {
		detail = append(detail, "条件："+strings.Join(aCond, " 且 ")+" → "+strings.Join(bCond, " 且 "))
	}
	aAct := make([]string, 0, len(a.Actions))
	bAct := make([]string, 0, len(b.Actions))
	for _, act := range a.Actions {
		aAct = append(aAct, actionText(act))
	}
	for _, act := range b.Actions {
		bAct = append(bAct, actionText(act))
	}
	if strings.Join(aAct, "，") != strings.Join(bAct, "，") {
		detail = append(detail, "输出："+strings.Join(aAct, "，")+" → "+strings.Join(bAct, "，"))
	}
	if a.HoldMS != b.HoldMS {
		detail = append(detail, fmt.Sprintf("持续确认：%d ms → %d ms", a.HoldMS, b.HoldMS))
	}
	if a.CooldownMS != b.CooldownMS {
		detail = append(detail, fmt.Sprintf("冷却：%d ms → %d ms", a.CooldownMS, b.CooldownMS))
	}
	return detail
}

// DependencyNode is one node in the rule dependency graph.
type DependencyNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // "input", "output" or "row"
	Label string `json:"label"`
}

// DependencyEdge connects a row to an input it reads or an output it writes.
type DependencyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // "condition" or "action"
}

// DependencyGraph is the set of nodes and edges a document references.
type DependencyGraph struct {
	Nodes []DependencyNode `json:"nodes"`
	Edges []DependencyEdge `json:"edges"`
}

// DependencyGraph builds the input/output/row reference graph of a document so
// the editor can show which points a rule reads and writes, and how rows chain.
func BuildDependencyGraph(d Document) DependencyGraph {
	graph := DependencyGraph{}
	rowIDs := make(map[string]bool)
	inputIDs := make(map[string]bool)
	outputIDs := make(map[string]bool)
	for _, row := range d.Table.Rows {
		rowIDs[row.ID] = true
		for _, condition := range row.Conditions {
			if condition.Kind == "time" {
				continue
			}
			inputIDs[condition.PointID] = true
		}
		for _, action := range row.Actions {
			outputIDs[action.ActionID] = true
		}
	}
	for id := range inputIDs {
		graph.Nodes = append(graph.Nodes, DependencyNode{ID: "input:" + id, Kind: "input", Label: id})
	}
	for id := range outputIDs {
		graph.Nodes = append(graph.Nodes, DependencyNode{ID: "output:" + id, Kind: "output", Label: id})
	}
	for _, row := range d.Table.Rows {
		graph.Nodes = append(graph.Nodes, DependencyNode{ID: "row:" + row.ID, Kind: "row", Label: row.ID})
		for _, condition := range row.Conditions {
			if condition.Kind == "time" {
				continue
			}
			graph.Edges = append(graph.Edges, DependencyEdge{From: "input:" + condition.PointID, To: "row:" + row.ID, Kind: "condition"})
		}
		for _, action := range row.Actions {
			graph.Edges = append(graph.Edges, DependencyEdge{From: "row:" + row.ID, To: "output:" + action.ActionID, Kind: "action"})
		}
	}
	sort.Slice(graph.Nodes, func(i, j int) bool { return graph.Nodes[i].ID < graph.Nodes[j].ID })
	return graph
}

func weekdayText(mask int) string {
	if mask == 0 || mask == 127 {
		return "每天"
	}
	labels := []string{"日", "一", "二", "三", "四", "五", "六"}
	var out []string
	for bit := 0; bit <= 6; bit++ {
		if mask&(1<<bit) != 0 {
			out = append(out, labels[bit])
		}
	}
	return "周" + strings.Join(out, "、")
}
