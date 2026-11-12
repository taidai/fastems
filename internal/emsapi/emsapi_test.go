package emsapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fastems/internal/ems/alarm"
	"fastems/internal/ems/bridge"
	"fastems/internal/ems/channel"
	"fastems/internal/ems/history"
	"fastems/internal/ems/runtime"
	"fastems/internal/ems/signal"
)

type fakeConfigurator struct{ created, deleted int }

func (f *fakeConfigurator) Create(access Access, input ChannelInput) (map[string]any, error) {
	if !access.Admin {
		return nil, fmt.Errorf("admin_required")
	}
	f.created++
	return map[string]any{"channel_id": input.ID, "node_id": input.ID, "point_nodes": []string{}}, nil
}

func (f *fakeConfigurator) Delete(access Access, id string) error {
	if !access.Admin {
		return fmt.Errorf("admin_required")
	}
	f.deleted++
	return nil
}

type fakeBus struct {
	holding     map[uint16]uint16
	coils       map[uint16]bool
	failServers map[byte]bool
}

func (f *fakeBus) fail(server byte) bool { return f.failServers[server] }

func (f *fakeBus) ReadCoils(_ byte, address, count uint16) ([]bool, error) {
	out := make([]bool, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.coils[address+i]
	}
	return out, nil
}

func (f *fakeBus) ReadDiscreteInputs(_ byte, address, count uint16) ([]bool, error) {
	return make([]bool, count), nil
}

func (f *fakeBus) ReadHoldingRegisters(server byte, address, count uint16) ([]uint16, error) {
	if f.fail(server) {
		return nil, fmt.Errorf("timeout")
	}
	out := make([]uint16, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.holding[address+i]
	}
	return out, nil
}

func (f *fakeBus) ReadInputRegisters(server byte, address, count uint16) ([]uint16, error) {
	if f.fail(server) {
		return nil, fmt.Errorf("timeout")
	}
	return make([]uint16, count), nil
}

func (f *fakeBus) WriteCoil(_ byte, address uint16, value bool) error {
	f.coils[address] = value
	return nil
}

func (f *fakeBus) WriteHoldingRegister(_ byte, address, value uint16) error {
	f.holding[address] = value
	return nil
}

func tempDevice() channel.Device {
	return channel.Device{
		ID: "d1", ServerID: 1,
		Points: []channel.Point{{
			ID: "temp", Function: channel.HoldingRegister, Address: 10,
			Format: "uint16", Scale: 0.1, Kind: signal.Number,
		}},
	}
}

func call(t *testing.T, handler *Handler, access Access, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request, access)
	return recorder
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestStateHistoryAndExport(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 1000)
	defer service.Close()
	bus := &fakeBus{holding: map[uint16]uint16{10: 351}}
	if err := service.AddChannel("c1", bus, []channel.Device{tempDevice()}, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	handler := New(service, 5*time.Second)
	waitFor(t, func() bool {
		value, ok := service.State.Get(time.Now(), "d1", "temp", time.Minute)
		return ok && value.Value.Num == 35.1
	})

	response := call(t, handler, Access{UserID: "viewer"}, http.MethodGet, "/api/ems/state", "")
	if response.Code != http.StatusOK {
		t.Fatalf("state status = %d", response.Code)
	}
	var state struct {
		Values []struct {
			DeviceID string `json:"device_id"`
			PointID  string `json:"point_id"`
			Value    struct {
				Kind   string  `json:"kind"`
				Number float64 `json:"number"`
			} `json:"value"`
			Quality string `json:"quality"`
		} `json:"values"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Values) != 1 || state.Values[0].Value.Number != 35.1 || state.Values[0].Quality != "good" {
		t.Fatalf("state = %+v", state)
	}

	history := call(t, handler, Access{UserID: "viewer"}, http.MethodGet, "/api/ems/history?device_id=d1&point_id=temp&limit=10", "")
	if history.Code != http.StatusOK {
		t.Fatalf("history status = %d", history.Code)
	}
	var payload struct {
		Samples []struct {
			Value struct {
				Number float64 `json:"number"`
			} `json:"value"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Samples) == 0 || payload.Samples[len(payload.Samples)-1].Value.Number != 35.1 {
		t.Fatalf("history payload = %+v", payload)
	}

	export := call(t, handler, Access{UserID: "viewer"}, http.MethodGet, "/api/ems/history/export?device_id=d1&point_id=temp", "")
	if export.Code != http.StatusOK || !strings.Contains(export.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("export status = %d type=%s", export.Code, export.Header().Get("Content-Type"))
	}
	if !strings.Contains(export.Body.String(), "35.1") {
		t.Fatalf("export missing value: %s", export.Body.String())
	}
}

func TestAlarmLifecycleAPI(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)
	if _, err := service.Alarms.Raise(time.Now(), "device-offline:d1", alarm.Critical, "设备通信中断", "timeout", "d1", ""); err != nil {
		t.Fatal(err)
	}

	active := call(t, handler, Access{UserID: "viewer"}, http.MethodGet, "/api/ems/alarms?state=active", "")
	if active.Code != http.StatusOK || !strings.Contains(active.Body.String(), "device-offline:d1") {
		t.Fatalf("active alarms = %d %s", active.Code, active.Body.String())
	}

	ack := call(t, handler, Access{UserID: "operator", AlarmAck: true}, http.MethodPost, "/api/ems/alarms/device-offline:d1/ack", "")
	if ack.Code != http.StatusOK || !strings.Contains(ack.Body.String(), "acknowledged") {
		t.Fatalf("ack = %d %s", ack.Code, ack.Body.String())
	}

	if _, err := service.Alarms.Clear(time.Now(), "device-offline:d1"); err != nil {
		t.Fatal(err)
	}
	closed := call(t, handler, Access{UserID: "operator", AlarmAck: true}, http.MethodPost, "/api/ems/alarms/device-offline:d1/close", "")
	if closed.Code != http.StatusOK || !strings.Contains(closed.Body.String(), "closed") {
		t.Fatalf("close = %d %s", closed.Code, closed.Body.String())
	}
}

func TestChannelConfigurationRequiresAdmin(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)
	configurator := &fakeConfigurator{}
	handler.Configure = configurator
	body := `{"id":"c9","parent":"root","protocol":"tcp","address":"127.0.0.1:502","period_ms":10,"devices":[{"id":"d9","server_id":1,"points":[{"id":"temp","name":"温度","function":"holdingRegister","address":10,"format":"uint16","scale":0.1,"kind":"number"}]}]}`

	if response := call(t, handler, Access{UserID: "operator"}, http.MethodPost, "/api/ems/channels", body); response.Code != http.StatusForbidden {
		t.Fatalf("non-admin create = %d", response.Code)
	}
	created := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodPost, "/api/ems/channels", body)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"channel_id":"c9"`) {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	deleted := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodDelete, "/api/ems/channels/c9", "")
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete = %d", deleted.Code)
	}
	if configurator.created != 1 || configurator.deleted != 1 {
		t.Fatalf("configurator calls = %+v", configurator)
	}
}

func TestScanFindsOnlyRespondingSlaves(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)
	handler.OpenBus = func(string, string, time.Duration) (channel.Bus, error) {
		return &fakeBus{holding: map[uint16]uint16{0: 1}, failServers: map[byte]bool{2: true}}, nil
	}
	response := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodPost, "/api/ems/scan",
		`{"protocol":"tcp","address":"127.0.0.1:1502","from":1,"to":2}`)
	if response.Code != http.StatusOK {
		t.Fatalf("scan = %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Scanned int `json:"scanned"`
		Found   []struct {
			ServerID int `json:"server_id"`
		} `json:"found"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scanned != 2 || len(payload.Found) != 1 || payload.Found[0].ServerID != 1 {
		t.Fatalf("scan payload = %+v", payload)
	}
}

func TestScanRejectsBusyAddress(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)
	handler.Buses = func() ([]bridge.BusConfig, error) {
		return []bridge.BusConfig{{ChannelID: "c1", Protocol: "tcp", Address: "127.0.0.1:1502"}}, nil
	}
	response := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodPost, "/api/ems/scan",
		`{"protocol":"tcp","address":"127.0.0.1:1502","from":1,"to":4}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "channel_busy") {
		t.Fatalf("scan busy = %d %s", response.Code, response.Body.String())
	}
}

func TestScanRejectsInvalidRange(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)
	response := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodPost, "/api/ems/scan",
		`{"protocol":"tcp","address":"127.0.0.1:1502","from":5,"to":2}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("scan range = %d %s", response.Code, response.Body.String())
	}
}

func TestIdentifyManualAndUnknownProfile(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)

	manual := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodPost, "/api/ems/identify",
		`{"server_id":1,"profile_id":"modbus-generic"}`)
	if manual.Code != http.StatusOK || !strings.Contains(manual.Body.String(), `"matched":true`) {
		t.Fatalf("manual identify = %d %s", manual.Code, manual.Body.String())
	}
	unknown := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodPost, "/api/ems/identify",
		`{"server_id":1,"profile_id":"missing"}`)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown profile = %d", unknown.Code)
	}
}

func TestProfilesList(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)
	response := call(t, handler, Access{UserID: "admin", Admin: true}, http.MethodGet, "/api/ems/profiles", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "modbus-generic") {
		t.Fatalf("profiles = %d %s", response.Code, response.Body.String())
	}
}

func TestAllocationPreview(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	handler := New(service, 5*time.Second)
	body := `{"target":800,"members":[
		{"id":"CAB01","online":true,"available_discharge":200,"max_discharge":200},
		{"id":"CAB02","online":true,"available_discharge":200,"max_discharge":200},
		{"id":"CAB03","online":true,"available_discharge":150,"max_discharge":150},
		{"id":"CAB04","online":false,"available_discharge":200,"max_discharge":200}]}`
	response := call(t, handler, Access{UserID: "viewer"}, http.MethodPost, "/api/ems/allocation/preview", body)
	if response.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Result struct {
			Delivered float64 `json:"delivered"`
			Shortfall float64 `json:"shortfall"`
			Shares    []struct {
				MemberID string  `json:"member_id"`
				Setpoint float64 `json:"setpoint"`
			} `json:"shares"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"CAB01": 200, "CAB02": 200, "CAB03": 150, "CAB04": 0}
	for _, share := range payload.Result.Shares {
		if share.Setpoint != want[share.MemberID] {
			t.Errorf("%s = %v, want %v", share.MemberID, share.Setpoint, want[share.MemberID])
		}
	}
	if payload.Result.Delivered != 550 || payload.Result.Shortfall != 250 {
		t.Fatalf("delivered/shortfall = %v/%v", payload.Result.Delivered, payload.Result.Shortfall)
	}
}

func TestTreeScopeFiltersProxyReads(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	for _, id := range []string{"c1", "c2"} {
		device := tempDevice()
		device.ID = id + "/1"
		device.Points[0].ID = map[string]string{"c1": "p1", "c2": "p2"}[id]
		if err := service.AddChannel(id, &fakeBus{holding: map[uint16]uint16{10: 10}}, []channel.Device{device}, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	service.State.Apply(now, []channel.Result{
		{DeviceID: "c1/1", PointID: "p1", Value: signal.Num(1), Quality: signal.Good, SourceTime: now},
		{DeviceID: "c2/1", PointID: "p2", Value: signal.Num(2), Quality: signal.Good, SourceTime: now},
	})
	service.History.Record([]history.Sample{
		{DeviceID: "c1/1", PointID: "p1", Value: signal.Num(1), Quality: signal.Good, SourceTime: now},
		{DeviceID: "c2/1", PointID: "p2", Value: signal.Num(2), Quality: signal.Good, SourceTime: now},
	})
	handler := New(service, 5*time.Second)
	scoped := Access{AllowNode: func(id string) bool { return id == "c1" || id == "p1" }}

	var state struct {
		Values []struct {
			PointID string `json:"point_id"`
		} `json:"values"`
	}
	response := call(t, handler, Access{}, http.MethodGet, "/api/ems/state", "")
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Values) != 2 {
		t.Fatalf("unscoped state = %+v", state.Values)
	}
	response = call(t, handler, scoped, http.MethodGet, "/api/ems/state", "")
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Values) != 1 || state.Values[0].PointID != "p1" {
		t.Fatalf("scoped state = %+v", state.Values)
	}

	var channels struct {
		Channels []struct {
			ID string `json:"id"`
		} `json:"channels"`
	}
	response = call(t, handler, Access{}, http.MethodGet, "/api/ems/channels", "")
	if err := json.Unmarshal(response.Body.Bytes(), &channels); err != nil {
		t.Fatal(err)
	}
	if len(channels.Channels) != 2 {
		t.Fatalf("unscoped channels = %+v", channels.Channels)
	}
	response = call(t, handler, scoped, http.MethodGet, "/api/ems/channels", "")
	if err := json.Unmarshal(response.Body.Bytes(), &channels); err != nil {
		t.Fatal(err)
	}
	if len(channels.Channels) != 1 || channels.Channels[0].ID != "c1" {
		t.Fatalf("scoped channels = %+v", channels.Channels)
	}

	var quality struct {
		Channels []struct {
			ChannelID string `json:"channel_id"`
		} `json:"channels"`
		Devices []struct {
			ChannelID string `json:"channel_id"`
		} `json:"devices"`
	}
	response = call(t, handler, Access{}, http.MethodGet, "/api/ems/quality", "")
	if err := json.Unmarshal(response.Body.Bytes(), &quality); err != nil {
		t.Fatal(err)
	}
	if len(quality.Channels) != 2 || len(quality.Devices) != 2 {
		t.Fatalf("unscoped quality = %+v", quality)
	}
	response = call(t, handler, scoped, http.MethodGet, "/api/ems/quality", "")
	if err := json.Unmarshal(response.Body.Bytes(), &quality); err != nil {
		t.Fatal(err)
	}
	if len(quality.Channels) != 1 || quality.Channels[0].ChannelID != "c1" ||
		len(quality.Devices) != 1 || quality.Devices[0].ChannelID != "c1" {
		t.Fatalf("scoped quality = %+v", quality)
	}

	response = call(t, handler, scoped, http.MethodGet, "/api/ems/runtime", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"channels":1`) {
		t.Fatalf("scoped runtime = %d %s", response.Code, response.Body.String())
	}
	allowed := call(t, handler, scoped, http.MethodGet, "/api/ems/history?device_id=c1/1&point_id=p1", "")
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), `"samples":[{`) {
		t.Fatalf("allowed history = %d %s", allowed.Code, allowed.Body.String())
	}
	for _, path := range []string{
		"/api/ems/history?device_id=c2/1&point_id=p2",
		"/api/ems/history/export?device_id=c2/1&point_id=p2",
	} {
		response = call(t, handler, scoped, http.MethodGet, path, "")
		if response.Code != http.StatusForbidden {
			t.Fatalf("denied history %s = %d %s", path, response.Code, response.Body.String())
		}
	}
}

func TestTreeScopeGuardsChannelMutations(t *testing.T) {
	service := runtime.NewService(nil, 5*time.Second, time.Second, 100)
	defer service.Close()
	for _, id := range []string{"c1", "c2"} {
		device := tempDevice()
		device.ID = id + "/1"
		if err := service.AddChannel(id, &fakeBus{holding: map[uint16]uint16{}}, []channel.Device{device}, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(service, 5*time.Second)
	configurator := &fakeConfigurator{}
	handler.Configure = configurator
	restarted := 0
	handler.RestartChannel = func(string) error {
		restarted++
		return nil
	}
	access := Access{Admin: true, AllowNode: func(id string) bool { return id == "root" || id == "c1" }}
	body := `{"id":"c9","parent":"root","protocol":"tcp","address":"127.0.0.1:502","devices":[{"id":"d9","server_id":1,"points":[{"id":"temp","function":"holdingRegister","address":10,"format":"uint16","kind":"number"}]}]}`

	response := call(t, handler, access, http.MethodPost, "/api/ems/channels", strings.Replace(body, `"parent":"root"`, `"parent":"other"`, 1))
	if response.Code != http.StatusForbidden {
		t.Fatalf("denied create = %d %s", response.Code, response.Body.String())
	}
	response = call(t, handler, access, http.MethodDelete, "/api/ems/channels/c2", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("denied delete = %d %s", response.Code, response.Body.String())
	}
	response = call(t, handler, access, http.MethodPost, "/api/ems/channels/c2/restart", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("denied restart = %d %s", response.Code, response.Body.String())
	}
	if configurator.created != 0 || configurator.deleted != 0 || restarted != 0 || len(service.Channels()) != 2 {
		t.Fatalf("denied mutations had side effects: config=%+v restarted=%d channels=%d",
			configurator, restarted, len(service.Channels()))
	}

	response = call(t, handler, access, http.MethodPost, "/api/ems/channels", body)
	if response.Code != http.StatusOK {
		t.Fatalf("allowed create = %d %s", response.Code, response.Body.String())
	}
	response = call(t, handler, access, http.MethodPost, "/api/ems/channels/c1/restart", "")
	if response.Code != http.StatusOK {
		t.Fatalf("allowed restart = %d %s", response.Code, response.Body.String())
	}
	response = call(t, handler, access, http.MethodDelete, "/api/ems/channels/c1", "")
	if response.Code != http.StatusOK {
		t.Fatalf("allowed delete = %d %s", response.Code, response.Body.String())
	}
	channels := service.Channels()
	if configurator.created != 1 || configurator.deleted != 1 || restarted != 1 ||
		len(channels) != 1 || channels[0].ID != "c2" {
		t.Fatalf("allowed mutations: config=%+v restarted=%d channels=%+v", configurator, restarted, channels)
	}
}
