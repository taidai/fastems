package bridge

import (
	"sync"
	"testing"
	"time"

	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/ems/channel"
	"fastems/internal/ems/runtime"
	"fastems/internal/ems/signal"
)

func node(id, typ string, numbers map[string]float64, texts map[string]string) data.NodeEdge {
	n := data.NodeEdge{ID: id, Type: typ}
	for name, value := range numbers {
		n.Points = append(n.Points, data.NewPointFloat(name, "0", value))
	}
	for name, value := range texts {
		n.Points = append(n.Points, data.NewPointString(name, "0", value))
	}
	return n
}

type fakeBus struct {
	mu     sync.Mutex
	writes int
}

func (f *fakeBus) ReadCoils(_ byte, _, count uint16) ([]bool, error) {
	return make([]bool, count), nil
}

func (f *fakeBus) ReadDiscreteInputs(_ byte, _, count uint16) ([]bool, error) {
	return make([]bool, count), nil
}

func (f *fakeBus) ReadHoldingRegisters(_ byte, _, count uint16) ([]uint16, error) {
	return make([]uint16, count), nil
}

func (f *fakeBus) ReadInputRegisters(_ byte, _, count uint16) ([]uint16, error) {
	return make([]uint16, count), nil
}

func (f *fakeBus) WriteCoil(_ byte, _ uint16, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	return nil
}

func (f *fakeBus) WriteHoldingRegister(_ byte, _ uint16, _ uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	return nil
}

func (f *fakeBus) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

type fakeOpener struct {
	mu    sync.Mutex
	bus   *fakeBus
	count int
}

func (o *fakeOpener) open(string, string, time.Duration) (channel.Bus, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.count++
	return o.bus, nil
}

func (o *fakeOpener) opens() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.count
}

type fakeSource struct {
	mu     sync.Mutex
	buses  []BusConfig
	sets   map[string]ValueSet
	values map[string]signal.Value
	health map[string]time.Time
}

func newFakeSource(buses ...BusConfig) *fakeSource {
	return &fakeSource{
		buses: buses, sets: map[string]ValueSet{},
		values: map[string]signal.Value{}, health: map[string]time.Time{},
	}
}

func (f *fakeSource) Buses() ([]BusConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]BusConfig(nil), f.buses...), nil
}

func (f *fakeSource) ValueSets() (map[string]ValueSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]ValueSet, len(f.sets))
	for k, v := range f.sets {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSource) PublishValue(id string, value signal.Value) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[id] = value
	return nil
}

func (f *fakeSource) PublishHealth(channelID string, last time.Time, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.health[channelID] = last
	return nil
}

func (f *fakeSource) PublishDeviceHealth(ioNodeID, health string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values["health:"+ioNodeID] = signal.Label(health)
	return nil
}

func (f *fakeSource) setBuses(buses ...BusConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.buses = buses
}

func (f *fakeSource) setValue(ioID string, set ValueSet) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets[ioID] = set
}

func (f *fakeSource) valueOf(ioID string) (signal.Value, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.values[ioID]
	return value, ok
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

func testConfig(address string) BusConfig {
	return BusConfig{
		ChannelID: "bus1", Protocol: "tcp", Address: address, Period: 10 * time.Millisecond,
		Devices: []channel.Device{{
			ID: "bus1/1", ServerID: 1,
			Points: []channel.Point{{
				ID: "io1", Function: channel.HoldingRegister, Address: 10,
				Format: "uint16", Kind: signal.Number,
			}},
		}},
	}
}

func TestSyncBuildsAndRebuildsChannels(t *testing.T) {
	source := newFakeSource(testConfig("127.0.0.1:1502"))
	opener := &fakeOpener{bus: &fakeBus{}}
	service := runtime.NewService(nil, time.Second, time.Second, 100)
	defer service.Close()
	b := New(service, source, opener.open)

	if err := b.Sync(time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(service.Channels()) != 1 || opener.opens() != 1 {
		t.Fatalf("channels=%d opens=%d", len(service.Channels()), opener.opens())
	}
	waitFor(t, func() bool {
		_, ok := source.valueOf("io1")
		return ok
	})

	if err := b.Sync(time.Now()); err != nil {
		t.Fatal(err)
	}
	if opener.opens() != 1 {
		t.Fatalf("unchanged config reopened the bus: %d", opener.opens())
	}

	source.setBuses(testConfig("127.0.0.1:1503"))
	if err := b.Sync(time.Now()); err != nil {
		t.Fatal(err)
	}
	if opener.opens() != 2 || len(service.Channels()) != 1 {
		t.Fatalf("changed config: opens=%d channels=%d", opener.opens(), len(service.Channels()))
	}

	source.setBuses()
	if err := b.Sync(time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(service.Channels()) != 0 {
		t.Fatal("removed node still has a running channel")
	}
}

func TestValueSetWritesOnlyAfterBaseline(t *testing.T) {
	bus := &fakeBus{}
	source := newFakeSource(testConfig("127.0.0.1:1502"))
	opener := &fakeOpener{bus: bus}
	service := runtime.NewService(nil, time.Second, time.Second, 100)
	defer service.Close()
	b := New(service, source, opener.open)

	baseline := time.Now()
	source.setValue("io1", ValueSet{Value: signal.Num(42), At: baseline})
	if err := b.Sync(time.Now()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	if bus.writeCount() != 0 {
		t.Fatalf("baseline valueSet wrote %d times", bus.writeCount())
	}

	source.setValue("io1", ValueSet{Value: signal.Num(42), At: baseline.Add(time.Second)})
	if err := b.Sync(time.Now()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return bus.writeCount() > 0 })
}

func TestHealthIsPublished(t *testing.T) {
	source := newFakeSource(testConfig("127.0.0.1:1502"))
	opener := &fakeOpener{bus: &fakeBus{}}
	service := runtime.NewService(nil, time.Second, time.Second, 100)
	defer service.Close()
	b := New(service, source, opener.open)
	if err := b.Sync(time.Now()); err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	_, ok := source.health["bus1"]
	source.mu.Unlock()
	if !ok {
		t.Fatal("health was not published")
	}
}

func TestBuildBusesFromNodes(t *testing.T) {
	bus := node("bus1", "modbus", map[string]float64{
		"pollPeriod": 500, "disabled": 0,
	}, map[string]string{
		"protocol": "TCP", "uri": "127.0.0.1:1502", "clientServer": "client",
	})
	io := node("io1", "modbusIo", map[string]float64{
		"id": 1, "address": 10, "scale": 0.1, "readOnly": 1,
	}, map[string]string{
		"modbusIoType": "modbusHoldingRegister", "dataFormat": "uint16",
	})
	io.Parent = "bus1"

	configs := buildBuses([]data.NodeEdge{bus}, []data.NodeEdge{io})
	if len(configs) != 1 {
		t.Fatalf("configs = %d", len(configs))
	}
	config := configs[0]
	if config.Protocol != "tcp" || config.Address != "127.0.0.1:1502" || config.Period != 500*time.Millisecond {
		t.Fatalf("unexpected config %+v", config)
	}
	if len(config.Devices) != 1 || len(config.Devices[0].Points) != 1 {
		t.Fatalf("unexpected devices %+v", config.Devices)
	}
	point := config.Devices[0].Points[0]
	if point.ID != "io1" || point.Address != 10 || point.Scale != 0.1 || !point.ReadOnly || point.Kind != signal.Number {
		t.Fatalf("unexpected point %+v", point)
	}
}

func TestBuildBusesSkipsServerAndDisabled(t *testing.T) {
	server := node("bus2", "modbus", map[string]float64{}, map[string]string{
		"protocol": "TCP", "uri": "127.0.0.1:1502", "clientServer": "server",
	})
	disabled := node("bus3", "modbus", map[string]float64{"disabled": 1}, map[string]string{
		"protocol": "TCP", "uri": "127.0.0.1:1503", "clientServer": "client",
	})
	if configs := buildBuses([]data.NodeEdge{server, disabled}, nil); len(configs) != 0 {
		t.Fatalf("unexpected configs %+v", configs)
	}
}

func TestBuildBusesEncodesSerialFraming(t *testing.T) {
	bus := node("bus-rtu", "modbus", map[string]float64{
		"dataBits": 7, "stopBits": 2, "pollPeriod": 500,
	}, map[string]string{
		"protocol": "RTU", "port": "COM3", "baud": "19200", "parity": "E", "clientServer": "client",
	})
	io := node("io-rtu", "modbusIo", map[string]float64{
		"id": 1, "address": 10, "scale": 1, "readOnly": 1,
	}, map[string]string{
		"modbusIoType": "modbusHoldingRegister", "dataFormat": "uint16",
	})
	io.Parent = "bus-rtu"
	configs := buildBuses([]data.NodeEdge{bus}, []data.NodeEdge{io})
	if len(configs) != 1 || configs[0].Protocol != "rtu" {
		t.Fatalf("configs = %+v", configs)
	}
	if configs[0].Address != "COM3@19200@7@E@2" {
		t.Fatalf("address = %q", configs[0].Address)
	}
}
