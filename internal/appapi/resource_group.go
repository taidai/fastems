package appapi

import (
	"encoding/json"
	"fmt"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/decision"
)

// ResourceGroup 的运行时接线：读组配置（成员 + 共享上限），分配后把各成员
// 的功率请求拆解到柜内设备并写 valueSet。让组共享约束真正落到设备。

// loadResourceGroup 读一个 resourceGroup 节点的成员与共享上限。
func loadResourceGroup(node data.NodeEdge) (decision.ResourceGroup, bool) {
	group := decision.ResourceGroup{GroupID: node.ID}
	if point, found := node.Points.Find(PointTypeSharedLimitKW, ""); found {
		group.SharedLimitKW = pointFloat(point)
	}
	point, found := node.Points.Find(PointTypeGroupMembers, "")
	if !found || point.Txt() == "" {
		return group, false
	}
	var ids []string
	if json.Unmarshal([]byte(point.Txt()), &ids) != nil || len(ids) == 0 {
		return group, false
	}
	for _, id := range ids {
		group.Members = append(group.Members, decision.EnergyMember{EnergyUnitID: id})
	}
	return group, true
}

// RunGroupDispatch 在组共享上限下分配 requestKW，并写各成员设备。
func (h *Handler) RunGroupDispatch(groupID string, requestKW float64) (decision.GroupAllocation, error) {
	nodes, err := siotclient.GetNodes(h.nc, "all", groupID, "", false)
	if err != nil || len(nodes) == 0 {
		return decision.GroupAllocation{}, fmt.Errorf("resource group %s not found", groupID)
	}
	group, ok := loadResourceGroup(nodes[0])
	if !ok {
		return decision.GroupAllocation{}, fmt.Errorf("resource group %s has no members", groupID)
	}
	ids := make([]string, 0, len(group.Members))
	for _, m := range group.Members {
		ids = append(ids, m.EnergyUnitID)
	}
	caps, err := LoadCapabilityValues(h.nc, ids)
	if err != nil {
		return decision.GroupAllocation{}, err
	}
	result, err := decision.AllocateWithGroupLimit(requestKW, group, caps)
	if err != nil {
		return decision.GroupAllocation{}, err
	}
	// 写各成员设备（复用柜内拆解 + 爬坡）。
	for _, allocation := range result.Allocations {
		if allocation.Excluded {
			continue
		}
		if _, _, err := h.writeEnergyUnitPower(allocation.EnergyUnitID, allocation.SetpointKW, dispatchTrace{ID: decision.NewDispatchID(), Source: "group"}); err != nil {
			return result, err
		}
	}
	return result, nil
}

// writeEnergyUnitPower 把一个 EnergyUnit 的功率请求拆解到柜内 Device 并写
// 设备（含爬坡限幅）。优先走共享命令管理器；没有命令运行时的单元测试
// 回退到 valueSet 路径。返回拆解结果，供追溯记录使用。
func (h *Handler) writeEnergyUnitPower(energyUnitID string, requestKW float64, trace dispatchTrace) ([]decision.DeviceDispatch, string, error) {
	unitNodes, err := siotclient.GetNodes(h.nc, "all", energyUnitID, "", false)
	if err != nil || len(unitNodes) == 0 || unitNodes[0].Type != NodeTypeEnergyUnit {
		return nil, "", fmt.Errorf("energy unit %s not found", energyUnitID)
	}
	unit := unitNodes[0]
	if !AllowsEnergyUnitAutoDispatch(unit) {
		return nil, "", fmt.Errorf("energy unit %s is interlocked or manually controlled", energyUnitID)
	}
	if point, found := unit.Points.Find(PointTypeCapability, ""); found {
		capability, err := DecodeCapability(point.Txt())
		if err == nil {
			if !capability.Allowed {
				return nil, "", fmt.Errorf("energy unit %s has no available capability", energyUnitID)
			}
			available := capability.PowerMaxKW
			if requestKW >= 0 && capability.DischargeMaxKW > 0 {
				available = capability.DischargeMaxKW
			}
			if requestKW < 0 && capability.ChargeMaxKW > 0 {
				available = capability.ChargeMaxKW
			}
			if (requestKW >= 0 && capability.Direction == "charge") || (requestKW < 0 && capability.Direction == "discharge") || available <= 0 {
				return nil, "", fmt.Errorf("energy unit %s capability direction is unavailable", energyUnitID)
			}
			if requestKW > available {
				requestKW = available
			}
			if requestKW < -available {
				requestKW = -available
			}
		}
	}
	inputs, _ := loadCapabilityInputs(unit)
	ramp := inputs.rampParams()
	devices, err := siotclient.GetNodes(h.nc, energyUnitID, "all", data.NodeTypeDevice, false)
	if err != nil {
		return nil, "", err
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
		cabinet = append(cabinet, decision.DeviceCapability{DeviceID: device.ID, RatedPowerKW: rated, AvailablePowerKW: available})
	}
	usedCommand := false
	dispatches, err := decision.DispatchCabinet(requestKW, cabinet)
	if err != nil {
		return nil, "", err
	}
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
		setpoint := dispatch.SetpointKW
		if current, ok := readCurrentSetpoint(h.nc, targetID); ok {
			setpoint = decision.RampLimit(current, dispatch.SetpointKW, ramp)
		}
		handled, err := h.submitPointCommand(trace, targetID, setpoint)
		if err != nil {
			return dispatches, "", fmt.Errorf("command device %s: %w", dispatch.DeviceID, err)
		}
		if handled {
			usedCommand = true
			continue
		}
		point := data.NewPointFloat(data.PointTypeValueSet, "", setpoint)
		point.Origin = "easyems-" + trace.Source
		if err := siotclient.SendNodePoint(h.nc, targetID, point, true); err != nil {
			return dispatches, "", fmt.Errorf("write device %s: %w", dispatch.DeviceID, err)
		}
	}
	stage := "valueSet_written"
	if usedCommand {
		stage = "submitted"
	}
	return dispatches, stage, nil
}
