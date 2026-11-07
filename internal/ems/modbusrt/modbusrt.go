// Package modbusrt adapts the upstream Modbus client to channel.Bus so the
// channel runtime speaks real RTU/TCP frames.
package modbusrt

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/simpleiot/simpleiot/modbus"
	"github.com/simpleiot/simpleiot/respreader"
	"go.bug.st/serial"
)

// Bus is one Modbus master connection.
type Bus struct {
	client *modbus.Client
}

// SerialConfig is the complete RTU serial framing configuration.
type SerialConfig struct {
	Port     string
	Baud     int
	DataBits int
	Parity   string
	StopBits int
}

func (c SerialConfig) normalized() (SerialConfig, error) {
	if c.Port == "" {
		return SerialConfig{}, fmt.Errorf("serial port is empty")
	}
	if c.Baud < 1 || c.Baud > 4_000_000 {
		return SerialConfig{}, fmt.Errorf("invalid baud %d", c.Baud)
	}
	if c.DataBits == 0 {
		c.DataBits = 8
	}
	if c.DataBits < 5 || c.DataBits > 8 {
		return SerialConfig{}, fmt.Errorf("invalid data bits %d", c.DataBits)
	}
	if c.Parity == "" {
		c.Parity = "N"
	}
	c.Parity = strings.ToUpper(c.Parity)
	switch c.Parity {
	case "N", "E", "O":
	default:
		return SerialConfig{}, fmt.Errorf("invalid parity %q", c.Parity)
	}
	if c.StopBits == 0 {
		c.StopBits = 1
	}
	if c.StopBits != 1 && c.StopBits != 2 {
		return SerialConfig{}, fmt.Errorf("invalid stop bits %d", c.StopBits)
	}
	return c, nil
}

// ParseSerialAddress parses port@baud or port@baud@data@parity@stop.
func ParseSerialAddress(address string) (SerialConfig, error) {
	parts := strings.Split(address, "@")
	if len(parts) < 2 || len(parts) > 5 {
		return SerialConfig{}, fmt.Errorf("rtu address must be port@baud[@data@parity@stop]")
	}
	cfg := SerialConfig{Port: parts[0], DataBits: 8, Parity: "N", StopBits: 1}
	baud, err := strconv.Atoi(parts[1])
	if err != nil {
		return SerialConfig{}, fmt.Errorf("invalid baud %q", parts[1])
	}
	cfg.Baud = baud
	if len(parts) > 2 && parts[2] != "" {
		if cfg.DataBits, err = strconv.Atoi(parts[2]); err != nil {
			return SerialConfig{}, fmt.Errorf("invalid data bits %q", parts[2])
		}
	}
	if len(parts) > 3 && parts[3] != "" {
		cfg.Parity = parts[3]
	}
	if len(parts) > 4 && parts[4] != "" {
		if cfg.StopBits, err = strconv.Atoi(parts[4]); err != nil {
			return SerialConfig{}, fmt.Errorf("invalid stop bits %q", parts[4])
		}
	}
	return cfg.normalized()
}

func (c SerialConfig) mode() (*serial.Mode, error) {
	normalized, err := c.normalized()
	if err != nil {
		return nil, err
	}
	mode := &serial.Mode{BaudRate: normalized.Baud, DataBits: normalized.DataBits, StopBits: serial.OneStopBit}
	switch normalized.Parity {
	case "E":
		mode.Parity = serial.EvenParity
	case "O":
		mode.Parity = serial.OddParity
	default:
		mode.Parity = serial.NoParity
	}
	if normalized.StopBits == 2 {
		mode.StopBits = serial.TwoStopBits
	}
	return mode, nil
}

// New wraps an existing transport, used by tests and by the RTU/TCP openers.
func New(transport modbus.Transport) *Bus {
	return &Bus{client: modbus.NewClient(transport, 0)}
}

// OpenRTUConfig opens a serial port with explicit framing parameters.
func OpenRTUConfig(cfg SerialConfig, timeout time.Duration) (*Bus, error) {
	mode, err := cfg.mode()
	if err != nil {
		return nil, err
	}
	port, err := serial.Open(cfg.Port, mode)
	if err != nil {
		return nil, err
	}
	reader := respreader.NewReadWriteCloser(port, timeout, 20*time.Millisecond)
	return New(&recordingTransport{Transport: modbus.NewRTU(reader), peer: cfg.Port}), nil
}

// OpenRTU opens a serial port with 8N1 for callers without explicit framing.
func OpenRTU(portName string, baud int, timeout time.Duration) (*Bus, error) {
	return OpenRTUConfig(SerialConfig{Port: portName, Baud: baud}, timeout)
}

// OpenTCP dials a Modbus TCP server.
func OpenTCP(address string, timeout time.Duration) (*Bus, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	return New(&recordingTransport{Transport: modbus.NewTCP(conn, timeout, modbus.TransportClient), peer: address}), nil
}

func (b *Bus) ReadCoils(server byte, address, count uint16) ([]bool, error) {
	return b.client.ReadCoils(server, address, count)
}

func (b *Bus) ReadDiscreteInputs(server byte, address, count uint16) ([]bool, error) {
	return b.client.ReadDiscreteInputs(server, address, count)
}

func (b *Bus) ReadHoldingRegisters(server byte, address, count uint16) ([]uint16, error) {
	return b.client.ReadHoldingRegs(server, address, count)
}

func (b *Bus) ReadInputRegisters(server byte, address, count uint16) ([]uint16, error) {
	return b.client.ReadInputRegs(server, address, count)
}

func (b *Bus) WriteCoil(server byte, address uint16, value bool) error {
	return b.client.WriteSingleCoil(server, address, value)
}

func (b *Bus) WriteHoldingRegister(server byte, address, value uint16) error {
	return b.client.WriteSingleReg(server, address, value)
}

// Close releases the transport.
func (b *Bus) Close() error { return b.client.Close() }
