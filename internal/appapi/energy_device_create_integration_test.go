package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"fastiot/internal/ems/bridge"
)

// TestUserBuiltUnitControlsDevice 验证完整用户路径：自建能源节点 → 挂
// Device（映射功率点位）→ 配已发布规则 → 调度把功率请求写到真实功率点位。
// 这是"能建"到"真正可控"的闭环（delivered>0）。
func TestUserBuiltUnitControlsDevice(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-user-control", DataDir: dataDir, HTTPPort: "9732",
		NatsPort: 9733, NatsServer: "nats://127.0.0.1:9733", AuthToken: data.NewID(),
		DeviceAuth: server.DeviceAuthRequired, CustomUIFS: fstest.MapFS{"index.html": {Data: []byte("easyems")}},
		AppOnly: true, NatsLoopback: true, ProvisioningDisable: true,
		AppHandlerFactory: func(nc *nats.Conn, u api.UserAuthority) http.Handler {
			users = u
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

	handler := New(nc, users, nil)
	identity := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true, Role: "admin"}, Anchors: []string{root.ID}}

	// 通道 + 功率设定点位。
	channelID, err := CreateDomainNode(nc, root.ID, data.NodeTypeModbus, map[string]string{"description": "总线"})
	if err != nil {
		t.Fatal(err)
	}
	powerPointID, err := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{
		"description": "有功功率设定", "id": "1", "address": "10",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 用户自建能源节点。
	unitBody, _ := json.Marshal(map[string]any{"description": "自建柜", "rated_power_kw": 100})
	unitRec := httptest.NewRecorder()
	handler.createEnergyUnit(unitRec, httptest.NewRequest(http.MethodPost, "/api/energy-units", bytes.NewReader(unitBody)), identity)
	if unitRec.Code != http.StatusOK {
		t.Fatalf("create unit: %d %s", unitRec.Code, unitRec.Body.String())
	}
	var unit struct {
		ID string `json:"id"`
	}
	json.Unmarshal(unitRec.Body.Bytes(), &unit)

	// 挂 Device，映射功率点位。
	deviceBody, _ := json.Marshal(map[string]any{"description": "自建PCS", "rated_power_kw": 100, "power_setpoint_id": powerPointID})
	deviceRec := httptest.NewRecorder()
	handler.createEnergyDevice(deviceRec, httptest.NewRequest(http.MethodPost, "/api/energy-units/"+unit.ID+"/devices", bytes.NewReader(deviceBody)), identity, unit.ID)
	if deviceRec.Code != http.StatusOK {
		t.Fatalf("create device: %d %s", deviceRec.Code, deviceRec.Body.String())
	}

	// 已发布规则：rated>50 → 放电 60。
	rule := decision.CapabilityRule{
		ID: "r", State: decision.RulePublished,
		Conditions: []decision.CapabilityCondition{
			{Ref: decision.CapabilityRef{EnergyUnitID: unit.ID, Field: decision.FieldRatedPower}, Operator: "gt", Value: 50},
		},
		Actions: []decision.CapabilityAction{{EnergyUnitID: unit.ID, RequestKW: 60}},
	}
	ruleText, _ := json.Marshal(rule)
	if err := siotclient.SendNodePoint(nc, unit.ID, data.NewPointString(capabilityRulePoint, "", string(ruleText)), true); err != nil {
		t.Fatal(err)
	}

	// 跑调度，功率点位应被写入。
	if _, err := handler.RunCapabilityDispatch(unit.ID); err != nil {
		t.Fatal(err)
	}
	source := bridge.NewNodeSource(nc)
	sets, err := source.ValueSets()
	if err != nil {
		t.Fatal(err)
	}
	set, ok := sets[powerPointID]
	if !ok || set.Value.Num <= 0 {
		t.Fatalf("power point not written, sets=%v", sets)
	}
}
