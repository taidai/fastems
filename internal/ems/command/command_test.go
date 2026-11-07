package command

import (
	"errors"
	"strings"
	"testing"
	"time"

	"easyems/internal/ems/device"
	"easyems/internal/ems/signal"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

type fakeWriter struct {
	err   error
	calls int
	last  []device.Write
}

func (f *fakeWriter) Write(_, _ string, writes []device.Write) error {
	f.calls++
	f.last = append([]device.Write(nil), writes...)
	return f.err
}

func pcsProfile() device.Profile {
	return device.Profile{
		ID: "enjo", Vendor: "恩玖", Model: "PCS",
		Points: []device.Point{
			{ID: "power", Kind: signal.Number, Unit: "kW", HasRange: true, Min: -500, Max: 500},
			{ID: "run", Kind: signal.Bool},
		},
		Commands: []device.Command{{
			ID: "start", Name: "启动",
			Writes:   []device.Write{{Point: "run", Value: signal.Flag(true)}},
			Readback: []device.Readback{{Point: "run", Op: "eq", Value: signal.Flag(true)}},
			Timeout:  5 * time.Second,
		}},
	}
}

func manager(writer Dispatcher, samples *map[string]signal.Sample) *Manager {
	return New(writer,
		func(deviceID string) (device.Profile, bool) {
			if deviceID != "pcs1" {
				return device.Profile{}, false
			}
			return pcsProfile(), true
		},
		func(string, time.Time) map[string]signal.Sample { return *samples },
		5*time.Second, 3*time.Second)
}

func request(id string) Request {
	return Request{ID: id, DeviceID: "pcs1", CommandID: "start", Actor: "operator"}
}

func TestSubmitWritesAndReadbackConfirms(t *testing.T) {
	writer := &fakeWriter{}
	samples := map[string]signal.Sample{}
	m := manager(writer, &samples)
	outcome, err := m.Submit(t0, request("r1"))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != Executing || writer.calls != 1 {
		t.Fatalf("expected executing after one write, got %+v calls=%d", outcome, writer.calls)
	}
	samples["run"] = signal.Sample{Input: "run", Value: signal.Flag(true), Quality: signal.Good, SourceTime: t0, ReceivedTime: t0}
	m.Tick(t0.Add(time.Second))
	got, _ := m.Get("r1")
	if got.State != Confirmed {
		t.Fatalf("expected confirmed, got %+v", got)
	}
}

func TestSubmitIsIdempotent(t *testing.T) {
	writer := &fakeWriter{}
	samples := map[string]signal.Sample{}
	m := manager(writer, &samples)
	if _, err := m.Submit(t0, request("r1")); err != nil {
		t.Fatal(err)
	}
	outcome, err := m.Submit(t0.Add(time.Second), request("r1"))
	if err != nil {
		t.Fatal(err)
	}
	if writer.calls != 1 {
		t.Fatalf("duplicate submit wrote %d times", writer.calls)
	}
	if outcome.RequestID != "r1" {
		t.Fatalf("unexpected outcome %+v", outcome)
	}

	conflict := request("r1")
	conflict.CommandID = "stop"
	if _, err := m.Submit(t0, conflict); err == nil {
		t.Fatal("expected conflict for reused request id")
	}
}

func TestTimeoutBecomesUnknown(t *testing.T) {
	writer := &fakeWriter{}
	samples := map[string]signal.Sample{}
	m := manager(writer, &samples)
	if _, err := m.Submit(t0, request("r1")); err != nil {
		t.Fatal(err)
	}
	m.Tick(t0.Add(4 * time.Second))
	if got, _ := m.Get("r1"); got.State != Executing {
		t.Fatalf("should still be executing before timeout, got %+v", got)
	}
	m.Tick(t0.Add(6 * time.Second))
	got, _ := m.Get("r1")
	if got.State != Unknown || !strings.Contains(got.Reason, "timeout") {
		t.Fatalf("expected unknown after timeout, got %+v", got)
	}
}

func TestWriteFailureIsFailed(t *testing.T) {
	writer := &fakeWriter{err: errors.New("port closed")}
	samples := map[string]signal.Sample{}
	m := manager(writer, &samples)
	outcome, err := m.Submit(t0, request("r1"))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != Failed || !strings.Contains(outcome.Reason, "port closed") {
		t.Fatalf("expected failed outcome, got %+v", outcome)
	}
}

func TestUnknownDeviceOrCommandRejected(t *testing.T) {
	writer := &fakeWriter{}
	samples := map[string]signal.Sample{}
	m := manager(writer, &samples)
	bad := Request{ID: "r1", DeviceID: "missing", CommandID: "start"}
	if _, err := m.Submit(t0, bad); err == nil {
		t.Fatal("expected unknown device error")
	}
	bad = Request{ID: "r2", DeviceID: "pcs1", CommandID: "missing"}
	if _, err := m.Submit(t0, bad); err == nil {
		t.Fatal("expected unknown command error")
	}
	if writer.calls != 0 {
		t.Fatalf("rejected commands must not write, calls=%d", writer.calls)
	}
}


func parameterProfile() device.Profile {
	return device.Profile{
		ID: "pcs", Vendor: "test", Model: "parameter",
		Points: []device.Point{{ID: "percent", Kind: signal.Number}},
		Commands: []device.Command{{
			ID: "set-percent", Name: "设置百分比", Timeout: 3 * time.Second,
			Parameters: []device.Parameter{{
				Name: "percent", Label: "百分比", Point: "percent", Kind: signal.Number,
				HasRange: true, Min: 0, Max: 100, Default: 10,
			}},
		}},
	}
}

func parameterManager(writer Dispatcher, samples *map[string]signal.Sample) *Manager {
	return New(writer,
		func(deviceID string) (device.Profile, bool) {
			if deviceID != "pcs1" {
				return device.Profile{}, false
			}
			return parameterProfile(), true
		},
		func(string, time.Time) map[string]signal.Sample { return *samples },
		5*time.Second, 3*time.Second)
}

func TestParameterizedCommandResolvesWritesAndReadback(t *testing.T) {
	writer := &fakeWriter{}
	samples := map[string]signal.Sample{}
	m := parameterManager(writer, &samples)
	req := Request{ID: "r1", DeviceID: "pcs1", CommandID: "set-percent", Actor: "operator", Parameters: map[string]float64{"percent": 42}}
	outcome, err := m.Submit(t0, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(writer.last) != 1 || writer.last[0].Point != "percent" || writer.last[0].Value.Num != 42 {
		t.Fatalf("writes = %+v", writer.last)
	}
	if len(outcome.Readback) != 1 || outcome.Readback[0].Value.Num != 42 {
		t.Fatalf("readback = %+v", outcome.Readback)
	}
	samples["percent"] = signal.Sample{Input: "percent", Value: signal.Num(42), Quality: signal.Good, SourceTime: t0, ReceivedTime: t0}
	m.Tick(t0.Add(time.Second))
	if got, _ := m.Get("r1"); got.State != Confirmed {
		t.Fatalf("expected confirmed, got %+v", got)
	}
}

func TestParameterizedCommandValidatesRangeAndUnknownNames(t *testing.T) {
	writer := &fakeWriter{}
	samples := map[string]signal.Sample{}
	m := parameterManager(writer, &samples)
	req := Request{ID: "r1", DeviceID: "pcs1", CommandID: "set-percent", Parameters: map[string]float64{"percent": 101}}
	if _, err := m.Submit(t0, req); err == nil {
		t.Fatal("expected range error")
	}
	req.Parameters = map[string]float64{"unknown": 1}
	if _, err := m.Submit(t0, req); err == nil {
		t.Fatal("expected unknown parameter error")
	}
	// Missing parameter uses the default and confirms against that value.
	req.Parameters = nil
	if _, err := m.Submit(t0, req); err != nil {
		t.Fatal(err)
	}
	if len(writer.last) != 1 || writer.last[0].Value.Num != 10 {
		t.Fatalf("default writes = %+v", writer.last)
	}
}
