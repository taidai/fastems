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

// TestFaultInterlocksDispatch 验证 CTL-01 互锁：EnergyUnit 置为 fault 后，
// 即使规则已发布，RunCapabilityDispatch 也必须拒绝（故障禁止自动调度）。
func TestFaultInterlocksDispatch(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-fault-interlock", DataDir: dataDir, HTTPPort: "9726",
		NatsPort: 9727, NatsServer: "nats://127.0.0.1:9727", AuthToken: data.NewID(),
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
		PointTypeAssetCode: "FAULT-CAB", PointTypeRatedPowerKW: "250",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 已发布规则。
	rule := decision.CapabilityRule{
		ID: "r", State: decision.RulePublished,
		Conditions: []decision.CapabilityCondition{
			{Ref: decision.CapabilityRef{EnergyUnitID: energyUnitID, Field: decision.FieldRatedPower}, Operator: "gt", Value: 100},
		},
		Actions: []decision.CapabilityAction{{EnergyUnitID: energyUnitID, RequestKW: 150}},
	}
	ruleText, _ := json.Marshal(rule)
	if err := siotclient.SendNodePoint(nc, energyUnitID, data.NewPointString(capabilityRulePoint, "", string(ruleText)), true); err != nil {
		t.Fatal(err)
	}
	// 柜内一台 Device。
	if _, err := CreateDomainNode(nc, energyUnitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "FAULT-PCS", PointTypeRatedPowerKW: "100",
	}); err != nil {
		t.Fatal(err)
	}

	handler := &Handler{nc: nc}

	// 默认（无状态点=running）可调度。
	if _, err := handler.RunCapabilityDispatch(energyUnitID); err != nil {
		t.Fatalf("running unit must dispatch: %v", err)
	}

	// 置为 fault。
	if err := SetCabinetState(nc, energyUnitID, decision.CabinetFault); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.RunCapabilityDispatch(energyUnitID); err == nil {
		t.Fatal("fault unit must NOT dispatch (interlock)")
	}

	// 人工确认解除故障 → standby，仍不可自动运行（需重新启动）。
	nodes, _ := siotclient.GetNodes(nc, "all", energyUnitID, "", false)
	next, ok, err := ApplyCabinetEvent(nc, nodes[0], decision.EvFaultCleared)
	if err != nil || !ok || next != decision.CabinetStandby {
		t.Fatalf("fault clear must go to standby, got %v ok=%v err=%v", next, ok, err)
	}
	if _, err := handler.RunCapabilityDispatch(energyUnitID); err == nil {
		t.Fatal("standby after fault-clear must NOT auto-dispatch")
	}
}
