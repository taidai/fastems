package appapi

import (
	"context"
	"encoding/json"
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

// TestCapabilityDispatchWritesPowerPoint 验证能力级调度把 valueSet 写到
// Device 的功率设定点位（modbusIo 节点），bridge 的 NodeSource 能读到，
// 从而触发写设备。这是「Device→功率点位映射」+ 模拟器重建的关键链路。
func TestCapabilityDispatchWritesPowerPoint(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-energy-bridge", DataDir: dataDir, HTTPPort: "9716",
		NatsPort: 9717, NatsServer: "nats://127.0.0.1:9717", AuthToken: data.NewID(),
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

	// modbus 通道 + 功率设定 modbusIo 点位。
	channelID, err := CreateDomainNode(nc, "root", data.NodeTypeModbus, map[string]string{"description": "验收总线"})
	if err != nil {
		t.Fatal(err)
	}
	powerPointID, err := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{
		"description": "有功功率设定", "id": "1", "address": "10",
	})
	if err != nil {
		t.Fatal(err)
	}

	// EnergyUnit + Device（powerSetpoint 指向功率点位）+ 能力级规则。
	energyUnitID, err := CreateDomainNode(nc, "root", NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode: "ES01-CAB01", PointTypeRatedPowerKW: "250",
	})
	if err != nil {
		t.Fatal(err)
	}
	deviceID, err := CreateDomainNode(nc, energyUnitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "ES01-CAB01-PCS01", PointTypeRatedPowerKW: "100",
		PointTypePowerSetpoint: powerPointID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule := decision.CapabilityRule{
		ID:    "discharge-150",
		State: decision.RulePublished,
		Conditions: []decision.CapabilityCondition{
			{Ref: decision.CapabilityRef{EnergyUnitID: energyUnitID, Field: decision.FieldRatedPower}, Operator: "gt", Value: 100},
		},
		Actions: []decision.CapabilityAction{{EnergyUnitID: energyUnitID, RequestKW: 150}},
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		t.Fatal(err)
	}
	if err := siotclient.SendNodePoint(nc, energyUnitID, data.NewPointString(capabilityRulePoint, "", string(encoded)), true); err != nil {
		t.Fatal(err)
	}

	if _, err := handler.RunCapabilityDispatch(energyUnitID); err != nil {
		t.Fatal(err)
	}

	// bridge 的 NodeSource 应能读到功率点位的 valueSet。
	source := bridge.NewNodeSource(nc)
	sets, err := source.ValueSets()
	if err != nil {
		t.Fatal(err)
	}
	set, ok := sets[powerPointID]
	if !ok {
		t.Fatalf("power point %s has no valueSet; sets=%v", powerPointID, sets)
	}
	if set.Value.Num <= 0 {
		t.Fatalf("power point valueSet = %v, want positive setpoint", set.Value)
	}
	_ = deviceID
}
