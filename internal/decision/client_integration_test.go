package decision

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"
)

func TestDecisionTableOnSimpleIoT(t *testing.T) {
	opts := server.TestServerOptions
	opts.ID = "easyems-decision-test"
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
	_, stopManager := runManager(t, nc)

	for _, variable := range []struct {
		id    string
		value float64
	}{
		{"temperature", 35}, {"occupied", 1}, {"fan", -1}, {"damper", -1},
	} {
		err := siotclient.SendNode(nc, data.NodeEdge{
			ID: variable.id, Type: data.NodeTypeVariable, Parent: root.ID,
			Points: data.Points{
				data.NewPointString(data.PointTypeDescription, "", variable.id),
				data.NewPointFloat(data.PointTypeValue, "", variable.value),
			},
		}, "test")
		if err != nil {
			t.Fatal(err)
		}
	}

	table := Table{Rows: []Row{
		{
			ID: "hot-and-occupied",
			Conditions: []Condition{
				{PointID: "room-temperature", Operator: "gt", Value: 30},
				{PointID: "room-occupied", Operator: "eq", Value: 1},
			},
			Actions: []Action{{ActionID: "start-fan", Value: 1}, {ActionID: "open-damper", Value: 80}},
		},
		{
			ID:         "hot-fallback",
			Conditions: []Condition{{PointID: "room-temperature", Operator: "gt", Value: 30}},
			Actions:    []Action{{ActionID: "start-fan", Value: 0}},
		},
		{ID: "default"},
	}}
	document := Document{Table: table, Bindings: BindingSet{
		Points: map[string]Target{
			"room-temperature": {NodeID: "temperature"},
			"room-occupied":    {NodeID: "occupied"},
		},
		Actions: map[string]Target{
			"start-fan":   {NodeID: "fan"},
			"open-damper": {NodeID: "damper"},
		},
	}}
	encodedDocument := marshal(t, document)

	// A saved draft does not execute, even when the table is enabled.
	err = siotclient.SendNodeType(nc, DecisionTable{
		ID: "comfort-policy", Parent: root.ID, Description: "Comfort",
		Draft: encodedDocument, Enabled: true,
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := pointValue(t, nc, "fan", data.PointTypeValueSet); got != -1 {
		t.Fatalf("draft executed: fan valueSet = %v", got)
	}

	publish(t, nc, "comfort-policy", "applied", encodedDocument)
	waitForValue(t, nc, "fan", 1)
	waitForValue(t, nc, "damper", 80)

	// Both rows match. Reordering changes the winner and the second row must
	// not overwrite the first row's two outputs.
	table.Rows[0], table.Rows[1] = table.Rows[1], table.Rows[0]
	document.Table = table
	publish(t, nc, "comfort-policy", "applied", marshal(t, document))
	waitForValue(t, nc, "fan", 0)
	if got := pointValue(t, nc, "damper", data.PointTypeValueSet); got != 80 {
		t.Fatalf("second row changed damper: %v", got)
	}

	stopManager()
	stopServer()

	nc, _, stopServer, err = server.TestServerOptsKeepStore(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer stopServer()

	outputWrites := make(chan data.Point, 8)
	stopSubscription, err := siotclient.SubscribePoints(nc, "fan", func(points []data.Point) {
		for _, point := range points {
			if point.Type == data.PointTypeValueSet && point.Origin == "comfort-policy" {
				outputWrites <- point
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stopSubscription()
	_, stopManager = runManager(t, nc)
	defer stopManager()

	if got := pointValue(t, nc, "fan", data.PointTypeValueSet); got != 0 {
		t.Fatalf("fan value not restored: %v", got)
	}
	tables, err := siotclient.GetNodesType[DecisionTable](nc, "all", "comfort-policy")
	if err != nil || len(tables) != 1 || !tables[0].Enabled || tables[0].LastMatch != "hot-fallback" {
		t.Fatalf("published table not restored: %#v, %v", tables, err)
	}
	select {
	case point := <-outputWrites:
		value, _ := point.ValueFloat()
		t.Fatalf("restart replayed output value %v", value)
	case <-time.After(300 * time.Millisecond):
	}

	sendFloat(t, nc, "fan", "easyemsManual", 1)
	sendFloat(t, nc, "fan", data.PointTypeValueSet, 1)
	sendFloat(t, nc, "temperature", data.PointTypeValue, 20)
	waitForMatch(t, nc, "default")
	sendFloat(t, nc, "temperature", data.PointTypeValue, 35)
	select {
	case point := <-outputWrites:
		t.Fatalf("manual target received automatic output: %v", point)
	case <-time.After(200 * time.Millisecond):
	}
	if got := pointValue(t, nc, "fan", data.PointTypeValueSet); got != 1 {
		t.Fatalf("manual target was overridden: %v", got)
	}
	sendFloat(t, nc, "fan", "easyemsManual", 0)
	waitForValue(t, nc, "fan", 0)

	if err := siotclient.SendNode(nc, data.NodeEdge{
		ID: "replacement-fan", Type: data.NodeTypeVariable, Parent: root.ID,
		Points: data.Points{data.NewPointFloat(data.PointTypeValue, "", -1)},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	sendFloat(t, nc, "fan", data.PointTypeValueSet, 1)
	document.Bindings.Actions["start-fan"] = Target{NodeID: "replacement-fan", PointType: "valueSet"}
	publish(t, nc, "comfort-policy", "applied", marshal(t, document))
	waitForValue(t, nc, "replacement-fan", 0)
	if got := pointValue(t, nc, "fan", data.PointTypeValueSet); got != 1 {
		t.Fatalf("old brand source was written after rebinding: %v", got)
	}
}

func runManager(t *testing.T, nc *nats.Conn) (*siotclient.Manager[DecisionTable], func()) {
	t.Helper()
	manager := siotclient.NewManager(nc, NewDecisionTableClient, nil)
	done := make(chan error, 1)
	go func() { done <- manager.Run() }()
	return manager, func() {
		manager.Stop(nil)
		if err := <-done; err != nil {
			t.Errorf("decision manager: %v", err)
		}
	}
}

func publish(t *testing.T, nc *nats.Conn, nodeID, pointType, value string) {
	t.Helper()
	point := data.NewPointString(pointType, "", value)
	point.Origin = "test"
	if err := siotclient.SendNodePoint(nc, nodeID, point, true); err != nil {
		t.Fatal(err)
	}
}

func sendFloat(t *testing.T, nc *nats.Conn, nodeID, pointType string, value float64) {
	t.Helper()
	point := data.NewPointFloat(pointType, "", value)
	point.Origin = "test"
	if err := siotclient.SendNodePoint(nc, nodeID, point, true); err != nil {
		t.Fatal(err)
	}
}

func waitForMatch(t *testing.T, nc *nats.Conn, want string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		tables, err := siotclient.GetNodesType[DecisionTable](nc, "all", "comfort-policy")
		if err == nil && len(tables) == 1 && tables[0].LastMatch == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("lastMatch did not become %s", want)
}

func pointValue(t *testing.T, nc *nats.Conn, nodeID, pointType string) float64 {
	t.Helper()
	nodes, err := siotclient.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		t.Fatalf("get %s: %v", nodeID, err)
	}
	point, ok := nodes[0].Points.Find(pointType, "")
	if !ok {
		return -1
	}
	value, err := point.ValueFloat()
	if err != nil {
		t.Fatalf("decode %s.%s: %v", nodeID, pointType, err)
	}
	return value
}

func waitForValue(t *testing.T, nc *nats.Conn, nodeID string, want float64) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if pointValue(t, nc, nodeID, data.PointTypeValueSet) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s valueSet = %v, want %v", nodeID, pointValue(t, nc, nodeID, data.PointTypeValueSet), want)
}

func marshal(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
