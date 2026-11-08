package appapi

import (
	"fmt"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// RunCapabilityDispatch 执行一次能力级调度：读 EnergyUnit 的能力值、求值
// 能力级规则、按柜内 Device 能力拆解、写每台 Device 的 valueSet 设定值。
// 这是破坏性切换里能力级模型接线的核心：把纯函数接到运行时写设备。
func (h *Handler) RunCapabilityDispatch(energyUnitID string) ([]decision.DeviceDispatch, error) {
	nodes, err := siotclient.GetNodes(h.nc, "all", energyUnitID, "", false)
	if err != nil || len(nodes) == 0 {
		return nil, fmt.Errorf("energy unit %s not found", energyUnitID)
	}
	rule, ok := loadCapabilityRule(nodes[0])
	if !ok {
		return nil, fmt.Errorf("no capability rule on %s", energyUnitID)
	}

	values, err := LoadCapabilityValues(h.nc, []string{energyUnitID})
	if err != nil {
		return nil, err
	}

	// 柜内 Device 能力：读 energyUnit 下的 device 节点，能力从 capability 点。
	devices, err := siotclient.GetNodes(h.nc, energyUnitID, "all", data.NodeTypeDevice, false)
	if err != nil {
		return nil, err
	}
	cabinet := make([]decision.DeviceCapability, 0, len(devices))
	for _, device := range devices {
		rated := 0.0
		if point, found := device.Points.Find(PointTypeRatedPowerKW, ""); found {
			rated = pointFloat(point)
		}
		available := rated
		if point, found := device.Points.Find(PointTypeCapability, ""); found {
			if capability, err := DecodeCapability(point.Txt()); err == nil && capability.PowerMaxKW > 0 {
				available = capability.PowerMaxKW
			}
		}
		cabinet = append(cabinet, decision.DeviceCapability{
			DeviceID:         device.ID,
			RatedPowerKW:     rated,
			AvailablePowerKW: available,
		})
	}

	dispatches := decision.DispatchCapability(rule, values, map[string][]decision.DeviceCapability{energyUnitID: cabinet})

	// 写 Device 设定值到 valueSet 点，触发 bridge 写设备。写目标优先取 Device
	// 的 powerSetpoint 点位（功率设定 modbusIo 节点），缺省写 Device 节点。
	for _, dispatch := range dispatches {
		if dispatch.Excluded {
			continue
		}
		targetID := dispatch.DeviceID
		if nodes, err := siotclient.GetNodes(h.nc, "all", dispatch.DeviceID, "", false); err == nil && len(nodes) > 0 {
			if point, found := nodes[0].Points.Find(PointTypePowerSetpoint, ""); found && point.Txt() != "" {
				targetID = point.Txt()
			}
		}
		// Q11 爬坡限幅：读目标节点当前设定值，按爬坡/死区推进一档再写，
		// 避免目标值突变造成设备冲击。无当前值（首次）时直接写目标。
		setpoint := dispatch.SetpointKW
		if current, ok := readCurrentSetpoint(h.nc, targetID); ok {
			setpoint = decision.RampLimit(current, dispatch.SetpointKW, decision.DefaultRampParams())
		}
		point := data.NewPointFloat(data.PointTypeValueSet, "", setpoint)
		point.Origin = "capability-dispatch"
		if err := siotclient.SendNodePoint(h.nc, targetID, point, true); err != nil {
			return nil, fmt.Errorf("write device %s: %w", dispatch.DeviceID, err)
		}
	}
	return dispatches, nil
}

// readCurrentSetpoint 读一个节点的当前设定值（valueSet 点）。无该点或非
// 数值时返回 ok=false，调用方按"无当前值"处理（直接写目标）。
func readCurrentSetpoint(nc *nats.Conn, nodeID string) (float64, bool) {
	nodes, err := siotclient.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return 0, false
	}
	point, found := nodes[0].Points.Find(data.PointTypeValueSet, "")
	if !found {
		return 0, false
	}
	return pointFloat(point), true
}
