package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	siotmodbus "github.com/simpleiot/simpleiot/modbus"
	"github.com/simpleiot/simpleiot/server"

	"easyems/internal/decision"
)

func TestProductAPIVerticalSlice(t *testing.T) {
	const password = "easyems-test-only"
	t.Setenv("EASYEMS_ADMIN_PASSWORD", password)
	dataDir := filepath.Join(t.TempDir(), "data")
	ready := new(atomic.Bool)
	options := server.Options{
		ID: "easyems-api-test", DataDir: dataDir, HTTPPort: "9711",
		NatsPort: 9710, NatsServer: "nats://127.0.0.1:9710", AuthToken: data.NewID(),
		DeviceAuth: server.DeviceAuthRequired, CustomUIFS: fstest.MapFS{"index.html": {Data: []byte("easyems")}},
		AppOnly: true, NatsLoopback: true, ProvisioningDisable: true,
		AppHandlerFactory: func(nc *nats.Conn, users api.UserAuthority) http.Handler {
			return New(nc, users, ready)
		},
	}
	siot, nc, err := server.NewServer(options)
	if err != nil {
		t.Fatal(err)
	}
	clients := client.NewRunGroup("test clients")
	clients.Add(client.NewManager(nc, client.NewModbusClient, nil))
	clients.Add(client.NewManager(nc, decision.NewDecisionTableClient, nil))
	siot.AddClient(clients)
	done := make(chan error, 1)
	go func() { done <- siot.Run() }()
	defer func() {
		siot.Stop(nil)
		<-done
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := siot.WaitStart(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(nc, dataDir); err != nil {
		t.Fatal(err)
	}
	ready.Store(true)
	base := "http://127.0.0.1:9711/api"
	waitFor(t, func() bool {
		response, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, "HTTP readiness")

	status, _ := call(t, http.MethodPost, base+"/login", "", map[string]string{"email": "admin", "password": "admin"}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("default password still works: %d", status)
	}
	token := loginAPI(t, base, "admin", password)

	status, _ = call(t, http.MethodGet, base+"/snapshot", "", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous snapshot: %d", status)
	}
	status, _ = call(t, http.MethodGet, "http://127.0.0.1:9711/v1/nodes", token, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("generic SIOT API is exposed: %d", status)
	}
	status, _ = call(t, http.MethodGet, "http://127.0.0.1:9711/old", token, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("engineering UI is exposed: %d", status)
	}

	groupID := createNodeAPI(t, base, token, "group", snapshotRoot(t, base, token), "A 站", 0)
	for _, item := range []struct {
		name  string
		value float64
	}{
		{"温度", 35}, {"有人", 1}, {"风机", -1}, {"风阀", -1},
	} {
		createNodeAPI(t, base, token, "variable", groupID, item.name, item.value)
	}
	nodes := snapshotNodes(t, base, token)
	byName := map[string]data.NodeEdge{}
	for _, node := range nodes {
		description, _ := node.Points.Text(data.PointTypeDescription, "")
		byName[description] = node
	}
	businessID := func(name string) string {
		node := byName[name]
		value, _ := node.Points.Text("businessId", "")
		return value
	}
	document := decision.Document{Table: decision.Table{Rows: []decision.Row{{
		ID: "hot-occupied",
		Conditions: []decision.Condition{
			{PointID: businessID("温度"), Operator: "gt", Value: 30},
			{PointID: businessID("有人"), Operator: "eq", Value: 1},
		},
		Actions: []decision.Action{
			{ActionID: businessID("风机"), Value: 1},
			{ActionID: businessID("风阀"), Value: 80},
		},
	}}}, Bindings: decision.BindingSet{
		Points: map[string]decision.Target{
			businessID("温度"): {NodeID: byName["温度"].ID, PointType: "value"},
			businessID("有人"): {NodeID: byName["有人"].ID, PointType: "value"},
		},
		Actions: map[string]decision.Target{
			businessID("风机"): {NodeID: byName["风机"].ID, PointType: "valueSet"},
			businessID("风阀"): {NodeID: byName["风阀"].ID, PointType: "valueSet"},
		},
	}}
	var created struct {
		ID string `json:"id"`
	}
	status, _ = call(t, http.MethodPost, base+"/rules", token, map[string]any{
		"parent": groupID, "description": "舒适控制", "document": document,
	}, &created)
	if status != http.StatusOK {
		t.Fatalf("create rule: %d", status)
	}
	var evaluation decision.Result
	status, _ = call(t, http.MethodPost, base+"/rules/"+created.ID+"/test", token, map[string]any{
		"document": document, "inputs": map[string]float64{businessID("温度"): 35, businessID("有人"): 1},
	}, &evaluation)
	if status != http.StatusOK || evaluation.RowID != "hot-occupied" || len(evaluation.Actions) != 2 {
		t.Fatalf("simulation: %d %#v", status, evaluation)
	}
	if value := valueOf(t, nc, byName["风机"].ID); value != 0 {
		t.Fatalf("simulation wrote an output: %v", value)
	}
	outsideID := createNodeAPI(t, base, token, "variable", snapshotRoot(t, base, token), "站外风机", 0)
	fanID := businessID("风机")
	fanTarget := document.Bindings.Actions[fanID]
	document.Bindings.Actions[fanID] = decision.Target{NodeID: outsideID, PointType: "valueSet"}
	status, _ = call(t, http.MethodPost, base+"/rules/"+created.ID+"/publish", token, document, nil)
	if status != http.StatusForbidden {
		t.Fatalf("out-of-scope automatic output was accepted: %d", status)
	}
	document.Bindings.Actions[fanID] = fanTarget
	for _, step := range []struct {
		path, method string
		body         any
	}{
		{"/rules/" + created.ID + "/publish", http.MethodPost, document},
		{"/rules/" + created.ID + "/enable", http.MethodPost, map[string]bool{"enabled": true}},
	} {
		status, _ = call(t, step.method, base+step.path, token, step.body, nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d", step.path, status)
		}
	}
	waitFor(t, func() bool {
		return valueOf(t, nc, byName["风机"].ID) == 1 && valueOf(t, nc, byName["风阀"].ID) == 80
	}, "ordered outputs")
	var history struct {
		Points []data.Point `json:"points"`
	}
	status, _ = call(t, http.MethodGet, base+"/history?node_id="+byName["风机"].ID+"&type=valueSet&limit=10", token, nil, &history)
	if status != http.StatusOK || len(history.Points) == 0 {
		t.Fatalf("native point history unavailable: %d", status)
	}
	var historyWG sync.WaitGroup
	historyErrs := make(chan string, 12)
	for range 12 {
		historyWG.Add(1)
		go func() {
			defer historyWG.Done()
			request, err := http.NewRequest(http.MethodGet, base+"/history?node_id="+byName["风机"].ID+"&type=valueSet&limit=10", nil)
			if err != nil {
				historyErrs <- err.Error()
				return
			}
			request.Header.Set("Authorization", "Bearer "+token)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				historyErrs <- err.Error()
				return
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				historyErrs <- fmt.Sprintf("concurrent history: %d", response.StatusCode)
			}
		}()
	}
	historyWG.Wait()
	close(historyErrs)
	for err := range historyErrs {
		t.Error(err)
	}
	status, exportedHistory := call(t, http.MethodGet, base+"/history/export?node_id="+byName["风机"].ID+"&type=valueSet", token, nil, nil)
	if status != http.StatusOK || !strings.Contains(exportedHistory, "时间") {
		t.Fatal("history CSV export failed")
	}

	status, _ = call(t, http.MethodPost, base+"/users", token, map[string]any{
		"parent": groupID, "email": "viewer@example.com", "password": "viewer-test-only", "role": "viewer",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create viewer: %d", status)
	}
	viewerToken := ""
	waitFor(t, func() bool {
		viewerToken = loginAPIOptional(base, "viewer@example.com", "viewer-test-only")
		return viewerToken != ""
	}, "viewer login")
	var viewerSnapshot struct {
		Nodes []data.NodeEdge `json:"nodes"`
	}
	status, _ = call(t, http.MethodGet, base+"/snapshot", viewerToken, nil, &viewerSnapshot)
	if status != http.StatusOK || len(viewerSnapshot.Nodes) == 0 {
		t.Fatalf("viewer snapshot: %d", status)
	}
	for _, node := range viewerSnapshot.Nodes {
		if node.Parent == snapshotRoot(t, base, token) && node.ID != groupID {
			t.Fatalf("viewer can see unrelated root node %s", node.ID)
		}
	}
	status, _ = call(t, http.MethodGet, base+"/history?node_id="+byName["风机"].ID+"&type=valueSet", viewerToken, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("scoped viewer cannot read point history: %d", status)
	}
	status, _ = call(t, http.MethodGet, base+"/history?node_id="+byName["风机"].ID+"&type=pass", viewerToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("sensitive point history was allowed: %d", status)
	}
	status, _ = call(t, http.MethodPost, base+"/nodes/"+byName["风机"].ID+"/value", viewerToken, map[string]any{"value": 0, "point_type": "valueSet"}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("viewer wrote a device: %d", status)
	}
	status, _ = call(t, http.MethodPost, base+"/rules/"+created.ID+"/publish", viewerToken, document, nil)
	if status != http.StatusForbidden {
		t.Fatalf("viewer published a rule: %d", status)
	}
	testRuleManagement(t, base, token, groupID, created.ID, document, byName["风机"], byName["风阀"], nc)
	testResourceManagement(t, base, token, viewerToken, groupID, byName, created.ID, outsideID, nc)
	testModbusTCP(t, base, token, groupID, nc)
}

func testRuleManagement(t *testing.T, base, token, groupID, ruleID string, document decision.Document, fan, damper data.NodeEdge, nc *nats.Conn) {
	t.Helper()
	var copied struct {
		ID string `json:"id"`
	}
	status, body := call(t, http.MethodPost, base+"/rules/"+ruleID+"/copy", token, map[string]string{"description": "控制副本"}, &copied)
	if status != http.StatusOK || copied.ID == "" {
		t.Fatalf("copy rule: %d %s", status, body)
	}
	copies, err := client.GetNodesType[decision.DecisionTable](nc, "all", copied.ID)
	if err != nil || len(copies) != 1 || copies[0].Enabled || copies[0].Applied != "" || copies[0].Draft == "" {
		t.Fatalf("copy unexpectedly active: %#v %v", copies, err)
	}
	status, _ = call(t, http.MethodPatch, base+"/nodes/"+ruleID, token, map[string]string{"description": "舒适控制（已验收）"}, nil)
	if status != http.StatusOK {
		t.Fatal("rename rule failed")
	}
	fanID, _ := fan.Points.Text("businessId", "")
	damperID, _ := damper.Points.Text("businessId", "")
	for i := range document.Table.Rows[0].Actions {
		switch document.Table.Rows[0].Actions[i].ActionID {
		case fanID:
			document.Table.Rows[0].Actions[i].Value = 0
		case damperID:
			document.Table.Rows[0].Actions[i].Value = 20
		}
	}
	status, _ = call(t, http.MethodPost, base+"/rules/"+ruleID+"/publish", token, document, nil)
	if status != http.StatusOK {
		t.Fatal("publish changed rule failed")
	}
	waitFor(t, func() bool { return valueOf(t, nc, fan.ID) == 0 && valueOf(t, nc, damper.ID) == 20 }, "changed outputs")
	nodes, _ := client.GetNodes(nc, "all", ruleID, "", false)
	var version RuleVersion
	found := false
	for _, point := range nodes[0].Points {
		if point.Type == "history" && point.Tombstone%2 == 0 && json.Unmarshal([]byte(point.Txt()), &version) == nil {
			found = true
			break
		}
	}
	if !found || version.ID == "" {
		t.Fatal("publication history not saved")
	}
	status, _ = call(t, http.MethodPost, base+"/rules/"+ruleID+"/restore", token, map[string]string{"version_id": version.ID}, nil)
	if status != http.StatusOK {
		t.Fatal("restore rule version failed")
	}
	waitFor(t, func() bool { return valueOf(t, nc, fan.ID) == 1 && valueOf(t, nc, damper.ID) == 80 }, "restored outputs")
	status, _ = call(t, http.MethodDelete, base+"/nodes/"+copied.ID, token, map[string]string{"parent": groupID}, nil)
	if status != http.StatusOK {
		t.Fatal("delete copied rule failed")
	}
}

func testResourceManagement(t *testing.T, base, token, viewerToken, groupID string, byName map[string]data.NodeEdge, ruleID, outsideID string, nc *nats.Conn) {
	t.Helper()
	status, body := call(t, http.MethodPatch, base+"/nodes/"+groupID, token, map[string]any{"description": "A 站（已核名）"}, nil)
	if status != http.StatusOK {
		t.Fatalf("rename group: %d %s", status, body)
	}
	status, _ = call(t, http.MethodPost, base+"/nodes/"+groupID+"/move", token, map[string]string{"old_parent": snapshotRoot(t, base, token), "new_parent": groupID}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("cyclic move accepted: %d", status)
	}
	status, _ = call(t, http.MethodPost, base+"/nodes/"+byName["温度"].ID+"/move", token, map[string]string{"old_parent": groupID, "new_parent": snapshotRoot(t, base, token)}, nil)
	if status != http.StatusConflict {
		t.Fatalf("rule-breaking point move accepted: %d", status)
	}
	status, _ = call(t, http.MethodDelete, base+"/nodes/"+byName["温度"].ID, token, map[string]string{"parent": groupID}, nil)
	if status != http.StatusConflict {
		t.Fatalf("referenced point delete accepted: %d", status)
	}
	status, _ = call(t, http.MethodDelete, base+"/nodes/"+groupID, token, map[string]string{"parent": snapshotRoot(t, base, token)}, nil)
	if status != http.StatusConflict {
		t.Fatalf("nonempty group delete accepted: %d", status)
	}
	otherGroupID := createNodeAPI(t, base, token, "group", snapshotRoot(t, base, token), "独立分组", 0)
	status, _ = call(t, http.MethodPost, base+"/nodes/"+groupID+"/move", token, map[string]string{"old_parent": snapshotRoot(t, base, token), "new_parent": otherGroupID}, nil)
	if status != http.StatusOK {
		t.Fatal("valid group move was rejected")
	}
	status, _ = call(t, http.MethodPost, base+"/nodes/"+groupID+"/move", token, map[string]string{"old_parent": otherGroupID, "new_parent": snapshotRoot(t, base, token)}, nil)
	if status != http.StatusOK {
		t.Fatal("valid group move back was rejected")
	}
	status, _ = call(t, http.MethodPost, base+"/nodes/"+ruleID+"/move", token, map[string]string{"old_parent": groupID, "new_parent": otherGroupID}, nil)
	if status != http.StatusConflict {
		t.Fatalf("rule moved outside target scope: %d", status)
	}
	status, _ = call(t, http.MethodPatch, base+"/nodes/"+byName["温度"].ID, token, map[string]any{"description": "温度", "disabled": true}, nil)
	if status != http.StatusOK {
		t.Fatal("disable input failed")
	}
	waitFor(t, func() bool {
		tables, _ := client.GetNodesType[decision.DecisionTable](nc, "all", ruleID)
		return len(tables) == 1 && strings.Contains(tables[0].Error, "disabled")
	}, "disabled input error")
	status, _ = call(t, http.MethodPatch, base+"/nodes/"+byName["温度"].ID, token, map[string]any{"description": "温度", "disabled": false}, nil)
	if status != http.StatusOK {
		t.Fatal("enable input failed")
	}
	waitFor(t, func() bool {
		tables, _ := client.GetNodesType[decision.DecisionTable](nc, "all", ruleID)
		return len(tables) == 1 && tables[0].Error == ""
	}, "input recovery")
	var firstCommand, secondCommand CommandResult
	for _, result := range []*CommandResult{&firstCommand, &secondCommand} {
		status, _ = call(t, http.MethodPost, base+"/nodes/"+byName["风机"].ID+"/value", token, map[string]any{"value": 2, "point_type": "valueSet", "request_id": "repeat-command-1"}, result)
		if status != http.StatusOK {
			t.Fatal("repeat command failed")
		}
	}
	if firstCommand.RequestID != "repeat-command-1" || secondCommand.RequestID != firstCommand.RequestID {
		t.Fatal("duplicate command did not return the original result")
	}
	for _, test := range []struct {
		method, suffix string
		body           any
	}{
		{http.MethodPatch, "", map[string]any{"description": "越权"}},
		{http.MethodDelete, "", map[string]string{"parent": groupID}},
		{http.MethodPost, "/move", map[string]string{"old_parent": groupID, "new_parent": groupID}},
	} {
		status, _ = call(t, test.method, base+"/nodes/"+byName["风机"].ID+test.suffix, viewerToken, test.body, nil)
		if status != http.StatusForbidden {
			t.Fatalf("viewer resource operation %s %s: %d", test.method, test.suffix, status)
		}
	}
	status, _ = call(t, http.MethodDelete, base+"/nodes/"+outsideID, token, map[string]string{"parent": snapshotRoot(t, base, token)}, nil)
	if status != http.StatusOK {
		t.Fatalf("unused point delete failed: %d", status)
	}
}
func testModbusTCP(t *testing.T, base, token, groupID string, nc *nats.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	regs := &siotmodbus.Regs{}
	regs.AddReg(0, 1)
	regs.AddReg(10, 1)
	if err := regs.WriteReg(10, 253); err != nil {
		t.Fatal(err)
	}
	modbusServer, err := siotmodbus.NewTCPServer(1, 5, strconv.Itoa(port), regs, 0)
	if err != nil {
		t.Fatal(err)
	}
	serverDone := make(chan struct{})
	go modbusServer.Listen(func(error) {}, func() {}, func() { close(serverDone) })
	defer func() {
		_ = modbusServer.Close()
		<-serverDone
	}()

	var testResult struct {
		Value []uint16 `json:"value"`
	}
	status, body := call(t, http.MethodPost, base+"/modbus/test", token, map[string]any{
		"protocol": "TCP", "uri": fmt.Sprintf("127.0.0.1:%d", port),
		"server_id": 1, "address": 10, "io_type": "modbusHoldingRegister",
	}, &testResult)
	if status != http.StatusOK || len(testResult.Value) != 1 || testResult.Value[0] != 253 {
		t.Fatalf("real Modbus read: %d %s", status, body)
	}
	var bus struct {
		ID string `json:"id"`
	}
	status, body = call(t, http.MethodPost, base+"/modbus", token, map[string]any{
		"parent": groupID, "description": "现场总线", "protocol": "TCP",
		"uri": fmt.Sprintf("127.0.0.1:%d", port), "poll_period": 50, "timeout": 500,
	}, &bus)
	if status != http.StatusOK {
		t.Fatalf("create Modbus bus: %d %s", status, body)
	}
	for _, input := range []map[string]any{
		{"description": "Modbus 温度", "server_id": 1, "address": 10, "io_type": "modbusHoldingRegister", "data_format": "uint16", "read_only": true, "scale": 0.1, "offset": 0, "units": "°C"},
		{"description": "Modbus 线圈", "server_id": 1, "address": 0, "io_type": "modbusCoil", "data_format": "uint16", "read_only": false, "scale": 1, "offset": 0},
	} {
		status, body = call(t, http.MethodPost, base+"/modbus/"+bus.ID+"/io", token, input, nil)
		if status != http.StatusOK {
			t.Fatalf("create Modbus IO: %d %s", status, body)
		}
	}

	invalidCSV := "名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位\n不应导入,1,10,输入寄存器,uint16,是,1,0,°C\n坏行,1,未知,线圈,uint16,否,1,0,\n"
	status, _ = callRaw(t, base+"/modbus/"+bus.ID+"/io/import", token, invalidCSV)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid CSV batch was accepted: %d", status)
	}
	for _, node := range snapshotNodes(t, base, token) {
		name, _ := node.Points.Text(data.PointTypeDescription, "")
		if name == "不应导入" {
			t.Fatal("invalid CSV batch partially imported")
		}
	}
	validCSV := "名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位\nCSV 温度,1,10,输入寄存器,uint16,是,0.1,0,°C\n"
	var preview struct {
		Count   int  `json:"count"`
		Preview bool `json:"preview"`
	}
	status, previewBody := callRaw(t, base+"/modbus/"+bus.ID+"/io/import?preview=true", token, validCSV)
	if err := json.Unmarshal([]byte(previewBody), &preview); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || !preview.Preview || preview.Count != 1 {
		t.Fatalf("CSV preview failed: %d", status)
	}
	for _, node := range snapshotNodes(t, base, token) {
		name, _ := node.Points.Text(data.PointTypeDescription, "")
		if name == "CSV 温度" {
			t.Fatal("CSV preview created a point")
		}
	}
	status, _ = callRaw(t, base+"/modbus/"+bus.ID+"/io/import", token, validCSV)
	if status != http.StatusOK {
		t.Fatalf("valid CSV import failed: %d", status)
	}

	var temperatureID, coilID, coilBusinessID, temperatureBusinessID string
	waitFor(t, func() bool {
		for _, node := range snapshotNodes(t, base, token) {
			description, _ := node.Points.Text(data.PointTypeDescription, "")
			if description == "Modbus 温度" {
				temperatureID = node.ID
				temperatureBusinessID, _ = node.Points.Text("businessId", "")
			}
			if description == "Modbus 线圈" {
				coilID = node.ID
				coilBusinessID, _ = node.Points.Text("businessId", "")
			}
		}
		return currentValueOf(nc, temperatureID) == 25.3 && coilID != ""
	}, "scaled Modbus value")

	var command CommandResult
	status, _ = call(t, http.MethodPost, base+"/nodes/"+coilID+"/value", token, map[string]any{"value": 1, "point_type": "valueSet", "request_id": "modbus-manual-1"}, &command)
	if status != http.StatusOK || command.Status != "confirmed" {
		t.Fatalf("manual Modbus command was not confirmed: %d %#v", status, command)
	}
	waitFor(t, func() bool {
		value, _ := regs.ReadCoil(0)
		return value && currentValueOf(nc, coilID) == 1
	}, "Modbus coil write and readback")

	document := decision.Document{Table: decision.Table{Rows: []decision.Row{{
		ID:         "hot-modbus",
		Conditions: []decision.Condition{{PointID: temperatureBusinessID, Operator: "gt", Value: 30}},
		Actions:    []decision.Action{{ActionID: coilBusinessID, Value: 1}},
	}}}, Bindings: decision.BindingSet{
		Points:  map[string]decision.Target{temperatureBusinessID: {NodeID: temperatureID, PointType: "value"}},
		Actions: map[string]decision.Target{coilBusinessID: {NodeID: coilID, PointType: "valueSet"}},
	}}
	var rule struct {
		ID string `json:"id"`
	}
	status, _ = call(t, http.MethodPost, base+"/rules", token, map[string]any{"parent": groupID, "description": "Modbus 温控", "document": document}, &rule)
	if status != http.StatusOK {
		t.Fatal("create Modbus rule failed")
	}
	status, _ = call(t, http.MethodPost, base+"/rules/"+rule.ID+"/publish", token, document, nil)
	if status != http.StatusOK {
		t.Fatal("publish Modbus rule failed")
	}
	status, _ = call(t, http.MethodPost, base+"/nodes/"+coilID+"/value", token, map[string]any{"value": 0, "point_type": "valueSet"}, nil)
	if status != http.StatusOK {
		t.Fatal("reset coil failed")
	}
	status, _ = call(t, http.MethodPost, base+"/rules/"+rule.ID+"/enable", token, map[string]bool{"enabled": true}, nil)
	if status != http.StatusOK {
		t.Fatal("enable Modbus rule failed")
	}
	if err := regs.WriteReg(10, 351); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		value, _ := regs.ReadCoil(0)
		return currentValueOf(nc, temperatureID) == 35.1 && value
	}, "decision table Modbus actuation")

	var template struct {
		ID string `json:"id"`
	}
	status, _ = call(t, http.MethodPost, base+"/templates", token, map[string]string{
		"parent": groupID, "description": "标准现场设备", "source_bus_id": bus.ID,
	}, &template)
	if status != http.StatusOK || template.ID == "" {
		t.Fatal("save device template failed")
	}
	templates, _ := client.GetNodesType[DeviceTemplate](nc, "all", template.ID)
	if len(templates) != 1 || strings.Contains(templates[0].Definition, bus.ID) || strings.Contains(templates[0].Definition, "businessId") {
		t.Fatal("template contains source IDs")
	}
	var secondBus struct {
		ID string `json:"id"`
	}
	status, _ = call(t, http.MethodPost, base+"/modbus", token, map[string]any{
		"parent": groupID, "description": "模板总线", "protocol": "TCP",
		"uri": fmt.Sprintf("127.0.0.1:%d", port), "poll_period": 50, "timeout": 500,
	}, &secondBus)
	if status != http.StatusOK {
		t.Fatal("create template target bus failed")
	}
	status, _ = call(t, http.MethodPost, base+"/templates/"+template.ID+"/apply", token, map[string]string{"bus_id": secondBus.ID}, nil)
	if status != http.StatusOK {
		t.Fatal("apply template failed")
	}
	waitFor(t, func() bool {
		children, _ := client.GetNodes(nc, secondBus.ID, "all", "", false)
		return len(children) == 3
	}, "template points")
	status, _ = call(t, http.MethodPost, base+"/templates/"+template.ID+"/apply", token, map[string]string{"bus_id": secondBus.ID}, nil)
	if status != http.StatusConflict {
		t.Fatalf("duplicate template apply accepted: %d", status)
	}

	status, exported := call(t, http.MethodGet, base+"/modbus/"+bus.ID+"/io/export", token, nil, nil)
	if status != http.StatusOK || !strings.Contains(exported, "Modbus 温度") {
		t.Fatalf("CSV export failed: %d", status)
	}
	var thirdBus struct {
		ID string `json:"id"`
	}
	status, _ = call(t, http.MethodPost, base+"/modbus", token, map[string]any{
		"parent": groupID, "description": "CSV 导入总线", "protocol": "TCP",
		"uri": fmt.Sprintf("127.0.0.1:%d", port), "poll_period": 50, "timeout": 500,
	}, &thirdBus)
	if status != http.StatusOK {
		t.Fatal("create CSV target bus failed")
	}
	status, _ = callRaw(t, base+"/modbus/"+thirdBus.ID+"/io/import", token, exported)
	if status != http.StatusOK {
		t.Fatalf("exported CSV could not be imported: %d", status)
	}
	status, _ = call(t, http.MethodPost, base+"/modbus/"+bus.ID+"/io", token, map[string]any{
		"description": "无回应点位", "server_id": 1, "address": 15, "io_type": "modbusHoldingRegister",
		"data_format": "uint16", "read_only": false, "scale": 1, "offset": 0,
	}, nil)
	if status != http.StatusOK {
		t.Fatal("create unavailable point failed")
	}
	var unknown CommandResult
	// Find the new point and verify that a missing readback is unknown.
	for _, node := range snapshotNodes(t, base, token) {
		name, _ := node.Points.Text(data.PointTypeDescription, "")
		if name == "无回应点位" {
			status, _ = call(t, http.MethodPost, base+"/nodes/"+node.ID+"/value", token, map[string]any{"value": 7, "point_type": "valueSet", "request_id": "modbus-unknown-1"}, &unknown)
			if status != http.StatusOK || unknown.Status != "unknown" {
				t.Fatalf("missing readback should be unknown: %d %#v", status, unknown)
			}
		}
	}
	if unknown.RequestID == "" {
		t.Fatal("unavailable point was not tested")
	}
	var delivery struct {
		Items []DeliveryItem `json:"items"`
	}
	status, _ = call(t, http.MethodGet, base+"/delivery/check", token, nil, &delivery)
	if status != http.StatusOK {
		t.Fatal("delivery check failed")
	}
	foundCommandFailure := false
	for _, item := range delivery.Items {
		if item.ID == "commands" && item.Status == "fail" && strings.Contains(item.Detail, "无回应点位") {
			foundCommandFailure = true
		}
	}
	if !foundCommandFailure {
		t.Fatal("delivery check did not report unknown command")
	}
}

func loginAPI(t *testing.T, base, email, password string) string {
	t.Helper()
	var result struct {
		Token string `json:"token"`
	}
	status, _ := call(t, http.MethodPost, base+"/login", "", map[string]string{"email": email, "password": password}, &result)
	if status != http.StatusOK || result.Token == "" {
		t.Fatalf("login %s: %d", email, status)
	}
	return result.Token
}

func loginAPIOptional(base, email, password string) string {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	response, err := http.Post(base+"/login", "application/json", bytes.NewReader(body))
	if err != nil || response.StatusCode != http.StatusOK {
		return ""
	}
	defer response.Body.Close()
	var result struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(response.Body).Decode(&result) != nil {
		return ""
	}
	return result.Token
}

func createNodeAPI(t *testing.T, base, token, typ, parent, name string, value float64) string {
	t.Helper()
	var result struct {
		ID string `json:"id"`
	}
	status, body := call(t, http.MethodPost, base+"/nodes", token, map[string]any{
		"type": typ, "parent": parent, "description": name, "value": value,
	}, &result)
	if status != http.StatusOK || result.ID == "" {
		t.Fatalf("create %s: %d %s", name, status, body)
	}
	return result.ID
}

func snapshotRoot(t *testing.T, base, token string) string {
	t.Helper()
	var result struct {
		Roots []string `json:"roots"`
	}
	status, _ := call(t, http.MethodGet, base+"/snapshot", token, nil, &result)
	if status != http.StatusOK || len(result.Roots) != 1 {
		t.Fatalf("roots: %d %#v", status, result.Roots)
	}
	return result.Roots[0]
}

func snapshotNodes(t *testing.T, base, token string) []data.NodeEdge {
	t.Helper()
	var result struct {
		Nodes []data.NodeEdge `json:"nodes"`
	}
	status, _ := call(t, http.MethodGet, base+"/snapshot", token, nil, &result)
	if status != http.StatusOK {
		t.Fatalf("snapshot: %d", status)
	}
	return result.Nodes
}

func call(t *testing.T, method, url, token string, body, result any) (int, string) {
	t.Helper()
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, input)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil && response.StatusCode == http.StatusOK {
		if err := json.Unmarshal(data, result); err != nil {
			t.Fatalf("decode %s: %v; %s", url, err, data)
		}
	}
	return response.StatusCode, string(data)
}

func callRaw(t *testing.T, url, token, body string) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "text/csv")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(data)
}

func currentValueOf(nc *nats.Conn, id string) float64 {
	if id == "" {
		return -1
	}
	nodes, err := client.GetNodes(nc, "all", id, "", false)
	if err != nil || len(nodes) == 0 {
		return -1
	}
	point, ok := nodes[0].Points.Find(data.PointTypeValue, "")
	if !ok {
		return -1
	}
	value, _ := point.ValueFloat()
	return value
}

func valueOf(t *testing.T, nc *nats.Conn, id string) float64 {
	t.Helper()
	nodes, err := client.GetNodes(nc, "all", id, "", false)
	if err != nil || len(nodes) == 0 {
		return 0
	}
	point, ok := nodes[0].Points.Find(data.PointTypeValueSet, "")
	if !ok {
		return 0
	}
	value, err := point.ValueFloat()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func waitFor(t *testing.T, check func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("timeout waiting for " + description)
}
