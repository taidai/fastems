package appapi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/ems/modbusrt"
	"fastiot/internal/emsapi"
)

// ChannelAdmin persists channel configuration as modbus and modbusIo nodes, so
// the bridge restores it after restart and there is a single runtime path.
type ChannelAdmin struct {
	NC *nats.Conn
}

// NewChannelAdmin returns a configurator for the EMS API.
func NewChannelAdmin(nc *nats.Conn) ChannelAdmin { return ChannelAdmin{NC: nc} }

// Create writes the channel node and its point nodes, rolling back on failure.
func (a ChannelAdmin) Create(access emsapi.Access, input emsapi.ChannelInput) (map[string]any, error) {
	if !access.Admin {
		return nil, fmt.Errorf("admin_required")
	}
	protocol := strings.ToUpper(input.Protocol)
	if protocol == "" {
		protocol = data.PointValueTCP
	}
	connection := ModbusConnectionInput{
		Parent: input.Parent, Description: "通道 " + input.Address,
		Protocol: protocol, PollPeriod: input.PeriodMS, Timeout: input.TimeoutMS,
	}
	if connection.PollPeriod == 0 {
		connection.PollPeriod = 1000
	}
	if connection.Timeout == 0 {
		connection.Timeout = 500
	}
	switch protocol {
	case data.PointValueTCP:
		connection.URI = input.Address
	case data.PointValueRTU:
		cfg, err := modbusrt.ParseSerialAddress(input.Address)
		if err != nil {
			return nil, err
		}
		connection.Port = cfg.Port
		connection.Baud = strconv.Itoa(cfg.Baud)
		connection.DataBits = cfg.DataBits
		connection.Parity = cfg.Parity
		connection.StopBits = cfg.StopBits
		if input.Baud > 0 {
			connection.Baud = strconv.Itoa(input.Baud)
		}
	default:
		return nil, fmt.Errorf("unsupported protocol %q", input.Protocol)
	}
	normalizeSerial(&connection)
	if err := validateModbusConnection(connection); err != nil {
		return nil, err
	}

	busID := data.NewID()
	err := client.SendNodeType(a.NC, client.Modbus{
		ID: busID, Parent: input.Parent, Description: connection.Description,
		ClientServer: data.PointValueClient, Protocol: connection.Protocol,
		URI: connection.URI, Port: connection.Port, Baud: connection.Baud,
		PollPeriod: connection.PollPeriod, Timeout: connection.Timeout,
	}, access.UserID)
	if err != nil {
		return nil, fmt.Errorf("create_failed")
	}
	if err := client.SendNodePoints(a.NC, busID, serialPoints(connection), true); err != nil {
		_ = client.DeleteNode(a.NC, busID, input.Parent, access.UserID)
		return nil, fmt.Errorf("create_failed")
	}

	created := make([]data.NodeEdge, 0)
	rollback := func() {
		for _, node := range created {
			_ = client.DeleteNode(a.NC, node.ID, node.Parent, access.UserID)
		}
		_ = client.DeleteNode(a.NC, busID, input.Parent, access.UserID)
	}
	for _, device := range input.Devices {
		if device.ServerID < 1 || device.ServerID > 247 {
			rollback()
			return nil, fmt.Errorf("invalid_device")
		}
		for _, point := range device.Points {
			ioType, err := ioTypeOf(point.Function)
			if err != nil {
				rollback()
				return nil, err
			}
			format := point.Format
			if format == "" {
				format = data.PointValueUINT16
			}
			readOnly := point.ReadOnly ||
				ioType == data.PointValueModbusInputRegister ||
				ioType == data.PointValueModbusDiscreteInput
			if ioType == data.PointValueModbusCoil || ioType == data.PointValueModbusDiscreteInput {
				if point.Kind != "" && point.Kind != "bool" {
					rollback()
					return nil, fmt.Errorf("point %s: bit area requires bool", point.Name)
				}
			}
			node, err := newModbusIONode(busID, ModbusIOInput{
				Description: point.Name, ServerID: device.ServerID, Address: point.Address,
				IOType: ioType, DataFormat: format, ReadOnly: readOnly,
				Scale: point.Scale, Offset: point.Offset, Units: point.Units,
			})
			if err != nil {
				rollback()
				return nil, fmt.Errorf("invalid_point")
			}
			if err := client.SendNode(a.NC, node, access.UserID); err != nil {
				rollback()
				return nil, fmt.Errorf("create_failed")
			}
			created = append(created, node)
		}
	}
	pointIDs := make([]string, 0, len(created))
	for _, node := range created {
		pointIDs = append(pointIDs, node.ID)
	}
	return map[string]any{"channel_id": busID, "node_id": busID, "point_nodes": pointIDs}, nil
}

// Delete removes the channel node and its point nodes.
func (a ChannelAdmin) Delete(access emsapi.Access, id string) error {
	if !access.Admin {
		return fmt.Errorf("admin_required")
	}
	nodes, err := client.GetNodes(a.NC, "all", id, "", false)
	if err != nil || len(nodes) == 0 {
		return fmt.Errorf("channel_not_found")
	}
	parent := nodes[0].Parent
	children, err := client.GetNodes(a.NC, id, "all", "", false)
	if err != nil {
		return fmt.Errorf("delete_failed")
	}
	for _, child := range children {
		if err := client.DeleteNode(a.NC, child.ID, child.Parent, access.UserID); err != nil {
			return fmt.Errorf("delete_child_failed")
		}
	}
	if err := client.DeleteNode(a.NC, id, parent, access.UserID); err != nil {
		return fmt.Errorf("delete_failed")
	}
	return nil
}

func ioTypeOf(function string) (string, error) {
	switch function {
	case "coil":
		return data.PointValueModbusCoil, nil
	case "discreteInput":
		return data.PointValueModbusDiscreteInput, nil
	case "holdingRegister":
		return data.PointValueModbusHoldingRegister, nil
	case "inputRegister":
		return data.PointValueModbusInputRegister, nil
	default:
		return "", fmt.Errorf("unsupported point function %q", function)
	}
}
