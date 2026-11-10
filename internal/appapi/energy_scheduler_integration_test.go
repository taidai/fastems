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

// TestSchedulerDispatchesEveryUnit 验证调度器对每个 EnergyUnit 调用一次
// 统一仲裁入口（站级目标由入口按所属 Plant 收集），不再单独跑站级写入。
func TestSchedulerDispatchesEveryUnit(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-plant-sched", DataDir: dataDir, HTTPPort: "9736",
		NatsPort: 9737, NatsServer: "nats://127.0.0.1:9737", AuthToken: data.NewID(),
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
	root, err := siotclient.GetRootNode(nc)
	if err != nil {
		t.Fatal(err)
	}

	plantID, err := CreateDomainNode(nc, root.ID, NodeTypePlant, map[string]string{PointTypeAssetCode: "P1"})
	if err != nil {
		t.Fatal(err)
	}
	unitID, err := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: "EU1"})
	if err != nil {
		t.Fatal(err)
	}
	// 停机的单元不应被自动调度。
	stoppedID, err := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: "EU2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCabinetState(nc, stoppedID, "stopped"); err != nil {
		t.Fatal(err)
	}

	called := map[string]int{}
	scheduler := NewEnergyScheduler(nc, func(id string) error {
		called[id]++
		return nil
	})
	scheduler.tick()
	if called[unitID] != 1 {
		t.Fatalf("运行态单元应被调用一次，实测 %d", called[unitID])
	}
	if called[stoppedID] != 0 {
		t.Fatalf("停机单元不应被自动调度，实测 %d", called[stoppedID])
	}
}
