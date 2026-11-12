package modbusrt

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"fastiot/internal/ems/channel"
	"fastiot/internal/ems/signal"

	"github.com/simpleiot/simpleiot/modbus"
	"github.com/simpleiot/simpleiot/respreader"
	"github.com/simpleiot/simpleiot/test"
	"go.bug.st/serial"
)

func newRTUBus(t *testing.T, wire *test.IoSim) *Bus {
	t.Helper()
	port := respreader.NewReadWriteCloser(wire, 2*time.Second, 5*time.Millisecond)
	bus := New(modbus.NewRTU(port))
	t.Cleanup(func() { _ = bus.Close() })
	return bus
}

func find(results []channel.Result, pointID string) (channel.Result, bool) {
	for _, result := range results {
		if result.PointID == pointID {
			return result, true
		}
	}
	return channel.Result{}, false
}

func TestRealRtuFramesReadAndWrite(t *testing.T) {
	slaveWire, masterWire := test.NewIoSim()
	regs := &modbus.Regs{}
	regs.AddReg(10, 3)
	if err := regs.WriteReg(10, 351); err != nil {
		t.Fatal(err)
	}
	if err := regs.WriteReg(11, 402); err != nil {
		t.Fatal(err)
	}
	portA := respreader.NewReadWriteCloser(slaveWire, 2*time.Second, 5*time.Millisecond)
	slave := modbus.NewServer(1, modbus.NewRTU(portA), regs, 0)
	go slave.Listen(func(error) {}, func() {}, func() {})
	t.Cleanup(func() { _ = slave.Close() })

	bus := newRTUBus(t, masterWire)
	ch, err := channel.New("rtu-1", bus, []channel.Device{{
		ID: "meter", ServerID: 1,
		Points: []channel.Point{
			{ID: "voltage", Function: channel.HoldingRegister, Address: 10, Format: "uint16", Kind: signal.Number},
			{ID: "current", Function: channel.HoldingRegister, Address: 11, Format: "uint16", Kind: signal.Number},
			{ID: "setpoint", Function: channel.HoldingRegister, Address: 12, Format: "uint16", Scale: 0.1, Kind: signal.Number},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	results, _ := ch.RunOnce(context.Background(), time.Now())
	if result, ok := find(results, "voltage"); !ok || result.Value.Num != 351 {
		t.Fatalf("voltage = %+v, want 351", result)
	}
	if result, ok := find(results, "current"); !ok || result.Value.Num != 402 {
		t.Fatalf("current = %+v, want 402", result)
	}
	if got := ch.Health("meter", time.Now(), time.Minute); got != channel.HealthHealthy {
		t.Fatalf("health = %s, want healthy", got)
	}

	if err := ch.QueueWrite("meter", "setpoint", signal.Num(25.3), "req-1"); err != nil {
		t.Fatalf("queue write: %v", err)
	}
	_, writes := ch.RunOnce(context.Background(), time.Now().Add(time.Second))
	if len(writes) != 1 || writes[0].Error != "" {
		t.Fatalf("write failed: %+v", writes)
	}
	raw, err := regs.ReadReg(12)
	if err != nil || raw != 253 {
		t.Fatalf("register 12 = %d err=%v, want 253", raw, err)
	}
}

func TestMissingSlaveDoesNotBreakOtherDevices(t *testing.T) {
	slaveWire, masterWire := test.NewIoSim()
	regs := &modbus.Regs{}
	regs.AddReg(5, 1)
	if err := regs.WriteReg(5, 1234); err != nil {
		t.Fatal(err)
	}
	portA := respreader.NewReadWriteCloser(slaveWire, 300*time.Millisecond, 5*time.Millisecond)
	slave := modbus.NewServer(1, modbus.NewRTU(portA), regs, 0)
	go slave.Listen(func(error) {}, func() {}, func() {})
	t.Cleanup(func() { _ = slave.Close() })

	bus := newRTUBus(t, masterWire)
	ch, err := channel.New("rtu-1", bus, []channel.Device{
		{ID: "present", ServerID: 1, Points: []channel.Point{
			{ID: "value", Function: channel.HoldingRegister, Address: 5, Format: "uint16", Kind: signal.Number},
		}},
		{ID: "absent", ServerID: 9, Points: []channel.Point{
			{ID: "value", Function: channel.HoldingRegister, Address: 5, Format: "uint16", Kind: signal.Number},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	results, _ := ch.RunOnce(context.Background(), time.Now())
	if result, ok := find(results, "value"); !ok || result.DeviceID != "present" || result.Value.Num != 1234 {
		t.Fatalf("present device result = %+v", result)
	}
	if stats := ch.Stats("absent"); stats.Failures == 0 {
		t.Fatal("absent slave must record a failure")
	}
	if got := ch.Health("present", time.Now(), time.Minute); got != channel.HealthHealthy {
		t.Fatalf("present device health = %s, want healthy", got)
	}
}

func TestRealTCPFrames(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	regs := &modbus.Regs{}
	regs.AddReg(1, 2)
	if err := regs.WriteReg(1, 42); err != nil {
		t.Fatal(err)
	}
	server, err := modbus.NewTCPServer(1, 1, strconv.Itoa(port), regs, 0)
	if err != nil {
		t.Fatal(err)
	}
	go server.Listen(func(error) {}, func() {}, func() {})
	t.Cleanup(func() { _ = server.Close() })

	bus, err := OpenTCP(fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Close() })

	ch, err := channel.New("tcp-1", bus, []channel.Device{{
		ID: "meter", ServerID: 1,
		Points: []channel.Point{
			{ID: "value", Function: channel.HoldingRegister, Address: 1, Format: "uint16", Kind: signal.Number},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	results, _ := ch.RunOnce(context.Background(), time.Now())
	if result, ok := find(results, "value"); !ok || result.Value.Num != 42 {
		t.Fatalf("tcp read = %+v ok=%v, want 42", result, ok)
	}
}

func TestParseSerialAddress(t *testing.T) {
	cases := []struct {
		address  string
		port     string
		baud     int
		dataBits int
		parity   string
		stopBits int
	}{
		{"COM3@9600", "COM3", 9600, 8, "N", 1},
		{"COM3@19200@7@E@2", "COM3", 19200, 7, "E", 2},
		{"/dev/ttyUSB0@115200@8@O@1", "/dev/ttyUSB0", 115200, 8, "O", 1},
	}
	for _, tc := range cases {
		cfg, err := ParseSerialAddress(tc.address)
		if err != nil {
			t.Fatalf("%s: %v", tc.address, err)
		}
		if cfg.Port != tc.port || cfg.Baud != tc.baud || cfg.DataBits != tc.dataBits || cfg.Parity != tc.parity || cfg.StopBits != tc.stopBits {
			t.Fatalf("%s: got %+v", tc.address, cfg)
		}
		mode, err := cfg.mode()
		if err != nil {
			t.Fatalf("%s: %v", tc.address, err)
		}
		if mode.BaudRate != tc.baud || mode.DataBits != tc.dataBits {
			t.Fatalf("%s: mode %+v", tc.address, mode)
		}
		switch tc.parity {
		case "E":
			if mode.Parity != serial.EvenParity {
				t.Fatalf("%s: parity %v", tc.address, mode.Parity)
			}
		case "O":
			if mode.Parity != serial.OddParity {
				t.Fatalf("%s: parity %v", tc.address, mode.Parity)
			}
		default:
			if mode.Parity != serial.NoParity {
				t.Fatalf("%s: parity %v", tc.address, mode.Parity)
			}
		}
		if tc.stopBits == 2 && mode.StopBits != serial.TwoStopBits {
			t.Fatalf("%s: stop bits %v", tc.address, mode.StopBits)
		}
	}
}

func TestParseSerialAddressRejectsInvalid(t *testing.T) {
	for _, address := range []string{"COM3", "COM3@abc", "COM3@9600@9@N@1", "COM3@9600@8@X@1", "COM3@9600@8@N@3"} {
		if _, err := ParseSerialAddress(address); err == nil {
			t.Fatalf("%s: expected error", address)
		}
	}
}
