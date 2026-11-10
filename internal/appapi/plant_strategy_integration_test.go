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

// TestPlantPeakValleyStrategy 验证削峰填谷进入站级调度：当前时刻落在谷时段
// 时，按配置功率充电并写入设备；防逆流/需量未触发时不接管。
func TestPlantPeakValleyStrategy(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-peak-valley", DataDir: dataDir, HTTPPort: "9746",
		NatsPort: 9747, NatsServer: "nats://127.0.0.1:9747", AuthToken: data.NewID(),
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

	// 通道 + 功率点位 + Plant + EnergyUnit + Device（映射点位）。
	channelID, _ := CreateDomainNode(nc, root.ID, data.NodeTypeModbus, map[string]string{"description": "总线"})
	powerPointID, _ := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{"description": "功率设定", "id": "1", "address": "10"})
	plantID, _ := CreateDomainNode(nc, root.ID, NodeTypePlant, map[string]string{PointTypeAssetCode: "PV1"})
	unitID, _ := CreateDomainNode(nc, plantID, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: "EU1", PointTypeRatedPowerKW: "100"})
	capText, _ := EncodeCapability(Capability{Kind: "charge", Direction: "bidir", PowerMaxKW: 100, Allowed: true})
	_ = siotclient.SendNodePoint(nc, unitID, data.NewPointString(PointTypeCapability, "", capText), true)
	_, _ = CreateDomainNode(nc, unitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "PCS1", PointTypeRatedPowerKW: "100", PointTypePowerSetpoint: powerPointID,
	})

	// 谷时段覆盖当前分钟。
	now := time.Now()
	minute := now.Hour()*60 + now.Minute()
	strategy := PlantStrategy{PeakValleyPowerKW: 40, Periods: []decision.TariffPeriod{{StartMin: minute, EndMin: minute + 1, Kind: "valley"}}}
	if err := savePlantStrategy(nc, plantID, strategy); err != nil {
		t.Fatal(err)
	}

	handler := &Handler{nc: nc}
	acted, err := handler.RunPlantStrategy(plantID)
	if err != nil {
		t.Fatal(err)
	}
	if !acted {
		t.Fatal("谷时段应接管站级分配")
	}
	source := bridge.NewNodeSource(nc)
	sets, err := source.ValueSets()
	if err != nil {
		t.Fatal(err)
	}
	set, ok := sets[powerPointID]
	if !ok || set.Value.Num >= 0 {
		t.Fatalf("谷时段应写充电负值，实测 %+v", set.Value)
	}
	t.Logf("削峰填谷接管：充电 %.1f kW", set.Value.Num)
}
