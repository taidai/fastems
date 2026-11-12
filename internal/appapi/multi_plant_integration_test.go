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

// TestMultiPlantIndependentPCC 验证多站逐 PCC 独立闭环（T19）：PlantA 反送
// -30、PlantB 正常 +30，站级合计为零，但 A 仍独立判定反送并削减，B 独立
// 达标不动作。局部越限不被站级合计掩盖。
func TestMultiPlantIndependentPCC(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-multi-pcc", DataDir: dataDir, HTTPPort: "9750",
		NatsPort: 9751, NatsServer: "nats://127.0.0.1:9751", AuthToken: data.NewID(),
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

	// 建两个 Plant，各有 PCC 主表与 EnergyUnit。
	buildPlant := func(code, meterDesc string, meterKW float64) string {
		plantID, _ := CreateDomainNode(nc, root.ID, NodeTypePlant, map[string]string{PointTypeAssetCode: code})
		meterID, _ := CreateDomainNode(nc, root.ID, "variable", map[string]string{"description": meterDesc})
		_ = siotclient.SendNodePoint(nc, meterID, data.NewPointFloat(data.PointTypeValue, "", meterKW), true)
		_ = savePCCFeedback(nc, plantID, decision.PCCFeedback{MeterPointID: meterID, Sign: 1, MaxAgeMS: 60000})
		unitID, _ := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: code + "-EU", PointTypeRatedPowerKW: "100"})
		capText, _ := EncodeCapability(Capability{Kind: "charge", Direction: "bidir", PowerMaxKW: 100, Allowed: true})
		_ = siotclient.SendNodePoint(nc, unitID, data.NewPointString(PointTypeCapability, "", capText), true)
		return plantID
	}
	plantA := buildPlant("PA", "A主表", -30) // 反送
	plantB := buildPlant("PB", "B主表", 30)  // 正常

	handler := &Handler{nc: nc}

	// PlantA：反送 30，应独立削减（分配充电）。
	allocA, err := handler.RunPlantAntiReverse(plantA)
	if err != nil {
		t.Fatal(err)
	}
	totalA := 0.0
	for _, a := range allocA {
		totalA += a.SetpointKW
	}
	if totalA >= 0 {
		t.Fatalf("PlantA 反送应独立削减（充电），实测合计 %.1f", totalA)
	}

	// PlantB：正常，应独立达标不动作。
	allocB, err := handler.RunPlantAntiReverse(plantB)
	if err != nil {
		t.Fatal(err)
	}
	if len(allocB) != 0 {
		t.Fatalf("PlantB 正常不应调节，实测 %d 条分配", len(allocB))
	}
	t.Logf("多站独立闭环通过：A 削减 %.1f，B 不动作", totalA)
}
