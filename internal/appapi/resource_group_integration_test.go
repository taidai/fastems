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
)

// TestGroupDispatchRespectsSharedLimit 验证组共享约束落到运行时：两台柜
// 各 60kW、组共享上限 70kW，请求 100kW 时分配合计≤70、缺口 30。
func TestGroupDispatchRespectsSharedLimit(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-group", DataDir: dataDir, HTTPPort: "9734",
		NatsPort: 9735, NatsServer: "nats://127.0.0.1:9735", AuthToken: data.NewID(),
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

	// 两个 EnergyUnit，各 60kW 可用。
	var ids []string
	for _, code := range []string{"GA", "GB"} {
		unitID, err := CreateDomainNode(nc, root.ID, NodeTypeEnergyUnit, map[string]string{
			PointTypeAssetCode: code, PointTypeRatedPowerKW: "60",
		})
		if err != nil {
			t.Fatal(err)
		}
		capText, _ := EncodeCapability(Capability{Kind: "discharge", Direction: "bidir", PowerMaxKW: 60, Allowed: true})
		if err := siotclient.SendNodePoint(nc, unitID, data.NewPointString(PointTypeCapability, "", capText), true); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, unitID)
	}

	// resourceGroup：成员两柜 + 共享上限 70。
	groupID, err := CreateDomainNode(nc, root.ID, NodeTypeResourceGroup, map[string]string{
		"description": "共享馈线组", PointTypeSharedLimitKW: "70",
	})
	if err != nil {
		t.Fatal(err)
	}
	memberText, _ := json.Marshal(ids)
	if err := siotclient.SendNodePoint(nc, groupID, data.NewPointString(PointTypeGroupMembers, "", string(memberText)), true); err != nil {
		t.Fatal(err)
	}

	handler := &Handler{nc: nc}
	result, err := handler.RunGroupDispatch(groupID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeliveredKW > 70 {
		t.Fatalf("共享上限 70 应封顶合计，实测 %.1f", result.DeliveredKW)
	}
	if result.ShortfallKW != 30 {
		t.Fatalf("缺口应为 30，实测 %.1f", result.ShortfallKW)
	}
	if result.LimitedBy != "group_shared_limit" {
		t.Fatalf("应标注组上限来源，实测 %q", result.LimitedBy)
	}
	t.Logf("组调度通过：合计 %.1f 缺口 %.1f（%s）", result.DeliveredKW, result.ShortfallKW, result.LimitedBy)
}
