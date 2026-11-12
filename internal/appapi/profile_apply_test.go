package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"

	"fastems/internal/ems/profile"
	"fastems/internal/ems/runtime"
	"fastems/internal/emsapi"
)

func TestApplyProfileVerdicts(t *testing.T) {
	registry := profile.Default()
	prof, ok := registry.Get("enjo-pcs")
	if !ok {
		t.Fatal("enjo-pcs profile missing")
	}
	points := make([]unitModelPointView, 0, len(prof.PointSpecs))
	missing := map[string]bool{"电网频率": true, "直流功率": true}
	scaled := map[string]bool{"硬件故障字 1": true, "硬件故障字 2": true}
	for index, spec := range prof.PointSpecs {
		if missing[spec.Name] {
			continue
		}
		scale := spec.Scale
		if scaled[spec.Name] {
			scale = spec.Scale * 10
		}
		points = append(points, unitModelPointView{
			ID: "p-" + string(rune('a'+index)), Name: spec.Name, ServerID: 1, Scale: scale,
			ReadOnly: spec.Name == "开机/关机指令",
		})
	}
	result := applyProfile(1, prof, points, unitModelConfig{})
	if result.Mapped != len(prof.PointSpecs)-len(missing) {
		t.Fatalf("mapped = %d", result.Mapped)
	}
	if result.Scaled != len(scaled) || len(result.Missing) != len(missing) {
		t.Fatalf("scaled = %d, missing = %#v", result.Scaled, result.Missing)
	}
	verdicts := map[string]string{}
	for _, verdict := range result.Points {
		verdicts[verdict.Name] = verdict.Verdict
	}
	if verdicts["硬件故障字 1"] != "scaled" || verdicts["电网频率"] != "missing" || verdicts["电池电压"] != "exact" {
		t.Fatalf("verdicts = %#v", verdicts)
	}
	if len(result.Blocked) != 2 {
		t.Fatalf("blocked = %#v", result.Blocked)
	}
	for _, blocked := range result.Blocked {
		if blocked == "" {
			t.Fatal("empty blocked message")
		}
	}
}

func TestApplyProfilesPersistsModels(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-profile-apply", DataDir: dataDir, HTTPPort: "9860",
		NatsPort: 9861, NatsServer: "nats://127.0.0.1:9861", AuthToken: data.NewID(),
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
	deadline := time.Now().Add(10 * time.Second)
	var root data.NodeEdge
	for {
		root, err = siotclient.GetRootNode(nc)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("store not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}

	channelID, err := CreateDomainNode(nc, root.ID, data.NodeTypeModbus, map[string]string{"description": "测试通道"})
	if err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"开机/关机指令", "有功功率设定", "交流输出总有功功率"} {
		ioID, err := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{"description": name})
		if err != nil {
			t.Fatal(err)
		}
		if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(ioID), data.Points{
			data.NewPointInt(data.PointTypeID, "0", 1),
			data.NewPointInt(data.PointTypeAddress, "0", int64(index+1)),
			data.NewPointFloat(data.PointTypeScale, "0", 1),
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	handler := New(nc, users, nil)
	handler.SetEMS(emsapi.New(runtime.NewService(nil, 15*time.Second, 5*time.Second, 100), 15*time.Second))
	admin := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true}, Anchors: []string{root.ID}}
	viewer := Identity{UserID: "viewer", Policy: AccessPolicy{Role: "viewer"}, Anchors: []string{root.ID}}

	// 先保存一个带业务节点绑定的旧型号，换厂商后该绑定必须保留。
	existing := unitModelConfig{Name: "旧型号", ProfileID: "modbus-generic", BusinessNodeID: "biz-1", PointMap: map[string]string{}}
	channelNode, err := siotclient.GetNodes(nc, "all", channelID, "", false)
	if err != nil || len(channelNode) == 0 {
		t.Fatal(err)
	}
	if err := handler.saveUnitModel(channelNode[0], 1, existing, handler.unitPoints(channelID)); err != nil {
		t.Fatal(err)
	}

	request := func(identity Identity, dryRun bool) *httptest.ResponseRecorder {
		body := `{"profile_id":"enjo-pcs","servers":[1],"dry_run":` + map[bool]string{true: "true", false: "false"}[dryRun] + `}`
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/modbus/"+channelID+"/units/apply", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		handler.applyProfilesHandler(recorder, req, identity, channelID)
		return recorder
	}

	if status := request(viewer, true).Code; status != 403 {
		t.Fatalf("viewer apply = %d, want 403", status)
	}
	recorder := request(admin, false)
	if recorder.Code != 200 {
		t.Fatalf("apply = %d %s", recorder.Code, recorder.Body.String())
	}

	channel, err := siotclient.GetNodes(nc, "all", channelID, "", false)
	if err != nil || len(channel) == 0 {
		t.Fatal(err)
	}
	model, ok := unitModelFor(channel[0], 1)
	if !ok || model.ProfileID != "enjo-pcs" || len(model.PointMap) != 3 {
		t.Fatalf("model = %#v", model)
	}
	if model.BusinessNodeID != "biz-1" {
		t.Fatalf("business node binding lost: %#v", model)
	}
}
