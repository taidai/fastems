package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"

	"fastems/internal/decision"
)

// TestMultiTenantIsolation 验证多租户隔离：用户 A 的 anchor 是站点 A，
// 不能访问站点 B 的资源（allows 跨站拒绝）。
func TestMultiTenantIsolation(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-tenant", DataDir: dataDir, HTTPPort: "9744",
		NatsPort: 9745, NatsServer: "nats://127.0.0.1:9745", AuthToken: data.NewID(),
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

	// 两个站点资源。
	siteA, _ := CreateDomainNode(nc, root.ID, NodeTypeSite, map[string]string{PointTypeAssetCode: "SITE-A"})
	siteB, _ := CreateDomainNode(nc, root.ID, NodeTypeSite, map[string]string{PointTypeAssetCode: "SITE-B"})
	unitB, _ := CreateDomainNode(nc, siteB, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: "B-CAB"})

	handler := New(nc, users, nil)
	identityA := Identity{UserID: "userA", Anchors: []string{siteA}}

	// A 可访问自己站点，不能访问 B 站点及其下资源。
	if !handler.allows(identityA, siteA) {
		t.Fatal("A 应能访问自己站点")
	}
	if handler.allows(identityA, siteB) {
		t.Fatal("A 不应能访问 B 站点（跨站拒绝）")
	}
	if handler.allows(identityA, unitB) {
		t.Fatal("A 不应能访问 B 站点下的能源节点（跨站拒绝）")
	}
	t.Log("多租户跨站隔离验证通过")
}

// TestTreeScopeHTTPIsolation exercises the real HTTP API with an implementer
// anchored at Site A. Site B is created first so the default-parent path must
// deliberately skip its otherwise globally first Plant.
func TestTreeScopeHTTPIsolation(t *testing.T) {
	const password = "easyems-scope-admin"
	t.Setenv("FASTEMS_ADMIN_PASSWORD", password)
	dataDir := filepath.Join(t.TempDir(), "data")
	ready := new(atomic.Bool)
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-tree-scope-http", DataDir: dataDir, HTTPPort: "9810",
		NatsPort: 9811, NatsServer: "nats://127.0.0.1:9811", AuthToken: data.NewID(),
		DeviceAuth: server.DeviceAuthRequired, CustomUIFS: fstest.MapFS{"index.html": {Data: []byte("easyems")}},
		AppOnly: true, NatsLoopback: true, ProvisioningDisable: true,
		AppHandlerFactory: func(nc *nats.Conn, u api.UserAuthority) http.Handler {
			users = u
			return New(nc, u, ready)
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
	if err := Bootstrap(nc, dataDir); err != nil {
		t.Fatal(err)
	}
	ready.Store(true)
	base := "http://127.0.0.1:9810/api"
	waitFor(t, func() bool {
		response, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, "tree-scope HTTP readiness")
	adminToken := loginAPI(t, base, "admin", password)
	root, err := siotclient.GetRootNode(nc)
	if err != nil {
		t.Fatal(err)
	}

	create := func(parent, nodeType string, fields map[string]string) string {
		t.Helper()
		id, err := CreateDomainNode(nc, parent, nodeType, fields)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	createUnit := func(plantID, code string) string {
		t.Helper()
		id := create(plantID, NodeTypeEnergyUnit, map[string]string{
			PointTypeAssetCode: code, PointTypeRatedPowerKW: "60",
		})
		capability, _ := EncodeCapability(Capability{Kind: "discharge", Direction: "bidir", PowerMaxKW: 60, Allowed: true})
		if err := siotclient.SendNodePoint(nc, id, data.NewPointString(PointTypeCapability, "", capability), true); err != nil {
			t.Fatal(err)
		}
		return id
	}

	siteB := create(root.ID, NodeTypeSite, map[string]string{PointTypeAssetCode: "SITE-B"})
	plantB := create(siteB, NodeTypePlant, map[string]string{PointTypeAssetCode: "PLANT-B"})
	meterB := create(siteB, data.NodeTypeVariable, map[string]string{"description": "B PCC"})
	pointB := create(siteB, data.NodeTypeModbusIO, map[string]string{"description": "B power"})
	unitB := createUnit(plantB, "UNIT-B")
	create(unitB, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "PCS-B", PointTypeRatedPowerKW: "60", PointTypePowerSetpoint: pointB,
	})

	siteA := create(root.ID, NodeTypeSite, map[string]string{PointTypeAssetCode: "SITE-A"})
	plantA := create(siteA, NodeTypePlant, map[string]string{PointTypeAssetCode: "PLANT-A"})
	meterA := create(siteA, data.NodeTypeVariable, map[string]string{"description": "A PCC"})
	pointA := create(siteA, data.NodeTypeModbusIO, map[string]string{"description": "A power"})
	unitA := createUnit(plantA, "UNIT-A")
	create(unitA, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode: "PCS-A", PointTypeRatedPowerKW: "60", PointTypePowerSetpoint: pointA,
	})

	originalRule := decision.CapabilityRule{ID: "b-original", State: decision.RuleDraft}
	ruleText, _ := json.Marshal(originalRule)
	if err := siotclient.SendNodePoint(nc, unitB, data.NewPointString(capabilityRulePoint, "", string(ruleText)), true); err != nil {
		t.Fatal(err)
	}
	originalMembers := []decision.EnergyMember{{EnergyUnitID: unitB}}
	memberText, _ := json.Marshal(originalMembers)
	if err := siotclient.SendNodePoint(nc, unitB, data.NewPointString(capabilityAllocatorPoint, "", string(memberText)), true); err != nil {
		t.Fatal(err)
	}
	if err := savePCCFeedback(nc, plantB, decision.PCCFeedback{MeterPointID: meterB, Sign: 1, MaxAgeMS: 60000, UpperLimitKW: 80}); err != nil {
		t.Fatal(err)
	}
	if err := savePlantStrategy(nc, plantB, PlantStrategy{PeakValleyPowerKW: 15}); err != nil {
		t.Fatal(err)
	}
	if err := appendDispatchRecord(nc, unitB, decision.DispatchRecord{DispatchID: "b-original"}); err != nil {
		t.Fatal(err)
	}

	createGroup := func(parent, description string, members []string) string {
		t.Helper()
		id := create(parent, NodeTypeResourceGroup, map[string]string{
			"description": description, PointTypeSharedLimitKW: "100",
		})
		encoded, _ := json.Marshal(members)
		if err := siotclient.SendNodePoint(nc, id, data.NewPointString(PointTypeGroupMembers, "", string(encoded)), true); err != nil {
			t.Fatal(err)
		}
		return id
	}
	groupA := createGroup(siteA, "A group", []string{unitA})
	groupB := createGroup(siteB, "B group", []string{unitB})
	mixedGroup := createGroup(siteA, "mixed group", []string{unitB})

	status, body := call(t, http.MethodPost, base+"/users", adminToken, map[string]any{
		"parent": siteA, "email": "site-a@example.com", "password": "site-a-password", "role": "implementer",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create scoped user: %d %s", status, body)
	}
	tokenA := ""
	waitFor(t, func() bool {
		tokenA = loginAPIOptional(base, "site-a@example.com", "site-a-password")
		return tokenA != ""
	}, "Site A login")
	status, body = call(t, http.MethodPost, base+"/users", adminToken, map[string]any{
		"parent": siteA, "email": "site-a-write-only@example.com", "password": "site-a-write-password",
		"role": "implementer", "actions": []string{ActionWriteValue},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create write-only user: %d %s", status, body)
	}
	writeOnlyToken := ""
	waitFor(t, func() bool {
		writeOnlyToken = loginAPIOptional(base, "site-a-write-only@example.com", "site-a-write-password")
		return writeOnlyToken != ""
	}, "Site A write-only login")

	listIDs := func(token, path, key string) []string {
		t.Helper()
		var response map[string]json.RawMessage
		status, body := call(t, http.MethodGet, base+path, token, nil, &response)
		if status != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, status, body)
		}
		var items []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(response[key], &items); err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		return ids
	}
	contains := func(ids []string, id string) bool {
		for _, candidate := range ids {
			if candidate == id {
				return true
			}
		}
		return false
	}
	for _, test := range []struct {
		path, key, visible, hidden string
	}{
		{"/energy-units", "units", unitA, unitB},
		{"/plants", "plants", plantA, plantB},
		{"/resource-groups", "groups", groupA, groupB},
	} {
		ids := listIDs(tokenA, test.path, test.key)
		if !contains(ids, test.visible) || contains(ids, test.hidden) {
			t.Fatalf("scoped %s filtering = %v", test.key, ids)
		}
		adminIDs := listIDs(adminToken, test.path, test.key)
		if !contains(adminIDs, test.visible) || !contains(adminIDs, test.hidden) {
			t.Fatalf("admin %s visibility = %v", test.key, adminIDs)
		}
	}
	if ids := listIDs(tokenA, "/resource-groups", "groups"); contains(ids, mixedGroup) {
		t.Fatalf("mixed-scope group leaked through list: %v", ids)
	}

	for _, path := range []string{
		"/energy-units/" + unitB + "/rule",
		"/energy-units/" + unitB + "/allocator",
		"/energy-units/" + unitB + "/devices",
		"/energy-units/" + unitB + "/dispatch-records",
		"/plants/" + plantB + "/pcc",
		"/plants/" + plantB + "/strategy",
	} {
		status, body := call(t, http.MethodGet, base+path, tokenA, nil, nil)
		if status != http.StatusForbidden {
			t.Fatalf("cross-site GET %s = %d %s", path, status, body)
		}
	}

	for _, test := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/energy-units/" + unitB + "/rule", decision.CapabilityRule{ID: "b-tampered"}},
		{http.MethodPost, "/energy-units/" + unitB + "/rule/publish", map[string]any{}},
		{http.MethodPost, "/energy-units/" + unitB + "/rule/simulate", map[string]any{"rule": decision.CapabilityRule{ID: "simulation"}}},
		{http.MethodPut, "/energy-units/" + unitB + "/allocator", map[string]any{"members": []string{}}},
		{http.MethodPost, "/energy-units/" + unitB + "/devices", map[string]any{"description": "intruder", "rated_power_kw": 10, "power_setpoint_id": pointB}},
		{http.MethodPost, "/energy-units", map[string]any{"plant_id": plantB, "description": "intruder", "rated_power_kw": 10}},
		{http.MethodPut, "/plants/" + plantB + "/pcc", decision.PCCFeedback{MeterPointID: meterA, Sign: 1, MaxAgeMS: 1000}},
		{http.MethodPut, "/plants/" + plantB + "/strategy", PlantStrategy{PeakValleyPowerKW: 99}},
		{http.MethodPost, "/resource-groups", map[string]any{"description": "intruder group", "members": []string{unitA, unitB}}},
		{http.MethodPost, "/resource-groups/" + groupB + "/dispatch", map[string]any{"request_kw": 20}},
		{http.MethodPost, "/resource-groups/" + mixedGroup + "/dispatch", map[string]any{"request_kw": 20}},
		{http.MethodPost, "/energy-units/" + unitA + "/devices", map[string]any{"description": "bad reference", "rated_power_kw": 10, "power_setpoint_id": pointB}},
	} {
		status, body := call(t, test.method, base+test.path, tokenA, test.body, nil)
		if status != http.StatusForbidden {
			t.Fatalf("cross-site %s %s = %d %s", test.method, test.path, status, body)
		}
	}

	assertSiteBUnchanged := func() {
		t.Helper()
		nodes, err := siotclient.GetNodes(nc, "all", unitB, "", false)
		if err != nil || len(nodes) != 1 {
			t.Fatalf("read Site B unit: %v", err)
		}
		if rule, ok := loadCapabilityRule(nodes[0]); !ok || rule.ID != originalRule.ID || rule.State != originalRule.State {
			t.Fatalf("Site B rule changed: %+v", rule)
		}
		if members, ok := loadEnergyAllocator(nodes[0]); !ok || len(members) != 1 || members[0].EnergyUnitID != unitB {
			t.Fatalf("Site B allocator changed: %+v", members)
		}
		if records := listDispatchRecords(nc, unitB); len(records) != 1 || records[0].DispatchID != "b-original" {
			t.Fatalf("Site B dispatch records changed: %+v", records)
		}
		devices, err := siotclient.GetNodes(nc, unitB, "all", data.NodeTypeDevice, false)
		if err != nil || len(devices) != 1 {
			t.Fatalf("Site B devices changed: %v %d", err, len(devices))
		}
		units, err := siotclient.GetNodes(nc, plantB, "all", NodeTypeEnergyUnit, false)
		if err != nil || len(units) != 1 {
			t.Fatalf("Site B units changed: %v %d", err, len(units))
		}
		plants, err := siotclient.GetNodes(nc, "all", plantB, "", false)
		if err != nil || len(plants) != 1 {
			t.Fatalf("read Site B plant: %v", err)
		}
		if binding, ok := loadPCCFeedback(plants[0]); !ok || binding.MeterPointID != meterB || binding.UpperLimitKW != 80 {
			t.Fatalf("Site B PCC changed: %+v", binding)
		}
		if strategy, ok := loadPlantStrategy(plants[0]); !ok || strategy.PeakValleyPowerKW != 15 {
			t.Fatalf("Site B strategy changed: %+v", strategy)
		}
		points, err := siotclient.GetNodes(nc, "all", pointB, "", false)
		if err != nil || len(points) != 1 {
			t.Fatalf("read Site B power point: %v", err)
		}
		if _, found := points[0].Points.Find(data.PointTypeValueSet, ""); found {
			t.Fatal("denied dispatch wrote Site B power setpoint")
		}
	}
	assertSiteBUnchanged()
	groups, err := siotclient.GetNodes(nc, "all", "all", NodeTypeResourceGroup, false)
	if err != nil || len(groups) != 3 {
		t.Fatalf("denied group creation changed groups: %v %d", err, len(groups))
	}
	devicesA, err := siotclient.GetNodes(nc, unitA, "all", data.NodeTypeDevice, false)
	if err != nil || len(devicesA) != 1 {
		t.Fatalf("denied device reference created a device: %v %d", err, len(devicesA))
	}

	var defaultUnit struct {
		ID      string `json:"id"`
		PlantID string `json:"plant_id"`
	}
	status, body = call(t, http.MethodPost, base+"/energy-units", tokenA, map[string]any{
		"description": "A default unit", "rated_power_kw": 25,
	}, &defaultUnit)
	if status != http.StatusOK || defaultUnit.PlantID != plantA {
		t.Fatalf("visible default plant = %d %s %+v", status, body, defaultUnit)
	}

	var createdGroup struct {
		ID string `json:"id"`
	}
	status, body = call(t, http.MethodPost, base+"/resource-groups", tokenA, map[string]any{
		"description": "A created group", "shared_limit_kw": 50, "members": []string{unitA},
	}, &createdGroup)
	if status != http.StatusOK {
		t.Fatalf("create visible group: %d %s", status, body)
	}
	createdNodes, err := siotclient.GetNodes(nc, "all", createdGroup.ID, "", false)
	if err != nil || len(createdNodes) != 1 {
		t.Fatalf("read created group: %v", err)
	}
	parent := createdNodes[0].Parent
	if parent == root.ID || (parent != siteA && !users.IsUnder(parent, siteA)) {
		t.Fatalf("scoped group has invisible parent %q", parent)
	}

	status, body = call(t, http.MethodPost, base+"/resource-groups/"+groupA+"/dispatch", writeOnlyToken, map[string]any{"request_kw": 20}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("write_value without command authorized dispatch: %d %s", status, body)
	}
	pointsA, err := siotclient.GetNodes(nc, "all", pointA, "", false)
	if err != nil || len(pointsA) != 1 {
		t.Fatalf("read Site A power point: %v", err)
	}
	if _, found := pointsA[0].Points.Find(data.PointTypeValueSet, ""); found {
		t.Fatal("command-denied dispatch wrote Site A power setpoint")
	}

	status, body = call(t, http.MethodPost, base+"/resource-groups/"+groupA+"/dispatch", tokenA, map[string]any{"request_kw": 20}, nil)
	if status != http.StatusOK {
		t.Fatalf("visible group dispatch: %d %s", status, body)
	}
	waitFor(t, func() bool { return valueOf(t, nc, pointA) > 0 }, "Site A dispatch write")
	assertSiteBUnchanged()
}
