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

// TestRefreshCapabilities 验证动态能力：SOC 低于下限时 capability 变为不可用，
// SOC 正常时按额定刷新为可用。
func TestRefreshCapabilities(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-cap-refresh", DataDir: dataDir, HTTPPort: "9748",
		NatsPort: 9749, NatsServer: "nats://127.0.0.1:9749", AuthToken: data.NewID(),
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

	socID, _ := CreateDomainNode(nc, root.ID, "variable", map[string]string{"description": "SOC"})
	unitID, _ := CreateDomainNode(nc, root.ID, NodeTypeEnergyUnit, map[string]string{PointTypeAssetCode: "EU1", PointTypeRatedPowerKW: "100"})
	inputs := CapabilityInputs{SOCPointID: socID, SOCMin: 10, SOCMax: 90}
	raw, _ := json.Marshal(inputs)
	encoded := string(raw)
	if err := siotclient.SendNodePoint(nc, unitID, data.NewPointString(capabilityInputsPoint, "", encoded), true); err != nil {
		t.Fatal(err)
	}

	// SOC 低：放电被剔除，仅允许充电。
	_ = siotclient.SendNodePoint(nc, socID, data.NewPointFloat(data.PointTypeValue, "", 5), true)
	RefreshCapabilities(nc)
	nodes, _ := siotclient.GetNodes(nc, "all", unitID, "", false)
	point, _ := nodes[0].Points.Find(PointTypeCapability, "")
	capability, _ := DecodeCapability(point.Txt())
	if capability.Direction != "charge" {
		t.Fatalf("SOC 5 应仅允许充电，实测 %+v", capability)
	}

	// SOC 正常：按额定可用。
	_ = siotclient.SendNodePoint(nc, socID, data.NewPointFloat(data.PointTypeValue, "", 60), true)
	RefreshCapabilities(nc)
	nodes, _ = siotclient.GetNodes(nc, "all", unitID, "", false)
	point, _ = nodes[0].Points.Find(PointTypeCapability, "")
	capability, _ = DecodeCapability(point.Txt())
	if !capability.Allowed || capability.PowerMaxKW != 100 || capability.Direction != "bidir" {
		t.Fatalf("SOC 60 应双向可用 100kW，实测 %+v", capability)
	}
}
