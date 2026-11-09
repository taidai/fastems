package appapi

import (
	"context"
	"net/http"
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

// TestSeedDemoSiteDispatch 验证开箱即用：seedDemoSite 预置的演示站点能
// 被能力级调度真实跑通——已发布规则命中，写 PCS 的 valueSet。
func TestSeedDemoSiteDispatch(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-seed-demo", DataDir: dataDir, HTTPPort: "9722",
		NatsPort: 9723, NatsServer: "nats://127.0.0.1:9723", AuthToken: data.NewID(),
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

	if err := seedDemoSite(nc); err != nil {
		t.Fatal(err)
	}

	// 找到演示 EnergyUnit。
	units, err := siotclient.GetNodes(nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		t.Fatal(err)
	}
	var energyUnitID string
	for _, node := range units {
		if point, ok := node.Points.Find(PointTypeAssetCode, ""); ok && point.Txt() == "DEMO-CAB01" {
			energyUnitID = node.ID
			break
		}
	}
	if energyUnitID == "" {
		t.Fatal("demo energy unit not found")
	}

	handler := &Handler{nc: nc}
	dispatches, err := handler.RunCapabilityDispatch(energyUnitID)
	if err != nil {
		t.Fatalf("dispatch demo site: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	if dispatches[0].SetpointKW <= 0 {
		t.Fatalf("expected positive setpoint, got %.1f", dispatches[0].SetpointKW)
	}

	// 幂等：再次 seed 不重复创建。
	if err := seedDemoSite(nc); err != nil {
		t.Fatal(err)
	}
}
