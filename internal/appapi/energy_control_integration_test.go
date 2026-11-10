package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"
)

// TestEnergyControlEvents 验证控制器人工操作的 HTTP 语义：人工模式立即停止
// 自动调度；检修/故障确认走状态机；没有命令运行时不假装启停成功。
func TestEnergyControlEvents(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-energy-control", DataDir: dataDir, HTTPPort: "9820",
		NatsPort: 9821, NatsServer: "nats://127.0.0.1:9821", AuthToken: data.NewID(),
		DeviceAuth: server.DeviceAuthRequired, CustomUIFS: fstest.MapFS{"index.html": {Data: []byte("easyems")}},
		AppOnly: true, NatsLoopback: true, ProvisioningDisable: true,
		AppHandlerFactory: func(nc *nats.Conn, u api.UserAuthority) http.Handler {
			users = u
			return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
		},
	}
	siot, nc, err := server.NewServer(options)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- siot.Run() }()
	defer func() { siot.Stop(nil); <-done }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := siot.WaitStart(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	var root data.NodeEdge
	for {
		root, err = siotclient.GetRootNode(nc)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("store not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}

	plantID, _ := CreateDomainNode(nc, root.ID, NodeTypePlant, map[string]string{PointTypeAssetCode: "P1"})
	unitID, _ := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: "EU1"})
	handler := New(nc, users, nil)
	identity := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true}, Anchors: []string{root.ID}}

	call := func(method, action string, body any) (int, map[string]any) {
		t.Helper()
		var reader *bytes.Reader
		if body != nil {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		} else {
			reader = bytes.NewReader(nil)
		}
		request := httptest.NewRequest(method, "/api/energy-units/"+unitID+"/"+action, reader)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.energyControl(recorder, request, identity, unitID, action)
		var payload map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &payload)
		return recorder.Code, payload
	}
	state := func() string {
		nodes, _ := siotclient.GetNodes(nc, "all", unitID, "", false)
		return string(LoadCabinetState(nodes[0]))
	}

	if status, _ := call(http.MethodPut, "mode", map[string]string{"mode": "manual"}); status != http.StatusOK {
		t.Fatalf("manual mode = %d", status)
	}
	nodes, _ := siotclient.GetNodes(nc, "all", unitID, "", false)
	if AllowsEnergyUnitAutoDispatch(nodes[0]) {
		t.Fatal("manual mode must block auto dispatch")
	}
	if status, _ := call(http.MethodPut, "mode", map[string]string{"mode": "auto"}); status != http.StatusOK {
		t.Fatalf("auto mode = %d", status)
	}

	if status, payload := call(http.MethodPost, "events", map[string]string{"event": "enter_maintenance"}); status != http.StatusOK || payload["state"] != "maintenance" {
		t.Fatalf("enter maintenance = %d %v", status, payload)
	}
	if status, _ := call(http.MethodPost, "events", map[string]string{"event": "start", "request_id": "r-maintenance"}); status != http.StatusConflict {
		t.Fatalf("start during maintenance = %d, want conflict", status)
	}
	if status, _ := call(http.MethodPost, "events", map[string]string{"event": "exit_maintenance"}); status != http.StatusOK {
		t.Fatalf("exit maintenance = %d", status)
	}
	if state() != "standby" {
		t.Fatalf("state after maintenance = %s", state())
	}

	// 没有命令运行时，启停必须明确失败而不是假装成功。
	if status, payload := call(http.MethodPost, "events", map[string]string{"event": "start", "request_id": "r-start"}); status != http.StatusServiceUnavailable || payload["error"] != "command_runtime_unavailable" {
		t.Fatalf("start without runtime = %d %v", status, payload)
	}

	if err := SetCabinetState(nc, unitID, "fault"); err != nil {
		t.Fatal(err)
	}
	if status, _ := call(http.MethodPost, "events", map[string]string{"event": "fault_clear"}); status != http.StatusOK {
		t.Fatalf("fault clear = %d", status)
	}
	if state() != "standby" {
		t.Fatalf("state after fault clear = %s", state())
	}
}
