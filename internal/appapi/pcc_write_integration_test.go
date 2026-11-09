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
	"easyems/internal/ems/bridge"
)

// TestPlantAntiReverseWritesDevice 隔离验证防逆流真正写设备：主表反送时，
// 储能柜的功率点位被写入充电（负值）。无单柜规则干扰。
func TestPlantAntiReverseWritesDevice(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-pcc-write", DataDir: dataDir, HTTPPort: "9738",
		NatsPort: 9739, NatsServer: "nats://127.0.0.1:9739", AuthToken: data.NewID(),
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

	// 通道 + 功率点位；Plant + PCC（反送 -50）；EnergyUnit + Device（映射功率点位）。
	channelID, _ := CreateDomainNode(nc, root.ID, data.NodeTypeModbus, map[string]string{"description": "总线"})
	powerPointID, _ := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{"description": "功率设定", "id": "1", "address": "10"})
	plantID, _ := CreateDomainNode(nc, root.ID, NodeTypePlant, map[string]string{PointTypeAssetCode: "P1"})
	meterID, _ := CreateDomainNode(nc, root.ID, "variable", map[string]string{"description": "PCC主表"})
	if err := siotclient.SendNodePoint(nc, meterID, data.NewPointFloat(data.PointTypeValue, "", -50), true); err != nil {
		t.Fatal(err)
	}
	if err := savePCCFeedback(nc, plantID, decision.PCCFeedback{MeterPointID: meterID, Sign: 1, MaxAgeMS: 60000}); err != nil {
		t.Fatal(err)
	}
	unitID, _ := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: "EU1", PointTypeRatedPowerKW: "100"})
	capText, _ := EncodeCapability(Capability{Kind: "charge", Direction: "bidir", PowerMaxKW: 100, Allowed: true})
	_ = siotclient.SendNodePoint(nc, unitID, data.NewPointString(PointTypeCapability, "", capText), true)
	_, _ = CreateDomainNode(nc, unitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "PCS1", PointTypeRatedPowerKW: "100", PointTypePowerSetpoint: powerPointID,
	})

	handler := &Handler{nc: nc}
	allocations, err := handler.RunPlantAntiReverse(plantID)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, a := range allocations {
		total += a.SetpointKW
	}
	if total >= 0 {
		t.Fatalf("防逆流应分配充电（负值），实测合计 %.1f", total)
	}
	// 验证功率点位被写入负值（充电削减）。
	source := bridge.NewNodeSource(nc)
	sets, err := source.ValueSets()
	if err != nil {
		t.Fatal(err)
	}
	set, ok := sets[powerPointID]
	if !ok {
		t.Fatalf("功率点位未写入 valueSet")
	}
	if set.Value.Num >= 0 {
		t.Fatalf("防逆流应写充电负值，实测 %.1f", set.Value.Num)
	}
	t.Logf("防逆流写设备验证通过：充电 %.1f kW", set.Value.Num)
}
