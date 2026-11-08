package appapi

import (
	"fmt"

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
		point := data.NewPointFloat(data.PointTypeValueSet, "", dispatch.SetpointKW)
		point.Origin = "capability-dispatch"
		if err := siotclient.SendNodePoint(h.nc, targetID, point, true); err != nil {
			return nil, fmt.Errorf("write device %s: %w", dispatch.DeviceID, err)
		}
	}
	return dispatches, nil
}
