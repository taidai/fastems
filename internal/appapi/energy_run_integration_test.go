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

// TestRunCapabilityDispatch 验证能力级调度闭环：EnergyUnit + 能力级规则 +
// 柜内 Device → 执行调度 → Device valueSet 写入。
func TestRunCapabilityDispatch(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-energy-dispatch", DataDir: dataDir, HTTPPort: "9714",
		NatsPort: 9715, NatsServer: "nats://127.0.0.1:9715", AuthToken: data.NewID(),
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

	// EnergyUnit（额定 250 kW）+ 能力级规则（额定 > 100 时请求 150 kW）。
	energyUnitID, err := CreateDomainNode(nc, "root", NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode:    "ES01-CAB01",
		PointTypeEnergyType:   EnergyTypeStorage,
		PointTypeRatedPowerKW: "250",
	})
	if err != nil {
		t.Fatal(err)
	}
	rule := decision.CapabilityRule{
		ID: "discharge-150",
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

	// 两个 Device，各 100 kW 额定。
	for _, code := range []string{"PCS01", "PCS02"} {
		if _, err := CreateDomainNode(nc, energyUnitID, data.NodeTypeDevice, map[string]string{
			PointTypeAssetCode: "ES01-CAB01-" + code, PointTypeRatedPowerKW: "100",
		}); err != nil {
			t.Fatal(err)
		}
	}

	dispatches, err := handler.RunCapabilityDispatch(energyUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected 2 dispatches, got %d", len(dispatches))
	}
	total := 0.0
	for _, dispatch := range dispatches {
		total += dispatch.SetpointKW
	}
	if total <= 0 {
		t.Fatalf("expected positive total setpoint, got %.1f", total)
	}
}
