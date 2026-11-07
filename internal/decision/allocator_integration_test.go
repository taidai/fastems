package decision

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"
)

func TestAllocatorRunsSequentialReadback(t *testing.T) {
	opts := server.TestServerOptions
	opts.ID = "easyems-allocation-test"
	opts.DataDir = filepath.Join(t.TempDir(), "data")
	opts.NatsPort = 19700
	opts.NatsServer = "nats://127.0.0.1:19700"
	opts.HTTPPort = "9701"
	opts.NatsMonitorPort = 19702
	opts.NatsWSPort = 19703
	opts.NatsMQTTPort = 0
	nc, root, stopServer, err := server.TestServerOpts(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer stopServer()
	_, stopManager := runManager(t, nc)
	defer stopManager()

	createVariable(t, nc, root.ID, "grid", 50)
	group := data.NodeEdge{ID: "ess", Type: data.NodeTypeGroup, Parent: root.ID}
	if err := siotclient.SendNode(nc, group, "test"); err != nil {
		t.Fatal(err)
	}
	createVariable(t, nc, group.ID, "pcs1", 0)
	createVariable(t, nc, group.ID, "pcs2", 0)
	createVariable(t, nc, group.ID, "pcs3", 0)
	createVariable(t, nc, group.ID, "pcs4", 0)
	createVariable(t, nc, group.ID, "cap1", 200)
	createVariable(t, nc, group.ID, "cap2", 200)
	createVariable(t, nc, group.ID, "cap3", 150)
	createVariable(t, nc, group.ID, "cap4", 200)
	publish(t, nc, "pcs4", "health", "offline")

	config := AllocatorConfig{
		TimeoutMS: 500,
		Members: []AllocatorMember{
			allocatorTestMember("CAB01", "pcs1", "cap1", 200),
			allocatorTestMember("CAB02", "pcs2", "cap2", 200),
			allocatorTestMember("CAB03", "pcs3", "cap3", 150),
			allocatorTestMember("CAB04", "pcs4", "cap4", 200),
		},
	}
	publish(t, nc, "ess", allocatorPointType, marshal(t, config))

	document := Document{
		Table: Table{Rows: []Row{{
			ID:         "discharge",
			Conditions: []Condition{{PointID: "grid", Operator: "gt", Value: 0}},
			Actions:    []Action{{ActionID: "ess-power", Value: 800}},
		}}},
		Bindings: BindingSet{
			Points:  map[string]Target{"grid": {NodeID: "grid"}},
			Actions: map[string]Target{"ess-power": {NodeID: "ess"}},
		},
	}
	if err := siotclient.SendNodeType(nc, DecisionTable{
		ID: "ess-rule", Parent: root.ID, Description: "储能分配",
		Applied: marshal(t, document), Enabled: true,
	}, "test"); err != nil {
		t.Fatal(err)
	}

	waitForValue(t, nc, "pcs1", 200)
	if got := pointValue(t, nc, "pcs2", data.PointTypeValueSet); got != -1 {
		t.Fatalf("pcs2 was written before pcs1 readback: %v", got)
	}
	sendFloat(t, nc, "pcs1", data.PointTypeValue, 200)
	waitForValue(t, nc, "pcs2", 200)
	if got := pointValue(t, nc, "pcs3", data.PointTypeValueSet); got != -1 {
		t.Fatalf("pcs3 was written before pcs2 readback: %v", got)
	}
	sendFloat(t, nc, "pcs2", data.PointTypeValue, 200)
	waitForValue(t, nc, "pcs3", 150)
	sendFloat(t, nc, "pcs3", data.PointTypeValue, 150)
	if got := pointValue(t, nc, "pcs4", data.PointTypeValueSet); got != -1 {
		t.Fatalf("offline member was written: %v", got)
	}

	report := waitCheck(t, nc, "ess-rule", func(report CheckReport) bool {
		return len(report.Actions) == 1 && report.Actions[0].Status == "confirmed"
	})
	action := report.Actions[0]
	if len(action.Shares) != 4 {
		t.Fatalf("unexpected shares: %+v", action.Shares)
	}
	for i, want := range []float64{200, 200, 150} {
		if action.Shares[i].Setpoint != want || !action.Shares[i].Confirmed || action.Shares[i].Actual != want {
			t.Errorf("share %d not confirmed: %+v", i, action.Shares[i])
		}
	}
	if !action.Shares[3].Excluded || action.Shares[3].Reason != "offline" {
		t.Errorf("offline member not excluded: %+v", action.Shares[3])
	}

	// A new version requests 400 kW. pcs1 confirms, pcs2 never readbacks;
	// pcs3 must not receive its new target and the result becomes unknown.
	document.Table.Rows[0].Actions[0].Value = 400
	wantShare := 400.0 * 200.0 / 550.0
	publish(t, nc, "ess-rule", "applied", marshal(t, document))
	waitForValue(t, nc, "pcs1", wantShare)
	sendFloat(t, nc, "pcs1", data.PointTypeValue, wantShare)
	waitForValue(t, nc, "pcs2", wantShare)
	waitCheck(t, nc, "ess-rule", func(report CheckReport) bool {
		return len(report.Actions) == 1 && report.Actions[0].Status == "unknown"
	})
	if got := pointValue(t, nc, "pcs3", data.PointTypeValueSet); got != 150 {
		t.Fatalf("pcs3 advanced after pcs2 timeout: %v", got)
	}
}

func allocatorTestMember(id, target, capability string, max float64) AllocatorMember {
	return AllocatorMember{
		ID: id, Target: Target{NodeID: target},
		MaxDischarge: max, MaxCharge: max,
		AvailableDischarge: &Target{NodeID: capability},
		AvailableCharge:    &Target{NodeID: capability},
	}
}

func createVariable(t *testing.T, nc *nats.Conn, parent, id string, value float64) {
	t.Helper()
	if err := siotclient.SendNode(nc, data.NodeEdge{
		ID: id, Type: data.NodeTypeVariable, Parent: parent,
		Points: data.Points{data.NewPointFloat(data.PointTypeValue, "", value)},
	}, "test"); err != nil {
		t.Fatal(err)
	}
}

func waitCheck(t *testing.T, nc *nats.Conn, ruleID string, predicate func(CheckReport) bool) CheckReport {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	last := ""
	for time.Now().Before(deadline) {
		nodes, err := siotclient.GetNodes(nc, "all", ruleID, "", false)
		if err == nil && len(nodes) == 1 {
			point, ok := nodes[0].Points.Find("lastCheck", "")
			if ok {
				last = point.Txt()
				var report CheckReport
				if json.Unmarshal([]byte(last), &report) == nil && predicate(report) {
					return report
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("lastCheck did not reach expected state: %s", last)
	return CheckReport{}
}

func TestAllocatorRejectsSharedOwnership(t *testing.T) {
	opts := server.TestServerOptions
	opts.DataDir = filepath.Join(t.TempDir(), "data")
	opts.NatsPort = 19700
	opts.NatsServer = "nats://127.0.0.1:19700"
	opts.HTTPPort = "9701"
	opts.NatsMonitorPort = 19702
	opts.NatsWSPort = 19703
	opts.NatsMQTTPort = 0
	nc, root, stopServer, err := server.TestServerOpts(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer stopServer()
	createVariable(t, nc, root.ID, "pcs1", 0)
	if err := siotclient.SendNode(nc, data.NodeEdge{ID: "ess", Type: data.NodeTypeGroup, Parent: root.ID}, "test"); err != nil {
		t.Fatal(err)
	}
	publish(t, nc, "ess", allocatorPointType, marshal(t, AllocatorConfig{
		Members: []AllocatorMember{{ID: "CAB01", Target: Target{NodeID: "pcs1"}, MaxDischarge: 100, MaxCharge: 100}},
	}))
	document := Document{
		Table: Table{Rows: []Row{{
			ID:      "r",
			Actions: []Action{{ActionID: "a", Value: 10}},
		}}},
		Bindings: BindingSet{Actions: map[string]Target{"a": {NodeID: "ess"}}},
	}
	if err := siotclient.SendNodeType(nc, DecisionTable{ID: "one", Parent: root.ID, Applied: marshal(t, document), Enabled: true}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAllocationOwnership(nc, "two", "ess"); err == nil || !strings.Contains(err.Error(), "already owned") {
		t.Fatalf("expected ownership conflict, got %v", err)
	}
	if err := ValidateAllocationOwnership(nc, "one", "ess"); err != nil {
		t.Fatalf("owner rejected itself: %v", err)
	}
}
