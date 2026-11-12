package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"

	"easyems/internal/ems/runtime"
	"easyems/internal/emsapi"
)

func TestCriticalPointSet(t *testing.T) {
	applied, _ := json.Marshal(map[string]any{
		"table": map[string]any{"rows": []any{}},
		"bindings": map[string]any{
			"points":  map[string]any{"p1": map[string]string{"node_id": "pt-rule-point"}},
			"actions": map[string]any{"a1": map[string]string{"node_id": "pt-rule-action"}},
		},
	})
	// PCCFeedback 没有 json tag，存储与读取都用 Go 字段名。
	pcc, _ := json.Marshal(map[string]any{
		"MeterPointID":  "pt-meter",
		"PhasePointIDs": []string{"pt-a"},
	})
	capability, _ := json.Marshal(map[string]any{
		"soc_point_id": "pt-soc",
		"max_age_ms":   7000,
	})
	nodes := []data.NodeEdge{
		{ID: "rule-1", Type: "decisionTable", Points: data.Points{
			data.NewPointFloat("enabled", "0", 1),
			data.NewPointString("applied", "0", string(applied)),
		}},
		{ID: "plant-1", Type: NodeTypePlant, Points: data.Points{
			data.NewPointString(PointTypePCCFeedback, "0", string(pcc)),
		}},
		{ID: "unit-1", Type: NodeTypeEnergyUnit, Points: data.Points{
			data.NewPointString(capabilityInputsPoint, "0", string(capability)),
		}},
	}
	points, ages := criticalPointSet(nodes)
	for _, id := range []string{"pt-rule-point", "pt-rule-action", "pt-meter", "pt-a", "pt-soc"} {
		if !points[id] {
			t.Fatalf("critical point %s missing: %#v", id, points)
		}
	}
	if ages["pt-soc"] != 7*time.Second {
		t.Fatalf("capability max age = %v, want 7s", ages["pt-soc"])
	}
	if len(points) != 5 {
		t.Fatalf("critical points = %d, want 5: %#v", len(points), points)
	}

	// 未启用的决策表不产生关键点位。
	nodes[0].Points = data.Points{data.NewPointFloat("enabled", "0", 0), data.NewPointString("applied", "0", string(applied))}
	points, _ = criticalPointSet(nodes)
	if points["pt-rule-point"] {
		t.Fatal("disabled rule must not mark its points critical")
	}
}

func TestPreflightReportPersistence(t *testing.T) {
	handler := &Handler{dataDir: t.TempDir()}
	if report, err := handler.readLatestPreflight(""); err != nil || report != nil {
		t.Fatalf("empty store = %#v, %v", report, err)
	}
	first := preflightReport{ID: "pf-1", Status: "warn", Summary: map[string]int{"pass": 1, "warn": 1, "fail": 0}}
	second := preflightReport{ID: "pf-2", Status: "fail", Summary: map[string]int{"pass": 0, "warn": 0, "fail": 1}}
	if err := handler.appendPreflight(first); err != nil {
		t.Fatal(err)
	}
	if err := handler.appendPreflight(second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(handler.dataDir, preflightFile)); err != nil {
		t.Fatalf("preflight file: %v", err)
	}
	latest, err := handler.readLatestPreflight("")
	if err != nil || latest == nil || latest.ID != "pf-2" {
		t.Fatalf("latest = %#v, %v", latest, err)
	}
}

// TestDeliveryPreflight 覆盖门槛与预检的 HTTP 语义：无通道时通道门槛阻断；
// 查看者无权运行；预检写文件并可取回最近一次。
func TestDeliveryPreflight(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-preflight", DataDir: dataDir, HTTPPort: "9830",
		NatsPort: 9831, NatsServer: "nats://127.0.0.1:9831", AuthToken: data.NewID(),
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

	handler := New(nc, users, nil)
	handler.SetDataDir(dataDir)
	handler.SetEMS(emsapi.New(runtime.NewService(nil, 15*time.Second, 5*time.Second, 100), 15*time.Second))
	admin := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true}, Anchors: []string{root.ID}}
	viewer := Identity{UserID: "viewer", Policy: AccessPolicy{Role: "viewer"}, Anchors: []string{root.ID}}

	recorder := httptest.NewRecorder()
	handler.deliveryGates(recorder, admin, "")
	if recorder.Code != 200 {
		t.Fatalf("gates = %d %s", recorder.Code, recorder.Body.String())
	}
	var gates preflightReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &gates); err != nil {
		t.Fatal(err)
	}
	if gates.Status != "fail" || len(gates.Gates) != 10 {
		t.Fatalf("report = %s with %d gates", gates.Status, len(gates.Gates))
	}
	channelFailed := false
	for _, gate := range gates.Gates {
		if gate.Key == "channels" && gate.Status == "fail" {
			channelFailed = true
		}
	}
	if !channelFailed {
		t.Fatal("empty site must fail the channel gate")
	}

	recorder = httptest.NewRecorder()
	handler.deliveryGates(recorder, viewer, "")
	if recorder.Code != 403 {
		t.Fatalf("viewer gates = %d, want 403", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	handler.runPreflight(recorder, viewer, "")
	if recorder.Code != 403 {
		t.Fatalf("viewer preflight = %d, want 403", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.runPreflight(recorder, admin, "")
	if recorder.Code != 200 {
		t.Fatalf("preflight = %d %s", recorder.Code, recorder.Body.String())
	}
	var snapshot preflightReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ID == "" || snapshot.ID[:3] != "pf-" || snapshot.Operator != "admin" {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	recorder = httptest.NewRecorder()
	handler.latestPreflight(recorder, admin, "")
	if recorder.Code != 200 {
		t.Fatalf("latest = %d", recorder.Code)
	}
	var latestPayload struct {
		Latest *preflightReport `json:"latest"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &latestPayload); err != nil {
		t.Fatal(err)
	}
	if latestPayload.Latest == nil || latestPayload.Latest.ID != snapshot.ID {
		t.Fatalf("latest = %#v, want %s", latestPayload.Latest, snapshot.ID)
	}

	empty := &Handler{dataDir: t.TempDir()}
	recorder = httptest.NewRecorder()
	empty.latestPreflight(recorder, admin, "")
	if recorder.Code != 200 {
		t.Fatalf("empty latest = %d, want 200", recorder.Code)
	}
	var emptyPayload struct {
		Latest *preflightReport `json:"latest"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &emptyPayload); err != nil {
		t.Fatal(err)
	}
	if emptyPayload.Latest != nil {
		t.Fatalf("empty latest = %#v, want null", emptyPayload.Latest)
	}
}
