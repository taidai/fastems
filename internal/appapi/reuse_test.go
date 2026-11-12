package appapi

import (
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

	"fastems/internal/decision"
)

func TestRewriteReferences(t *testing.T) {
	idMap := map[string]string{"old-meter": "new-meter", "old-a": "new-a", "old-unit": "new-unit"}
	pcc, _ := json.Marshal(decision.PCCFeedback{MeterPointID: "old-meter", PhasePointIDs: []string{"old-a", "external"}})
	document, _ := json.Marshal(decision.Document{
		Table: decision.Table{},
		Bindings: decision.BindingSet{
			Points:  map[string]decision.Target{"p1": {NodeID: "old-unit"}, "p2": {NodeID: "external"}},
			Actions: map[string]decision.Target{"a1": {NodeID: "old-meter"}},
		},
	})
	node := data.NodeEdge{ID: "plant-1", Type: NodeTypePlant, Points: data.Points{
		data.NewPointString(PointTypePCCFeedback, "0", string(pcc)),
	}}
	points, unresolved := rewriteReferences(node, idMap)
	if len(points) != 1 {
		t.Fatalf("points = %d, want 1", len(points))
	}
	var binding decision.PCCFeedback
	if err := json.Unmarshal([]byte(points[0].Txt()), &binding); err != nil {
		t.Fatal(err)
	}
	if binding.MeterPointID != "new-meter" || len(binding.PhasePointIDs) != 1 || binding.PhasePointIDs[0] != "new-a" {
		t.Fatalf("rewritten binding = %#v", binding)
	}
	if len(unresolved) != 1 || unresolved[0] != "external" {
		t.Fatalf("unresolved = %#v", unresolved)
	}

	rule := data.NodeEdge{ID: "rule-1", Type: "decisionTable", Points: data.Points{
		data.NewPointString("applied", "0", string(document)),
	}}
	points, unresolved = rewriteReferences(rule, idMap)
	if len(points) != 1 {
		t.Fatalf("rule points = %d, want 1", len(points))
	}
	var rewritten decision.Document
	if err := json.Unmarshal([]byte(points[0].Txt()), &rewritten); err != nil {
		t.Fatal(err)
	}
	if rewritten.Bindings.Points["p1"].NodeID != "new-unit" || rewritten.Bindings.Actions["a1"].NodeID != "new-meter" {
		t.Fatalf("rewritten document = %#v", rewritten.Bindings)
	}
	if _, ok := rewritten.Bindings.Points["p2"]; ok {
		t.Fatal("unresolved binding must be removed")
	}
	if len(unresolved) != 1 || unresolved[0] != "external" {
		t.Fatalf("rule unresolved = %#v", unresolved)
	}
}

func TestReuseProject(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-reuse", DataDir: dataDir, HTTPPort: "9840",
		NatsPort: 9841, NatsServer: "nats://127.0.0.1:9841", AuthToken: data.NewID(),
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

	siteID, err := CreateDomainNode(nc, root.ID, NodeTypeSite, map[string]string{"description": "一期"})
	if err != nil {
		t.Fatal(err)
	}
	plantID, _ := CreateDomainNode(nc, siteID, NodeTypePlant, map[string]string{"description": "A 电站"})
	meterID, _ := CreateDomainNode(nc, plantID, "variable", map[string]string{"description": "主表"})
	unitID, _ := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{"description": "EU-1"})
	time.Sleep(100 * time.Millisecond)

	pcc, _ := json.Marshal(decision.PCCFeedback{MeterPointID: meterID})
	if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(plantID), data.Points{
		data.NewPointString(PointTypePCCFeedback, "0", string(pcc)),
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(unitID), data.Points{
		data.NewPointString(reuseSetpointPoint, "0", meterID),
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(siteID), data.Points{
		data.NewPointString("apiToken", "0", "secret-value"),
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(meterID), data.Points{
		data.NewPointFloat("value", "0", 123),
	}, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	handler := New(nc, users, nil)
	handler.SetDataDir(dataDir)
	admin := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true}, Anchors: []string{root.ID}}
	// 复用源必须是已发布（已验收）项目：先冻结一个基线版本。
	baseline, err := handler.snapshotSite(admin, siteID, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.saveVersion(baseline); err != nil {
		t.Fatal(err)
	}
	viewer := Identity{UserID: "viewer", Policy: AccessPolicy{Role: "viewer"}, Anchors: []string{root.ID}}

	preview, err := handler.buildReusePreview(admin, siteID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Counts["change"] < 1 || preview.Counts["confirm"] < 1 {
		t.Fatalf("preview counts = %#v", preview.Counts)
	}

	recorder := httptest.NewRecorder()
	handler.reuseProjectHandler(recorder, viewer, reuseRequest{Source: siteID, Name: "二期"})
	if recorder.Code != 403 {
		t.Fatalf("viewer reuse = %d, want 403", recorder.Code)
	}

	result, err := handler.createProjectFromSource(admin, reuseRequest{Source: siteID, Name: "二期"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SiteID == "" || result.SiteID == siteID || result.Created < 4 {
		t.Fatalf("result = %#v", result)
	}

	var cloned []data.NodeEdge
	deadline = time.Now().Add(5 * time.Second)
	for {
		cloned, err = handler.loadSubtree(result.SiteID)
		if err == nil && len(cloned) >= 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cloned subtree not ready: %d nodes, %v", len(cloned), err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	var newMeter, newUnit, newPlant data.NodeEdge
	for _, node := range cloned {
		switch node.Desc() {
		case "主表":
			newMeter = node
		case "EU-1":
			newUnit = node
		case "A 电站":
			newPlant = node
		}
		if point, ok := node.Points.Find("apiToken", ""); ok && point.Txt() != "" {
			t.Fatal("secret point must not be copied")
		}
	}
	if newMeter.ID == "" || newMeter.ID == meterID {
		t.Fatalf("meter not cloned: %#v", newMeter.ID)
	}
	if _, ok := newMeter.Points.Find("value", ""); ok {
		t.Fatal("runtime value must not be copied")
	}
	binding, ok := loadPCCFeedback(newPlant)
	if !ok || binding.MeterPointID != newMeter.ID {
		t.Fatalf("pcc binding = %#v, want meter %s", binding, newMeter.ID)
	}
	if setpoint, _ := newUnit.Points.Text(reuseSetpointPoint, ""); setpoint != newMeter.ID {
		t.Fatalf("setpoint = %q, want %s", setpoint, newMeter.ID)
	}
	if state, _ := newUnit.Points.Text(PointTypeCabinetState, ""); state != string(decision.CabinetStandby) {
		t.Fatalf("cloned unit state = %q, want standby", state)
	}
}
