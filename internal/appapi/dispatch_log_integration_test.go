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
)

// TestDispatchRecordTraceable 验证 OBS-01 追溯：一次调度生成带 dispatchID
// 的记录，能力快照/分配/设备设定齐全，可按 dispatchID 复盘（T18）。
func TestDispatchRecordTraceable(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-trace", DataDir: dataDir, HTTPPort: "9728",
		NatsPort: 9729, NatsServer: "nats://127.0.0.1:9729", AuthToken: data.NewID(),
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

	energyUnitID, err := CreateDomainNode(nc, root.ID, NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode: "TRACE-CAB", PointTypeRatedPowerKW: "250",
	})
	if err != nil {
		t.Fatal(err)
	}
	rule := decision.CapabilityRule{
		ID: "trace-rule", State: decision.RulePublished,
		Conditions: []decision.CapabilityCondition{
			{Ref: decision.CapabilityRef{EnergyUnitID: energyUnitID, Field: decision.FieldRatedPower}, Operator: "gt", Value: 100},
		},
		Actions: []decision.CapabilityAction{{EnergyUnitID: energyUnitID, RequestKW: 150}},
	}
	ruleText, _ := json.Marshal(rule)
	if err := siotclient.SendNodePoint(nc, energyUnitID, data.NewPointString(capabilityRulePoint, "", string(ruleText)), true); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateDomainNode(nc, energyUnitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "TRACE-PCS", PointTypeRatedPowerKW: "100",
	}); err != nil {
		t.Fatal(err)
	}

	handler := &Handler{nc: nc}
	if _, err := handler.RunCapabilityDispatch(energyUnitID); err != nil {
		t.Fatal(err)
	}

	records := listDispatchRecords(nc, energyUnitID)
	if len(records) != 1 {
		t.Fatalf("expected 1 dispatch record, got %d", len(records))
	}
	record := records[0]
	if record.DispatchID == "" {
		t.Fatal("record must carry a dispatchID")
	}
	if record.RuleID != "trace-rule" || record.RequestKW != 150 {
		t.Fatalf("record rule/request wrong: %+v", record)
	}
	if len(record.Dispatches) != 1 {
		t.Fatalf("record must capture device dispatch, got %d", len(record.Dispatches))
	}
	if record.Capability[energyUnitID][decision.FieldRatedPower] != 250 {
		t.Fatal("record must capture capability snapshot")
	}

	// 按 dispatchID 复盘。
	found, ok := GetDispatchRecord(nc, energyUnitID, record.DispatchID)
	if !ok || found.DispatchID != record.DispatchID {
		t.Fatal("must be able to replay by dispatchID")
	}
}
