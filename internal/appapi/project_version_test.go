package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"

	"fastems/internal/decision"
	"fastems/internal/ems/runtime"
	"fastems/internal/emsapi"
)

func TestProjectVersionPublishAndRollback(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-versions", DataDir: dataDir, HTTPPort: "9850",
		NatsPort: 9851, NatsServer: "nats://127.0.0.1:9851", AuthToken: data.NewID(),
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

	siteID, _ := CreateDomainNode(nc, root.ID, NodeTypeSite, map[string]string{"description": "版本站"})
	plantID, _ := CreateDomainNode(nc, siteID, NodeTypePlant, map[string]string{"description": "A 电站"})
	meterA, _ := CreateDomainNode(nc, plantID, "variable", map[string]string{"description": "主表 A"})
	meterB, _ := CreateDomainNode(nc, plantID, "variable", map[string]string{"description": "主表 B"})
	time.Sleep(100 * time.Millisecond)
	writePCC := func(meter string) {
		encoded, _ := json.Marshal(decision.PCCFeedback{MeterPointID: meter})
		if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(plantID), data.Points{
			data.NewPointString(PointTypePCCFeedback, "0", string(encoded)),
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	writePCC(meterA)
	if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(siteID), data.Points{
		data.NewPointString("apiToken", "0", "secret"),
	}, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	handler := New(nc, users, nil)
	handler.SetDataDir(dataDir)
	handler.SetEMS(emsapi.New(runtime.NewService(nil, 15*time.Second, 5*time.Second, 100), 15*time.Second))
	admin := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true}, Anchors: []string{root.ID}}
	viewer := Identity{UserID: "viewer", Policy: AccessPolicy{Role: "viewer"}, Anchors: []string{root.ID}}

	recorder := httptest.NewRecorder()
	handler.publishHandler(recorder, viewer, siteID, "", false)
	if recorder.Code != 403 {
		t.Fatalf("viewer publish = %d, want 403", recorder.Code)
	}

	// 站点没有可用通道时应先被预检阻断；确认后才允许冻结版本。
	recorder = httptest.NewRecorder()
	handler.publishHandler(recorder, admin, siteID, "首次发布", false)
	if recorder.Code != 409 {
		t.Fatalf("publish without confirm = %d, want 409", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.publishHandler(recorder, admin, siteID, "首次发布", true)
	if recorder.Code != 200 {
		t.Fatalf("publish = %d %s", recorder.Code, recorder.Body.String())
	}
	var published versionSummary
	if err := json.Unmarshal(recorder.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if published.ID == "" || published.Nodes < 4 {
		t.Fatalf("published = %#v", published)
	}

	raw, err := os.ReadFile(filepath.Join(handler.projectDir(siteID), published.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "apiToken") || strings.Contains(string(raw), `"secret"`) {
		t.Fatal("version must not contain credentials")
	}
	var stored projectVersion
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	for _, node := range stored.Nodes {
		if node.ID != plantID {
			continue
		}
		types := make([]string, 0, len(node.Points))
		for _, p := range node.Points {
			types = append(types, p.Type)
		}
		t.Logf("plant points in version: %v", types)
	}

	recorder = httptest.NewRecorder()
	handler.versionsHandler(recorder, admin, siteID)
	if recorder.Code != 200 {
		t.Fatalf("versions = %d", recorder.Code)
	}
	var listed struct {
		Versions []versionSummary `json:"versions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Versions) != 1 || listed.Versions[0].ID != published.ID {
		t.Fatalf("versions = %#v", listed.Versions)
	}

	// 发布后改配置，回退应恢复发布时的绑定。
	writePCC(meterB)
	waitPCC := func(meter string) {
		deadline := time.Now().Add(5 * time.Second)
		for {
			nodes, err := handler.loadSubtree(siteID)
			if err == nil {
				for _, node := range nodes {
					if binding, ok := loadPCCFeedback(node); ok && binding.MeterPointID == meter {
						return
					}
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("pcc binding did not become %s", meter)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	waitPCC(meterB)

	recorder = httptest.NewRecorder()
	handler.rollbackHandler(recorder, viewer, siteID, published.ID)
	if recorder.Code != 403 {
		t.Fatalf("viewer rollback = %d, want 403", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.rollbackHandler(recorder, admin, siteID, published.ID)
	if recorder.Code != 200 {
		t.Fatalf("rollback = %d %s", recorder.Code, recorder.Body.String())
	}
	var rolled rollbackResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &rolled); err != nil {
		t.Fatal(err)
	}
	if rolled.Updated < 4 {
		t.Fatalf("rollback result = %#v", rolled)
	}

	deadline = time.Now().Add(5 * time.Second)
	for {
		nodes, err := handler.loadSubtree(siteID)
		if err == nil {
			var pcc decision.PCCFeedback
			for _, node := range nodes {
				if binding, ok := loadPCCFeedback(node); ok {
					pcc = binding
				}
			}
			if pcc.MeterPointID == meterA {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("pcc binding was not restored")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 回退本身也留下一个"回退前自动快照"，历史可追。
	summaries, err := handler.listVersions(siteID)
	if err != nil || len(summaries) < 2 {
		t.Fatalf("versions after rollback = %d, %v", len(summaries), err)
	}
}
