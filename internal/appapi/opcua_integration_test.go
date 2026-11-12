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

	"fastiot/internal/ems/opcua"
	"fastiot/internal/ems/signal"
)

type fakeOpcuaConn struct{}

func (fakeOpcuaConn) Read(_ context.Context, nodeID string) (opcua.NodeValue, error) {
	return opcua.NodeValue{Value: signal.Num(42), At: time.Now()}, nil
}
func (fakeOpcuaConn) Write(context.Context, string, signal.Value) error { return nil }
func (fakeOpcuaConn) Close(context.Context)                             {}

// TestOpcuaRunnerPublishesValues 验证 OPC-UA 通道配置能把真实轮询结果写回
// 点位 value；离线时不生成替代数据。
func TestOpcuaRunnerPublishesValues(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	options := server.Options{
		ID: "easyems-opcua", DataDir: dataDir, HTTPPort: "9830",
		NatsPort: 9831, NatsServer: "nats://127.0.0.1:9831", AuthToken: data.NewID(),
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

	channelID, err := CreateDomainNode(nc, root.ID, NodeTypeOpcua, map[string]string{
		data.PointTypeDescription: "测试端点", pointTypeOpcuaURI: "opc.tcp://127.0.0.1:4840",
	})
	if err != nil {
		t.Fatal(err)
	}
	pointID, err := CreateDomainNode(nc, channelID, NodeTypeOpcuaPoint, map[string]string{
		data.PointTypeDescription: "SOC", pointTypeOpcuaNode: "ns=2;s=SOC",
	})
	if err != nil {
		t.Fatal(err)
	}

	manager := NewOpcuaManager(nc, nil)
	manager.Dial = func(string) opcua.Connection { return fakeOpcuaConn{} }
	manager.Sync()
	defer manager.Sync()

	waitFor(t, func() bool {
		nodes, err := siotclient.GetNodes(nc, "all", pointID, "", false)
		if err != nil || len(nodes) == 0 {
			return false
		}
		value, found := nodes[0].Points.Find(data.PointTypeValue, "")
		return found && pointFloat(value) == 42
	}, "OPC-UA value publish")

	channels, _ := siotclient.GetNodes(nc, "all", channelID, "", false)
	health, _ := channels[0].Points.Value(pointTypeOpcuaHealth, "")
	if health == 0 {
		t.Fatal("channel should report online after a successful poll")
	}
}
