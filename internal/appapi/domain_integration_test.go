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
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"
)

// TestCreateDomainNodeHierarchy 验证四层领域节点（site→plant→energyUnit→device）
// 能创建、父子关系正确、字段可读写。
func TestCreateDomainNodeHierarchy(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-domain-test", DataDir: dataDir, HTTPPort: "9713",
		NatsPort: 9712, NatsServer: "nats://127.0.0.1:9712", AuthToken: data.NewID(),
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

	// Store 的 nodes.*.* 订阅是异步建立的，轮询等它就绪（同 Bootstrap）。
	readyDeadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := client.GetRootNode(nc); err == nil {
			break
		}
		if time.Now().After(readyDeadline) {
			t.Fatal("store not ready for node requests")
		}
		time.Sleep(50 * time.Millisecond)
	}

	siteID, err := CreateDomainNode(nc, "root", NodeTypeSite, map[string]string{
		PointTypeAssetCode: "ES01", "description": "场站一",
	})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	plantID, err := CreateDomainNode(nc, siteID, NodeTypePlant, map[string]string{
		PointTypeAssetCode: "ES01-PLANT01", "description": "电站一",
	})
	if err != nil {
		t.Fatalf("create plant: %v", err)
	}
	energyUnitID, err := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode:    "ES01-PLANT01-CAB01",
		PointTypeEnergyType:   EnergyTypeStorage,
		PointTypeRatedPowerKW: "250",
	})
	if err != nil {
		t.Fatalf("create energy unit: %v", err)
	}
	deviceID, err := CreateDomainNode(nc, energyUnitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "ES01-PLANT01-CAB01-PCS01",
	})
	if err != nil {
		t.Fatalf("create device: %v", err)
	}

	assertParent := func(child, wantParent string) {
		t.Helper()
		nodes, err := client.GetNodes(nc, "all", child, "", false)
		if err != nil {
			t.Fatal(err)
		}
		for _, node := range nodes {
			if node.ID == child {
				if node.Parent != wantParent {
					t.Fatalf("node %s parent = %s, want %s", child, node.Parent, wantParent)
				}
				return
			}
		}
		t.Fatalf("node %s not found", child)
	}
	assertParent(plantID, siteID)
	assertParent(energyUnitID, plantID)
	assertParent(deviceID, energyUnitID)
}
