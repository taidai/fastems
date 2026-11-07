package decision

import (
	"fmt"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// ValidateTarget is the shared safety boundary for automatic actions and
// publication. It rejects arbitrary node types, point types, read-only
// outputs and targets outside the table's parent subtree.
func ValidateTarget(nc *nats.Conn, parent string, target Target, output bool, value *float64) error {
	if target.NodeID == "" {
		return fmt.Errorf("missing node_id")
	}
	pointType := target.PointType
	if pointType == "" {
		if output {
			pointType = data.PointTypeValueSet
		} else {
			pointType = data.PointTypeValue
		}
	}
	if output && pointType != data.PointTypeValueSet || !output && pointType != data.PointTypeValue {
		return fmt.Errorf("unsupported point_type %q", pointType)
	}
	nodes, err := siotclient.GetNodes(nc, "all", target.NodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return fmt.Errorf("target unavailable")
	}
	nodeType := nodes[0].Type
	if nodeType == data.NodeTypeGroup && output {
		config, err := LoadAllocator(nc, target.NodeID)
		if err != nil {
			return fmt.Errorf("target type %s is not controllable", nodeType)
		}
		if err := ValidateAllocatorScope(nc, target.NodeID, config); err != nil {
			return err
		}
	} else if nodeType != data.NodeTypeVariable && nodeType != data.NodeTypeModbusIO {
		return fmt.Errorf("target type %s is not controllable", nodeType)
	}
	if output {
		disabled, _ := nodes[0].Points.Value(data.PointTypeDisabled, "")
		if disabled != 0 {
			return fmt.Errorf("target is disabled")
		}
	}
	if output && nodeType == data.NodeTypeModbusIO {
		readOnly, _ := nodes[0].Points.Value(data.PointTypeReadOnly, "")
		ioType, _ := nodes[0].Points.Text(data.PointTypeModbusIOType, "")
		if readOnly != 0 || ioType != data.PointValueModbusCoil && ioType != data.PointValueModbusHoldingRegister {
			return fmt.Errorf("target is read-only")
		}
		if ioType == data.PointValueModbusCoil && value != nil && *value != 0 && *value != 1 {
			return fmt.Errorf("coil value must be 0 or 1")
		}
	}

	seen := map[string]bool{}
	frontier := []string{}
	for _, node := range nodes {
		frontier = append(frontier, node.Parent)
	}
	for len(frontier) > 0 {
		id := frontier[0]
		frontier = frontier[1:]
		if id == parent {
			return nil
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		parents, err := siotclient.GetNodes(nc, "all", id, "", false)
		if err != nil {
			return fmt.Errorf("target scope unavailable")
		}
		for _, node := range parents {
			frontier = append(frontier, node.Parent)
		}
	}
	return fmt.Errorf("target is outside the table scope")
}
