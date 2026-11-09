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

// TestRichCapabilityFields 验证通用型能力模型的丰富字段：光伏的可发/限发/
// 预测出力、充电桩的最大负载/可削减/在线率都能被读取，而非只有方向约束。
func TestRichCapabilityFields(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-rich-cap", DataDir: dataDir, HTTPPort: "9742",
		NatsPort: 9743, NatsServer: "nats://127.0.0.1:9743", AuthToken: data.NewID(),
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

	// 光伏节点：当前发电 35、可发 80、限发 50、预测 65。
	pvID, _ := CreateDomainNode(nc, root.ID, NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode: "PV1", PointTypeEnergyType: EnergyTypePV, PointTypeRatedPowerKW: "100",
		PointTypeActualPower: "35", PointTypePVAvailableKW: "80", PointTypePVLimitKW: "50", PointTypePVForecastKW: "65",
	})
	// 充电桩节点：当前负载 30、最大负载 60、可削减 20、在线率 0.9。
	evID, _ := CreateDomainNode(nc, root.ID, NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode: "EV1", PointTypeEnergyType: EnergyTypeEV, PointTypeRatedPowerKW: "60",
		PointTypeActualPower: "30", PointTypeEVMaxLoadKW: "60", PointTypeEVCurtailableKW: "20", PointTypeEVOnlineRatio: "0.9",
	})

	values, err := LoadCapabilityValues(nc, []string{pvID, evID})
	if err != nil {
		t.Fatal(err)
	}
	pv := values[pvID]
	if pv[decision.FieldPVAvailableKW] != 80 || pv[decision.FieldPVLimitKW] != 50 || pv[decision.FieldPVForecastKW] != 65 {
		t.Fatalf("光伏丰富字段读取错误: %+v", pv)
	}
	if pv[decision.FieldActualPower] != 35 {
		t.Fatalf("光伏当前发电应为 35，实测 %.1f", pv[decision.FieldActualPower])
	}
	ev := values[evID]
	if ev[decision.FieldEVMaxLoadKW] != 60 || ev[decision.FieldEVCurtailableKW] != 20 || ev[decision.FieldEVOnlineRatio] != 0.9 {
		t.Fatalf("充电桩丰富字段读取错误: %+v", ev)
	}
	t.Logf("通用型能力模型丰富字段读取通过：光伏可发80/限发50/预测65，充电桩最大60/可削减20/在线率0.9")
}
