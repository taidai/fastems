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

	"easyems/internal/decision"
)

// TestSchedulerRunsPlantStrategy 验证站级策略进入周期调度：有 PCC 绑定的
// Plant 会被 EnergyScheduler 每周期触发防逆流。
func TestSchedulerRunsPlantStrategy(t *testing.T) {
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

	// 主表 + Plant + PCC 绑定（反送 -40kW）。
	meterID, err := CreateDomainNode(nc, root.ID, "variable", map[string]string{"description": "PCC 主表"})
	if err != nil {
		t.Fatal(err)
	}
	if err := siotclient.SendNodePoint(nc, meterID, data.NewPointFloat(data.PointTypeValue, "", -40), true); err != nil {
		t.Fatal(err)
	}
	plantID, err := CreateDomainNode(nc, root.ID, NodeTypePlant, map[string]string{PointTypeAssetCode: "P1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := savePCCFeedback(nc, plantID, decision.PCCFeedback{MeterPointID: meterID, Sign: 1, MaxAgeMS: 15000}); err != nil {
		t.Fatal(err)
	}

	// 调度器 + mock plant dispatch 记录触发。
	called := map[string]int{}
	scheduler := NewEnergyScheduler(nc, func(string) error { return nil })
	scheduler.SetPlantDispatch(func(plantID string) error {
		called[plantID]++
		return nil
	})
	scheduler.tick()
	if called[plantID] == 0 {
		t.Fatal("有 PCC 绑定的 Plant 应被周期调度触发")
	}
}
