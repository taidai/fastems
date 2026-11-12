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

	"fastems/internal/ems/runtime"
	"fastems/internal/emsapi"
)

func TestScopeNodesToSite(t *testing.T) {
	nodes := []data.NodeEdge{
		{ID: "root"},
		{ID: "site-a", Type: NodeTypeSite, Parent: "root"},
		{ID: "plant-a", Type: NodeTypePlant, Parent: "site-a"},
		{ID: "unit-a", Type: NodeTypeEnergyUnit, Parent: "plant-a"},
		{ID: "site-b", Type: NodeTypeSite, Parent: "root"},
		{ID: "unit-b", Type: NodeTypeEnergyUnit, Parent: "site-b"},
		{ID: "channel", Type: data.NodeTypeModbus, Parent: "root"},
		{ID: "io", Type: data.NodeTypeModbusIO, Parent: "channel"},
	}
	ids := func(list []data.NodeEdge) map[string]bool {
		out := map[string]bool{}
		for _, node := range list {
			out[node.ID] = true
		}
		return out
	}
	scoped := ids(scopeNodesToSite(nodes, "site-a"))
	for _, want := range []string{"root", "site-a", "plant-a", "unit-a", "channel", "io"} {
		if !scoped[want] {
			t.Fatalf("site-a scope missing %s: %#v", want, scoped)
		}
	}
	for _, notWant := range []string{"site-b", "unit-b"} {
		if scoped[notWant] {
			t.Fatalf("site-a scope leaked %s", notWant)
		}
	}
}

func TestLedgerScopedBySite(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	var users api.UserAuthority
	options := server.Options{
		ID: "easyems-site-ledger", DataDir: dataDir, HTTPPort: "9870",
		NatsPort: 9871, NatsServer: "nats://127.0.0.1:9871", AuthToken: data.NewID(),
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

	channelID, _ := CreateDomainNode(nc, root.ID, data.NodeTypeModbus, map[string]string{"description": "共享通道"})
	ioA, _ := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{"description": "A 柜点位"})
	ioB, _ := CreateDomainNode(nc, channelID, data.NodeTypeModbusIO, map[string]string{"description": "B 柜点位"})
	// 站号是浮点点位（modbus 客户端写入方式），字符串点位读不出整数。
	for id, server := range map[string]float64{ioA: 1, ioB: 2} {
		if err := siotclient.SendPoints(nc, siotclient.SubjectNodePoints(id), data.Points{
			data.NewPointFloat(data.PointTypeID, "", server),
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	siteA, _ := CreateDomainNode(nc, root.ID, NodeTypeSite, map[string]string{"description": "A 站"})
	unitA, _ := CreateDomainNode(nc, siteA, NodeTypeEnergyUnit, map[string]string{"description": "A 单元"})
	siteB, _ := CreateDomainNode(nc, root.ID, NodeTypeSite, map[string]string{"description": "B 站"})
	unitB, _ := CreateDomainNode(nc, siteB, NodeTypeEnergyUnit, map[string]string{"description": "B 单元"})
	CreateDomainNode(nc, unitA, data.NodeTypeDevice, map[string]string{"description": "A PCS", PointTypePowerSetpoint: ioA})
	CreateDomainNode(nc, unitB, data.NodeTypeDevice, map[string]string{"description": "B PCS", PointTypePowerSetpoint: ioB})
	time.Sleep(300 * time.Millisecond)

	handler := New(nc, users, nil)
	handler.SetEMS(emsapi.New(runtime.NewService(nil, 15*time.Second, 5*time.Second, 100), 15*time.Second))
	admin := Identity{UserID: "admin", Policy: AccessPolicy{Admin: true}, Anchors: []string{root.ID}}

	devicesA, err := handler.ledgerDevices(admin, siteA)
	if err != nil {
		t.Fatal(err)
	}
	if len(devicesA) != 1 || devicesA[0].ServerID != 1 {
		t.Fatalf("site A ledger = %#v", devicesA)
	}
	devicesB, err := handler.ledgerDevices(admin, siteB)
	if err != nil {
		t.Fatal(err)
	}
	if len(devicesB) != 1 || devicesB[0].ServerID != 2 {
		t.Fatalf("site B ledger = %#v", devicesB)
	}
	all, err := handler.ledgerDevices(admin, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("global ledger = %#v, %v", all, err)
	}
}
