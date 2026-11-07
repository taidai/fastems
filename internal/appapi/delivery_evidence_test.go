package appapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	"github.com/simpleiot/simpleiot/data"
	"github.com/simpleiot/simpleiot/server"

	"easyems/internal/ems/runtime"
	"easyems/internal/emsapi"
)

func TestModbusCSVDiffAndDeliveryEvidence(t *testing.T) {
	const password = "easyems-evidence-test"
	t.Setenv("EASYEMS_ADMIN_PASSWORD", password)
	dataDir := filepath.Join(t.TempDir(), "data")
	ready := new(atomic.Bool)
	options := server.Options{
		ID: "easyems-evidence-test", DataDir: dataDir, HTTPPort: "9721",
		NatsPort: 9720, NatsServer: "nats://127.0.0.1:9720", AuthToken: data.NewID(),
		DeviceAuth: server.DeviceAuthRequired, CustomUIFS: fstest.MapFS{"index.html": {Data: []byte("easyems")}},
		AppOnly: true, NatsLoopback: true, ProvisioningDisable: true,
		AppHandlerFactory: func(nc *nats.Conn, users api.UserAuthority) http.Handler {
			handler := New(nc, users, ready)
			handler.SetDataDir(dataDir)
			handler.SetEMS(emsapi.New(runtime.NewService(nil, 15*time.Second, 5*time.Second, 100), 15*time.Second))
			return handler
		},
	}
	siot, nc, err := server.NewServer(options)
	if err != nil {
		t.Fatal(err)
	}
	_ = nc
	done := make(chan error, 1)
	go func() { done <- siot.Run() }()
	defer func() {
		siot.Stop(nil)
		<-done
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := siot.WaitStart(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(nc, dataDir); err != nil {
		t.Fatal(err)
	}
	ready.Store(true)
	base := "http://127.0.0.1:9721/api"
	waitFor(t, func() bool {
		response, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, "HTTP readiness")
	token := loginAPI(t, base, "admin", password)
	groupID := createNodeAPI(t, base, token, "group", snapshotRoot(t, base, token), "交付站", 0)

	var bus struct {
		ID string `json:"id"`
	}
	status, body := call(t, http.MethodPost, base+"/modbus", token, map[string]any{
		"parent": groupID, "description": "交付总线", "protocol": "TCP",
		"uri": "127.0.0.1:62999", "poll_period": 1000, "timeout": 500,
	}, &bus)
	if status != http.StatusOK {
		t.Fatalf("create bus: %d %s", status, body)
	}
	for _, input := range []map[string]any{
		{"description": "温度", "server_id": 1, "address": 10, "io_type": "modbusInputRegister", "data_format": "uint16", "read_only": true, "scale": 0.1, "offset": 0, "units": "°C"},
		{"description": "启停", "server_id": 1, "address": 0, "io_type": "modbusCoil", "data_format": "uint16", "read_only": false, "scale": 1, "offset": 0},
		{"description": "旧点位", "server_id": 1, "address": 20, "io_type": "modbusHoldingRegister", "data_format": "uint16", "read_only": true, "scale": 1, "offset": 0},
	} {
		status, body = call(t, http.MethodPost, base+"/modbus/"+bus.ID+"/io", token, input, nil)
		if status != http.StatusOK {
			t.Fatalf("create IO: %d %s", status, body)
		}
	}

	// CSV: 温度 changes scale+units, 启停 unchanged, 旧点位 missing, 新点位 added.
	csvBody := "名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位\n" +
		"温度,1,10,输入寄存器,uint16,是,0.5,0,℃\n" +
		"启停,1,0,线圈,uint16,否,1,0,\n" +
		"新点位,1,30,保持寄存器,uint16,是,1,0,kW\n"
	type diffResponse struct {
		Diff    bool            `json:"diff"`
		Added   []ModbusIOInput `json:"added"`
		Changed []struct {
			Name   string        `json:"name"`
			Before ModbusIOInput `json:"before"`
			After  ModbusIOInput `json:"after"`
			Fields []string      `json:"fields"`
		} `json:"changed"`
		Removed   []ModbusIOInput `json:"removed"`
		Unchanged int             `json:"unchanged"`
	}
	var diff diffResponse
	status, raw := callRaw(t, base+"/modbus/"+bus.ID+"/io/import?diff=true", token, csvBody)
	if status != http.StatusOK {
		t.Fatalf("diff failed: %d %s", status, raw)
	}
	decodeTestJSON(t, raw, &diff)
	if !diff.Diff || len(diff.Added) != 1 || diff.Added[0].Description != "新点位" {
		t.Fatalf("diff added wrong: %s", raw)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].Name != "温度" ||
		!strings.Contains(strings.Join(diff.Changed[0].Fields, ","), "缩放") ||
		!strings.Contains(strings.Join(diff.Changed[0].Fields, ","), "单位") {
		t.Fatalf("diff changed wrong: %s", raw)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Description != "旧点位" || diff.Unchanged != 1 {
		t.Fatalf("diff removed/unchanged wrong: %s", raw)
	}
	if nodeByDescription(t, base, token, "新点位") != nil {
		t.Fatal("diff preview created a point")
	}

	// Apply without remove: add + update, keep the missing point.
	status, raw = callRaw(t, base+"/modbus/"+bus.ID+"/io/import?apply=diff", token, csvBody)
	if status != http.StatusOK || !strings.Contains(raw, `"applied":true`) {
		t.Fatalf("apply diff failed: %d %s", status, raw)
	}
	waitFor(t, func() bool {
		node := nodeByDescription(t, base, token, "温度")
		if node == nil {
			return false
		}
		scale, _ := node.Points.Value(data.PointTypeScale, "")
		return scale == 0.5 && nodeByDescription(t, base, token, "新点位") != nil
	}, "diff apply updated scale and added point")
	if nodeByDescription(t, base, token, "旧点位") == nil {
		t.Fatal("apply without remove deleted a point")
	}

	// Apply with remove=true: the missing point is deleted.
	status, raw = callRaw(t, base+"/modbus/"+bus.ID+"/io/import?apply=diff&remove=true", token, csvBody)
	if status != http.StatusOK || !strings.Contains(raw, `"removed":1`) {
		t.Fatalf("apply diff with remove failed: %d %s", status, raw)
	}
	waitFor(t, func() bool {
		return nodeByDescription(t, base, token, "旧点位") == nil
	}, "diff apply removed the missing point")

	// Evidence package: one zip, the four CSVs plus the readme, no secrets.
	request, err := http.NewRequest(http.MethodGet, base+"/delivery/evidence", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("evidence failed: %d %s", response.StatusCode, payload)
	}
	if bytes.Contains(payload, []byte(password)) {
		t.Fatal("evidence zip contains the admin password")
	}
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		text, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		contents[file.Name] = string(text)
	}
	for _, name := range []string{"00-说明.txt", "01-设备台账.csv", "02-验证记录.csv", "03-通信测试报告.csv", "04-报警统计.csv"} {
		if _, ok := contents[name]; !ok {
			t.Fatalf("evidence zip missing %s (has %v)", name, fileNames(archive))
		}
	}
	if !strings.Contains(contents["01-设备台账.csv"], "交付总线") {
		t.Fatal("device ledger CSV missing the bus")
	}
	if !strings.Contains(contents["03-通信测试报告.csv"], "通信测试报告") {
		t.Fatal("communication report CSV missing header")
	}
	if strings.Contains(contents["00-说明.txt"], password) {
		t.Fatal("readme leaks the password")
	}

	// Viewer-level accounts must not export evidence.
	status, _ = call(t, http.MethodPost, base+"/users", token, map[string]any{
		"parent": groupID, "email": "viewer@easyems.local", "password": "viewer-password-123", "role": "viewer",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create viewer: %d", status)
	}
	viewerToken := loginAPI(t, base, "viewer@easyems.local", "viewer-password-123")
	status, _ = call(t, http.MethodGet, base+"/delivery/evidence", viewerToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("viewer could export evidence: %d", status)
	}
}

func decodeTestJSON(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
}

func nodeByDescription(t *testing.T, base, token, name string) *data.NodeEdge {
	t.Helper()
	for _, node := range snapshotNodes(t, base, token) {
		description, _ := node.Points.Text(data.PointTypeDescription, "")
		if description == name {
			found := node
			return &found
		}
	}
	return nil
}

func fileNames(archive *zip.Reader) []string {
	names := make([]string, 0, len(archive.File))
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	return names
}
