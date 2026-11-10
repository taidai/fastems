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

// TestPlantAntiReverseWritesDevice 验证多目标仲裁：单柜规则放电 +60 与站级
// 防逆流充电 -50 同时存在时，仲裁选防逆流（高优先级），功率点位写充电负值。
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
	// 单柜规则：放电 +60（与站级防逆流充电 -50 冲突）。
	rule := decision.CapabilityRule{
		ID: "r", State: decision.RulePublished,
		Conditions: []decision.CapabilityCondition{
			{Ref: decision.CapabilityRef{EnergyUnitID: unitID, Field: decision.FieldRatedPower}, Operator: "gt", Value: 50},
		},
		Actions: []decision.CapabilityAction{{EnergyUnitID: unitID, RequestKW: 60}},
	}
	ruleText, _ := json.Marshal(rule)
	_ = siotclient.SendNodePoint(nc, unitID, data.NewPointString(capabilityRulePoint, "", string(ruleText)), true)

	handler := &Handler{nc: nc}
	// 统一仲裁：防逆流（高优先级）应赢过单柜放电规则。
	if err := handler.RunUnitArbitratedDispatch(unitID); err != nil {
		t.Fatal(err)
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
	// 仲裁路径也必须留下同一笔写设备的追溯记录。
	records := listDispatchRecords(nc, unitID)
	if len(records) != 1 {
		t.Fatalf("仲裁后应有 1 条追溯记录，实测 %d", len(records))
	}
	record := records[0]
	if record.DispatchID == "" || record.RuleID != "r" || record.RequestKW >= 0 || record.DeliveredKW >= 0 || len(record.Dispatches) == 0 {
		t.Fatalf("追溯记录不完整：%+v", record)
	}
	t.Logf("防逆流写设备验证通过：充电 %.1f kW，追溯 %s", set.Value.Num, record.DispatchID)
}
