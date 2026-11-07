//go:build linux

package decision

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	siotmodbus "github.com/simpleiot/simpleiot/modbus"
	"github.com/simpleiot/simpleiot/respreader"
	"github.com/simpleiot/simpleiot/server"
)

func TestDecisionTableOverModbusRTU(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	slaveName := slave.Name()
	_ = slave.Close()

	regs := &siotmodbus.Regs{}
	regs.AddReg(0, 1)
	regs.AddReg(10, 1)
	if err := regs.WriteReg(10, 253); err != nil {
		t.Fatal(err)
	}
	reader := respreader.NewReadWriteCloser(master, 500*time.Millisecond, 20*time.Millisecond)
	rtuServer := siotmodbus.NewServer(1, siotmodbus.NewRTU(reader), regs, 0)
	go rtuServer.Listen(func(error) {}, func() {}, func() {})
	var stopOnce sync.Once
	stopRTU := func() { stopOnce.Do(func() { _ = rtuServer.Close() }) }
	defer stopRTU()

	opts := server.TestServerOptions
	opts.ID = "easyems-rtu-test"
	opts.DataDir = filepath.Join(t.TempDir(), "data")
	opts.NatsPort = 9740
	opts.NatsServer = "nats://127.0.0.1:9740"
	opts.HTTPPort = "9741"
	opts.NatsMonitorPort = 9742
	opts.NatsWSPort = 9743
	opts.NatsMQTTPort = 0
	nc, root, stopServer, err := server.TestServerOpts(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer stopServer()
	_, stopManager := runManager(t, nc)
	defer stopManager()

	if err := siotclient.SendNodeType(nc, siotclient.Modbus{
		ID: "rtu-bus", Parent: root.ID, Description: "RTU bus",
		ClientServer: data.PointValueClient, Protocol: data.PointValueRTU,
		Port: slaveName, Baud: "9600", PollPeriod: 50, Timeout: 500,
	}, "test"); err != nil {
		t.Fatal(err)
	}
	for _, io := range []siotclient.ModbusIo{
		{ID: "rtu-temperature", Parent: "rtu-bus", Description: "temperature", ServerID: 1, Address: 10, ModbusIOType: data.PointValueModbusHoldingRegister, DataFormat: data.PointValueUINT16, ReadOnly: true, Scale: 0.1},
		{ID: "rtu-coil", Parent: "rtu-bus", Description: "coil", ServerID: 1, Address: 0, ModbusIOType: data.PointValueModbusCoil, ReadOnly: false, Scale: 1},
	} {
		node, err := data.Encode(io)
		if err != nil {
			t.Fatal(err)
		}
		node.Points = append(node.Points, data.NewPointString("businessId", "", io.ID+"-business"))
		if err := siotclient.SendNode(nc, node, "test"); err != nil {
			t.Fatal(err)
		}
	}
	waitForRTUValue(t, nc, "rtu-temperature", 25.3)

	document := Document{Table: Table{Rows: []Row{{
		ID:         "rtu-hot",
		Conditions: []Condition{{PointID: "temperature-business", Operator: "gt", Value: 30}},
		Actions:    []Action{{ActionID: "coil-business", Value: 1}},
	}}}, Bindings: BindingSet{
		Points:  map[string]Target{"temperature-business": {NodeID: "rtu-temperature", PointType: "value"}},
		Actions: map[string]Target{"coil-business": {NodeID: "rtu-coil", PointType: "valueSet"}},
	}}
	if err := siotclient.SendNodeType(nc, DecisionTable{
		ID: "rtu-policy", Parent: root.ID, Description: "RTU policy",
		Draft: marshal(t, document), Applied: marshal(t, document), Enabled: true,
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := regs.WriteReg(10, 351); err != nil {
		t.Fatal(err)
	}
	waitForRTUValue(t, nc, "rtu-coil", 1)
	coil, err := regs.ReadCoil(0)
	if err != nil || !coil {
		t.Fatalf("RTU coil was not actuated: %v %v", coil, err)
	}

	for _, variable := range []string{"rtu-enable", "rtu-alarm"} {
		if err := siotclient.SendNode(nc, data.NodeEdge{
			ID: variable, Type: data.NodeTypeVariable, Parent: root.ID,
			Points: data.Points{data.NewPointFloat(data.PointTypeValue, "", 0)},
		}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	document.Table.Rows[0].Conditions = append(document.Table.Rows[0].Conditions,
		Condition{PointID: "enable-business", Operator: "eq", Value: 1})
	document.Table.Rows[0].Actions = []Action{{ActionID: "alarm-business", Value: 1}}
	document.Bindings.Points["enable-business"] = Target{NodeID: "rtu-enable", PointType: "value"}
	document.Bindings.Actions = map[string]Target{"alarm-business": {NodeID: "rtu-alarm", PointType: "valueSet"}}
	publish(t, nc, "rtu-policy", "applied", marshal(t, document))
	stopRTU()

	deadline := time.Now().Add(12 * time.Second)
	for {
		buses, err := siotclient.GetNodes(nc, "all", "rtu-bus", "", false)
		if err == nil && len(buses) > 0 {
			heartbeat, ok := buses[0].Points.Find("lastGoodPoll", "")
			if ok && time.Since(heartbeat.Time) > 5*time.Second {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("Modbus heartbeat did not become stale")
		}
		time.Sleep(50 * time.Millisecond)
	}
	sendFloat(t, nc, "rtu-enable", data.PointTypeValue, 1)
	deadline = time.Now().Add(5 * time.Second)
	for {
		tables, err := siotclient.GetNodesType[DecisionTable](nc, "all", "rtu-policy")
		if err == nil && len(tables) == 1 && strings.Contains(tables[0].Error, "stale") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale Modbus input was not reported")
		}
		time.Sleep(30 * time.Millisecond)
	}
	if got := pointValue(t, nc, "rtu-alarm", data.PointTypeValueSet); got == 1 {
		t.Fatal("stale Modbus input caused an output")
	}
}

func waitForRTUValue(t *testing.T, nc *nats.Conn, id string, want float64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		nodes, err := siotclient.GetNodes(nc, "all", id, "", false)
		if err == nil && len(nodes) > 0 {
			if point, ok := nodes[0].Points.Find(data.PointTypeValue, ""); ok {
				value, _ := point.ValueFloat()
				if value == want {
					return
				}
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s = %v", id, want)
}
