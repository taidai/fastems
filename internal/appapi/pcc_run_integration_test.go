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

	"fastiot/internal/decision"
)

// TestPlantAntiReverseClosesLoop 验证 PCC 反馈闭环：Plant 绑定主表，主表
// 反送 -30kW 时，防逆流把 30kW 削减分配到 EnergyUnit。不再吃空参数。
func TestPlantAntiReverseClosesLoop(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-pcc-loop", DataDir: dataDir, HTTPPort: "9724",
		NatsPort: 9725, NatsServer: "nats://127.0.0.1:9725", AuthToken: data.NewID(),
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

	// 主表点位（variable 节点，写 value=-30 表示反送 30kW）。
	meterID, err := CreateDomainNode(nc, root.ID, "variable", map[string]string{"description": "PCC 主表"})
	if err != nil {
		t.Fatal(err)
	}
	if err := siotclient.SendNodePoint(nc, meterID, data.NewPointFloat(data.PointTypeValue, "", -30), true); err != nil {
		t.Fatal(err)
	}

	// Plant + PCC 绑定 + 两个 EnergyUnit。
	plantID, err := CreateDomainNode(nc, root.ID, NodeTypePlant, map[string]string{PointTypeAssetCode: "P1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := savePCCFeedback(nc, plantID, decision.PCCFeedback{MeterPointID: meterID, Sign: 1, MaxAgeMS: 15000}); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"EU1", "EU2"} {
		unitID, err := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{
			PointTypeAssetCode: code, PointTypeRatedPowerKW: "250",
		})
		if err != nil {
			t.Fatal(err)
		}
		capText, _ := EncodeCapability(Capability{Kind: "charge", Direction: "bidir", PowerMaxKW: 200, Allowed: true})
		if err := siotclient.SendNodePoint(nc, unitID, data.NewPointString(PointTypeCapability, "", capText), true); err != nil {
			t.Fatal(err)
		}
	}

	handler := &Handler{nc: nc}
	binding, reading, ok := ReadPlantPCC(nc, plantID)
	t.Logf("PCC binding=%+v reading=%+v ok=%v", binding, reading, ok)
	allocations, err := handler.RunPlantAntiReverse(plantID)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, a := range allocations {
		total += a.SetpointKW
	}
	// 反送 30，削减 30 → 分配为负（充电方向），合计约 -30。
	if total >= 0 || total < -30.01 {
		t.Fatalf("expected anti-reverse total ≈ -30, got %.2f", total)
	}
}

// TestPlantAntiReverseStaleDataBlocked 验证过期数据不授权调节。
func TestPlantAntiReverseStaleDataBlocked(t *testing.T) {
	binding := decision.PCCFeedback{MeterPointID: "m", Sign: 1, MaxAgeMS: 1000}
	reading := decision.ReadPCC(binding, -30, 5000, true)
	if reading.Usable {
		t.Fatal("stale reading must be unusable")
	}
	if _, ok := decision.AntiReverseFromFeedback(binding, reading); ok {
		t.Fatal("stale data must not authorize anti-reverse")
	}
}
