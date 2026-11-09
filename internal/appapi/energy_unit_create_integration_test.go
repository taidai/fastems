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

// TestCreateEnergyUnitByUser 验证用户能自建能源节点：POST 创建后出现在
// 列表、自动归属默认 Plant、asset_code 生成。这是"能用"而非"演示"的关键。
func TestCreateEnergyUnitByUser(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-create-eu", DataDir: dataDir, HTTPPort: "9730",
		NatsPort: 9731, NatsServer: "nats://127.0.0.1:9731", AuthToken: data.NewID(),
		DeviceAuth: server.DeviceAuthRequired, CustomUIFS: fstest.MapFS{"index.html": {Data: []byte("easyems")}},
		AppOnly: true, NatsLoopback: true, ProvisioningDisable: true,
		AppHandlerFactory: func(nc *nats.Conn, users api.UserAuthority) http.Handler {
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
	readyDeadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := siotclient.GetRootNode(nc); err == nil {
			break
		}
		if time.Now().After(readyDeadline) {
			t.Fatal("store not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}

	handler := &Handler{nc: nc}
	identity := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true, Role: "admin"}}

	body, _ := json.Marshal(map[string]any{"description": "2 号储能柜", "rated_power_kw": 120})
	req := httptest.NewRequest(http.MethodPost, "/api/energy-units", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.createEnergyUnit(rec, req, identity)

	if rec.Code != http.StatusOK {
		t.Fatalf("create failed: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID        string `json:"id"`
		AssetCode string `json:"asset_code"`
		PlantID   string `json:"plant_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.AssetCode == "" || created.PlantID == "" {
		t.Fatalf("incomplete create response: %+v", created)
	}

	// 自动归属的 Plant 应存在。
	plants, err := siotclient.GetNodes(nc, "all", created.PlantID, "", false)
	if err != nil || len(plants) == 0 {
		t.Fatal("default plant was not created")
	}

	// 新能源节点应在列表中，额定功率正确。
	nodes, err := siotclient.GetNodes(nc, "all", created.ID, "", false)
	if err != nil || len(nodes) == 0 {
		t.Fatal("created energy unit not found")
	}
	rated, found := nodes[0].Points.Find(PointTypeRatedPowerKW, "")
	if !found || pointFloat(rated) != 120 {
		t.Fatalf("rated power mismatch: %+v", rated)
	}
	if nodes[0].Parent != created.PlantID {
		t.Fatalf("energy unit parent = %s, want %s", nodes[0].Parent, created.PlantID)
	}
}
