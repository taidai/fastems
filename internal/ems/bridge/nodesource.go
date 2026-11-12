package bridge

import (
	"fmt"
	"sort"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/ems/channel"
	"fastems/internal/ems/signal"
)

// NodeSource reads Modbus configuration from product nodes and writes
// observations back to the same nodes, so configuration stays in JetStream and
// there is only one runtime path.
type NodeSource struct {
	NC     *nats.Conn
	Origin string
}

// NewNodeSource returns a source backed by the embedded store.
func NewNodeSource(nc *nats.Conn) *NodeSource {
	return &NodeSource{NC: nc, Origin: "easyems-runtime"}
}

// Buses reads modbus and modbusIo nodes and builds channel configurations.
func (s *NodeSource) Buses() ([]BusConfig, error) {
	buses, err := client.GetNodes(s.NC, "all", "all", data.NodeTypeModbus, false)
	if err != nil {
		return nil, err
	}
	ios, err := client.GetNodes(s.NC, "all", "all", data.NodeTypeModbusIO, false)
	if err != nil {
		return nil, err
	}
	return buildBuses(buses, ios), nil
}

func buildBuses(buses, ios []data.NodeEdge) []BusConfig {
	children := make(map[string][]data.NodeEdge)
	for _, io := range ios {
		children[io.Parent] = append(children[io.Parent], io)
	}
	configs := make([]BusConfig, 0, len(buses))
	for _, bus := range buses {
		if number(bus.Points, data.PointTypeDisabled) != 0 {
			continue
		}
		if text(bus.Points, data.PointTypeClientServer) == data.PointValueServer {
			continue
		}
		protocol := text(bus.Points, data.PointTypeProtocol)
		address := ""
		switch protocol {
		case data.PointValueTCP:
			address = text(bus.Points, data.PointTypeURI)
			protocol = "tcp"
		case data.PointValueRTU:
			dataBits := int(number(bus.Points, "dataBits"))
			if dataBits == 0 {
				dataBits = 8
			}
			parity := text(bus.Points, "parity")
			if parity == "" {
				parity = "N"
			}
			stopBits := int(number(bus.Points, "stopBits"))
			if stopBits == 0 {
				stopBits = 1
			}
			address = fmt.Sprintf("%s@%s@%d@%s@%d",
				text(bus.Points, data.PointTypePort), text(bus.Points, data.PointTypeBaud),
				dataBits, parity, stopBits)
			protocol = "rtu"
		default:
			continue
		}
		if address == "" {
			continue
		}
		period := time.Duration(number(bus.Points, data.PointTypePollPeriod)) * time.Millisecond
		if period <= 0 {
			period = time.Second
		}
		devices := buildDevices(bus.ID, children[bus.ID])
		if len(devices) == 0 {
			continue
		}
		configs = append(configs, BusConfig{
			ChannelID: bus.ID, Protocol: protocol, Address: address, Period: period, Devices: devices,
		})
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].ChannelID < configs[j].ChannelID })
	return configs
}

func buildDevices(busID string, nodes []data.NodeEdge) []channel.Device {
	byServer := make(map[int][]channel.Point)
	for _, node := range nodes {
		if number(node.Points, data.PointTypeDisabled) != 0 {
			continue
		}
		server := int(number(node.Points, data.PointTypeID))
		if server < 1 || server > 247 {
			continue
		}
		point, ok := buildPoint(node)
		if !ok {
			continue
		}
		byServer[server] = append(byServer[server], point)
	}
	servers := make([]int, 0, len(byServer))
	for server := range byServer {
		servers = append(servers, server)
	}
	sort.Ints(servers)
	devices := make([]channel.Device, 0, len(servers))
	for _, server := range servers {
		points := byServer[server]
		sort.Slice(points, func(i, j int) bool { return points[i].Address < points[j].Address })
		devices = append(devices, channel.Device{
			ID: fmt.Sprintf("%s/%d", busID, server), ServerID: byte(server), Points: points,
		})
	}
	return devices
}

func buildPoint(node data.NodeEdge) (channel.Point, bool) {
	function := channel.Function("")
	switch text(node.Points, data.PointTypeModbusIOType) {
	case data.PointValueModbusCoil:
		function = channel.Coil
	case data.PointValueModbusDiscreteInput:
		function = channel.DiscreteInput
	case data.PointValueModbusHoldingRegister:
		function = channel.HoldingRegister
	case data.PointValueModbusInputRegister:
		function = channel.InputRegister
	default:
		return channel.Point{}, false
	}
	kind := signal.Number
	if function == channel.Coil || function == channel.DiscreteInput {
		kind = signal.Bool
	}
	address := int(number(node.Points, data.PointTypeAddress))
	if address < 0 || address > 65535 {
		return channel.Point{}, false
	}
	return channel.Point{
		ID: node.ID, Name: text(node.Points, data.PointTypeDescription),
		Function: function, Address: uint16(address),
		Format: text(node.Points, data.PointTypeDataFormat),
		Scale:  number(node.Points, data.PointTypeScale),
		Offset: number(node.Points, data.PointTypeOffset),
		Kind:   kind, ReadOnly: number(node.Points, data.PointTypeReadOnly) != 0,
	}, true
}

// ValueSets reads the latest valueSet point of each IO node.
func (s *NodeSource) ValueSets() (map[string]ValueSet, error) {
	nodes, err := client.GetNodes(s.NC, "all", "all", data.NodeTypeModbusIO, false)
	if err != nil {
		return nil, err
	}
	sets := make(map[string]ValueSet, len(nodes))
	for _, node := range nodes {
		point, ok := node.Points.Find(data.PointTypeValueSet, "")
		if !ok || point.Tombstone%2 != 0 {
			continue
		}
		value, err := point.ValueFloat()
		if err != nil {
			continue
		}
		sets[node.ID] = ValueSet{Value: signal.Num(value), At: point.Time}
	}
	return sets, nil
}

// PublishValue writes a read value back to the IO node.
func (s *NodeSource) PublishValue(ioNodeID string, value signal.Value) error {
	number, ok := asFloat(value)
	if !ok {
		return fmt.Errorf("point %s: cannot publish %s", ioNodeID, value.Kind)
	}
	point := data.NewPointFloat(data.PointTypeValue, "", number)
	point.Origin = s.Origin
	point.Time = time.Now()
	return client.SendNodePoint(s.NC, ioNodeID, point, true)
}

// PublishHealth updates the channel node's lastGoodPoll and errorCount so the
// existing channel page reflects the new runtime.
func (s *NodeSource) PublishHealth(channelID string, lastGood time.Time, failures int) error {
	last := data.NewPointFloat("lastGoodPoll", "", float64(lastGood.Unix()))
	errors := data.NewPointFloat(data.PointTypeErrorCount, "", float64(failures))
	points := data.Points{last, errors}
	for i := range points {
		points[i].Origin = s.Origin
		points[i].Time = time.Now()
	}
	return client.SendNodePoints(s.NC, channelID, points, true)
}

// PublishDeviceHealth records one device's health on each of its points. The
// allocator reads this instead of treating a bus heartbeat as device health.
func (s *NodeSource) PublishDeviceHealth(ioNodeID, health string) error {
	point := data.NewPointString("health", "", health)
	point.Origin = s.Origin
	point.Time = time.Now()
	return client.SendNodePoint(s.NC, ioNodeID, point, true)
}

func asFloat(value signal.Value) (float64, bool) {
	switch value.Kind {
	case signal.Number:
		return value.Num, true
	case signal.Bool:
		if value.Bool {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func text(points data.Points, typ string) string {
	value, _ := points.Text(typ, "")
	return value
}

func number(points data.Points, typ string) float64 {
	value, _ := points.Value(typ, "")
	return value
}
