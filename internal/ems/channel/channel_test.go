package channel

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"fastiot/internal/ems/signal"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

type fakeBus struct {
	fail       map[byte]bool
	writeErr   error
	coils      map[uint16]bool
	discrete   map[uint16]bool
	holding    map[uint16]uint16
	input      map[uint16]uint16
	calls      []string
	writeCalls []string
	sequence   []string
}

func newFakeBus() *fakeBus {
	return &fakeBus{
		fail:     map[byte]bool{},
		coils:    map[uint16]bool{},
		discrete: map[uint16]bool{},
		holding:  map[uint16]uint16{},
		input:    map[uint16]uint16{},
	}
}

func (f *fakeBus) ReadCoils(server byte, address, count uint16) ([]bool, error) {
	f.calls = append(f.calls, fmt.Sprintf("coil:%d:%d:%d", server, address, count))
	if f.fail[server] {
		return nil, errors.New("timeout")
	}
	out := make([]bool, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.coils[address+i]
	}
	return out, nil
}

func (f *fakeBus) ReadDiscreteInputs(server byte, address, count uint16) ([]bool, error) {
	f.calls = append(f.calls, fmt.Sprintf("di:%d:%d:%d", server, address, count))
	if f.fail[server] {
		return nil, errors.New("timeout")
	}
	out := make([]bool, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.discrete[address+i]
	}
	return out, nil
}

func (f *fakeBus) ReadHoldingRegisters(server byte, address, count uint16) ([]uint16, error) {
	f.calls = append(f.calls, fmt.Sprintf("hr:%d:%d:%d", server, address, count))
	f.sequence = append(f.sequence, "read")
	if f.fail[server] {
		return nil, errors.New("timeout")
	}
	out := make([]uint16, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.holding[address+i]
	}
	return out, nil
}

func (f *fakeBus) ReadInputRegisters(server byte, address, count uint16) ([]uint16, error) {
	f.calls = append(f.calls, fmt.Sprintf("ir:%d:%d:%d", server, address, count))
	if f.fail[server] {
		return nil, errors.New("timeout")
	}
	out := make([]uint16, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.input[address+i]
	}
	return out, nil
}

func (f *fakeBus) WriteCoil(server byte, address uint16, value bool) error {
	f.writeCalls = append(f.writeCalls, fmt.Sprintf("coil:%d:%d:%t", server, address, value))
	if f.writeErr != nil {
		return f.writeErr
	}
	f.coils[address] = value
	return nil
}

func (f *fakeBus) WriteHoldingRegister(server byte, address, value uint16) error {
	f.writeCalls = append(f.writeCalls, fmt.Sprintf("hr:%d:%d:%d", server, address, value))
	f.sequence = append(f.sequence, "write")
	if f.writeErr != nil {
		return f.writeErr
	}
	f.holding[address] = value
	return nil
}

func tempPoint(address uint16) Point {
	return Point{ID: "temp", Function: HoldingRegister, Address: address, Format: "uint16", Scale: 0.1, Kind: signal.Number}
}

func device(id string, server byte, points ...Point) Device {
	return Device{ID: id, ServerID: server, Points: points}
}

func find(results []Result, deviceID, pointID string) (Result, bool) {
	for _, result := range results {
		if result.DeviceID == deviceID && result.PointID == pointID {
			return result, true
		}
	}
	return Result{}, false
}

func TestPollsAllDevicesOnOneChannel(t *testing.T) {
	bus := newFakeBus()
	bus.holding[10] = 351
	bus.holding[20] = 402
	bus.holding[30] = 253
	ch, err := New("c1", bus, []Device{
		device("d1", 1, tempPoint(10)),
		device("d2", 2, tempPoint(20)),
		device("d3", 3, tempPoint(30)),
	})
	if err != nil {
		t.Fatal(err)
	}
	results, _ := ch.RunOnce(context.Background(), t0)
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if result, _ := find(results, "d2", "temp"); math.Abs(result.Value.Num-40.2) > 1e-9 {
		t.Fatalf("d2 temp = %v, want 40.2", result.Value.Num)
	}
	for _, id := range []string{"d1", "d2", "d3"} {
		if got := ch.Health(id, t0, time.Minute); got != HealthHealthy {
			t.Errorf("%s health = %s, want healthy", id, got)
		}
	}
}

func TestOneFailingDeviceDoesNotStopTheChannel(t *testing.T) {
	bus := newFakeBus()
	bus.holding[10] = 100
	bus.holding[30] = 300
	bus.fail[2] = true
	ch, _ := New("c1", bus, []Device{
		device("d1", 1, tempPoint(10)),
		device("d2", 2, tempPoint(20)),
		device("d3", 3, tempPoint(30)),
	})
	for i := 0; i < 3; i++ {
		results, _ := ch.RunOnce(context.Background(), t0.Add(time.Duration(i)*time.Second))
		if _, ok := find(results, "d1", "temp"); !ok {
			t.Fatalf("run %d: healthy device d1 missing", i)
		}
		if _, ok := find(results, "d3", "temp"); !ok {
			t.Fatalf("run %d: healthy device d3 missing", i)
		}
		if _, ok := find(results, "d2", "temp"); ok {
			t.Fatalf("run %d: failing device d2 returned a value", i)
		}
	}
	if got := ch.Health("d2", t0.Add(3*time.Second), time.Minute); got != HealthOffline {
		t.Fatalf("d2 health = %s, want offline", got)
	}
	if got := ch.Health("d1", t0.Add(3*time.Second), time.Minute); got != HealthHealthy {
		t.Fatalf("d1 must stay healthy, got %s", got)
	}
	if stats := ch.Stats("d2"); stats.ConsecutiveFailures != 3 || stats.LastError == "" {
		t.Fatalf("unexpected stats %+v", stats)
	}
}

func TestContiguousPointsShareOneRead(t *testing.T) {
	bus := newFakeBus()
	points := []Point{
		{ID: "a", Function: HoldingRegister, Address: 10, Format: "uint16", Kind: signal.Number},
		{ID: "b", Function: HoldingRegister, Address: 11, Format: "uint16", Kind: signal.Number},
		{ID: "c", Function: HoldingRegister, Address: 12, Format: "uint16", Kind: signal.Number},
		{ID: "d", Function: HoldingRegister, Address: 20, Format: "uint16", Kind: signal.Number},
	}
	ch, _ := New("c1", bus, []Device{device("d1", 1, points...)})
	ch.RunOnce(context.Background(), t0)
	if len(bus.calls) != 2 {
		t.Fatalf("block reads = %d, want 2: %v", len(bus.calls), bus.calls)
	}
	if bus.calls[0] != "hr:1:10:3" || bus.calls[1] != "hr:1:20:1" {
		t.Fatalf("unexpected blocks: %v", bus.calls)
	}
}

func TestMaxGapMergesClosePoints(t *testing.T) {
	bus := newFakeBus()
	points := []Point{
		{ID: "a", Function: HoldingRegister, Address: 10, Format: "uint16", Kind: signal.Number},
		{ID: "b", Function: HoldingRegister, Address: 12, Format: "uint16", Kind: signal.Number},
	}
	ch, _ := New("c1", bus, []Device{device("d1", 1, points...)})
	ch.MaxGap = 1
	ch.RunOnce(context.Background(), t0)
	if len(bus.calls) != 1 || bus.calls[0] != "hr:1:10:3" {
		t.Fatalf("expected one merged block, got %v", bus.calls)
	}
}

func TestQueuedWriteRunsBeforePoll(t *testing.T) {
	bus := newFakeBus()
	point := Point{ID: "power", Function: HoldingRegister, Address: 5, Format: "uint16", Scale: 0.1, Kind: signal.Number}
	ch, _ := New("c1", bus, []Device{device("d1", 1, point)})
	if err := ch.QueueWrite("d1", "power", signal.Num(25.3), "req-1"); err != nil {
		t.Fatal(err)
	}
	_, writes := ch.RunOnce(context.Background(), t0)
	if len(writes) != 1 || writes[0].Error != "" || writes[0].RequestID != "req-1" {
		t.Fatalf("unexpected write result %+v", writes)
	}
	if bus.holding[5] != 253 {
		t.Fatalf("holding[5] = %d, want 253", bus.holding[5])
	}
	if len(bus.sequence) < 2 || bus.sequence[0] != "write" || bus.sequence[1] != "read" {
		t.Fatalf("write must run before poll: %v", bus.sequence)
	}
}

func TestWriteFailuresAreReported(t *testing.T) {
	bus := newFakeBus()
	bus.writeErr = errors.New("port closed")
	point := Point{ID: "run", Function: Coil, Address: 1, Kind: signal.Bool}
	ch, _ := New("c1", bus, []Device{device("d1", 1, point)})
	ch.QueueWrite("d1", "run", signal.Flag(true), "req-1")
	_, writes := ch.RunOnce(context.Background(), t0)
	if len(writes) != 1 || writes[0].Error == "" {
		t.Fatalf("expected write error, got %+v", writes)
	}
	if stats := ch.Stats("d1"); stats.Failures != 1 {
		t.Fatalf("write failure must be counted, got %+v", stats)
	}
	// The device still answers polls, so communication health stays healthy;
	// the write failure lives in the write result and the command state.
	if got := ch.Health("d1", t0, time.Minute); got != HealthHealthy {
		t.Fatalf("health after a failed write = %s, want healthy", got)
	}
}

func TestQueueWriteValidatesTarget(t *testing.T) {
	bus := newFakeBus()
	readOnly := Point{ID: "temp", Function: InputRegister, Address: 1, Format: "uint16", Kind: signal.Number, ReadOnly: true}
	ch, _ := New("c1", bus, []Device{device("d1", 1, readOnly)})
	if err := ch.QueueWrite("nope", "temp", signal.Num(1), "r"); err == nil {
		t.Fatal("unknown device must be rejected")
	}
	if err := ch.QueueWrite("d1", "nope", signal.Num(1), "r"); err == nil {
		t.Fatal("unknown point must be rejected")
	}
	if err := ch.QueueWrite("d1", "temp", signal.Num(1), "r"); err == nil {
		t.Fatal("read-only point must be rejected")
	}
	if err := ch.QueueWrite("d1", "temp", signal.Flag(true), "r"); err == nil {
		t.Fatal("kind mismatch must be rejected")
	}
}

func TestDecodesSignedAndFloatFormats(t *testing.T) {
	bus := newFakeBus()
	bus.holding[10] = 0xFF9C // -100, scale 0.1 => -10
	bits := math.Float32bits(12.5)
	bus.holding[12] = uint16(bits >> 16)
	bus.holding[13] = uint16(bits)
	bus.holding[14] = 0x0001
	bus.holding[15] = 0x0002
	points := []Point{
		{ID: "signed", Function: HoldingRegister, Address: 10, Format: "int16", Scale: 0.1, Kind: signal.Number},
		{ID: "float", Function: HoldingRegister, Address: 12, Format: "float32", Kind: signal.Number},
		{ID: "wide", Function: HoldingRegister, Address: 14, Format: "uint32", Kind: signal.Number},
	}
	ch, _ := New("c1", bus, []Device{device("d1", 1, points...)})
	results, _ := ch.RunOnce(context.Background(), t0)
	if result, _ := find(results, "d1", "signed"); math.Abs(result.Value.Num+10) > 1e-9 {
		t.Errorf("signed = %v, want -10", result.Value.Num)
	}
	if result, _ := find(results, "d1", "float"); math.Abs(result.Value.Num-12.5) > 1e-9 {
		t.Errorf("float = %v, want 12.5", result.Value.Num)
	}
	if result, _ := find(results, "d1", "wide"); result.Value.Num != 65538 {
		t.Errorf("wide = %v, want 65538", result.Value.Num)
	}
}

func TestHealthGoesOfflineWhenSuccessIsStale(t *testing.T) {
	bus := newFakeBus()
	ch, _ := New("c1", bus, []Device{device("d1", 1, tempPoint(10))})
	ch.RunOnce(context.Background(), t0)
	if got := ch.Health("d1", t0.Add(5*time.Second), 10*time.Second); got != HealthHealthy {
		t.Fatalf("health = %s, want healthy", got)
	}
	if got := ch.Health("d1", t0.Add(11*time.Second), 10*time.Second); got != HealthOffline {
		t.Fatalf("stale health = %s, want offline", got)
	}
}
