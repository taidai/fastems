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
