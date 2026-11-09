package appapi

import (
	"fmt"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"

	"easyems/internal/decision"
)

// RunPlantAntiReverse 执行一次 PCC 防逆流闭环：读 Plant 的 PCC 反馈绑定，
// 实测越限则按各 EnergyUnit 可削减能力分配削减量，写设备。数据过期或不可
// 用时不授权动作（G06）。这是把 PCC 反馈从"空参数"接到真实闭环的关键。
func (h *Handler) RunPlantAntiReverse(plantID string) ([]decision.EnergyAllocation, error) {
	binding, reading, ok := ReadPlantPCC(h.nc, plantID)
	if !ok {
		return nil, fmt.Errorf("plant %s has no PCC feedback binding", plantID)
	}
	reduction, usable := decision.AntiReverseFromFeedback(binding, reading)
	if !usable {
		return nil, fmt.Errorf("PCC feedback unusable: %s", reading.Reason)
	}
	if reduction <= 0 {
		return nil, nil // 无反送，无需调节
	}

	// 读该 Plant 下的 EnergyUnit 及能力，分配削减量。
	units, err := siotclient.GetNodes(h.nc, plantID, "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return nil, err
	}
	members := make([]decision.EnergyMember, 0, len(units))
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		members = append(members, decision.EnergyMember{EnergyUnitID: unit.ID})
		ids = append(ids, unit.ID)
	}
	caps, err := LoadCapabilityValues(h.nc, ids)
	if err != nil {
		return nil, err
	}
	allocations, err := decision.ApplyAntiReverse(reduction, members, caps)
	if err != nil {
		return nil, err
	}
	// 闭环要落到设备：把各成员削减量写入其柜内设备。
	for _, allocation := range allocations {
		if allocation.Excluded {
			continue
		}
		if err := h.writeEnergyUnitPower(allocation.EnergyUnitID, allocation.SetpointKW); err != nil {
			return allocations, err
		}
	}
	return allocations, nil
}

// RunPlantPeakShaving 执行一次 PCC 需量削减闭环。
func (h *Handler) RunPlantPeakShaving(plantID string) ([]decision.EnergyAllocation, error) {
	binding, reading, ok := ReadPlantPCC(h.nc, plantID)
	if !ok {
		return nil, fmt.Errorf("plant %s has no PCC feedback binding", plantID)
	}
	shave, usable := decision.PeakShavingFromFeedback(binding, reading)
	if !usable {
		return nil, fmt.Errorf("PCC feedback unusable: %s", reading.Reason)
	}
	if shave <= 0 {
		return nil, nil
	}
	units, err := siotclient.GetNodes(h.nc, plantID, "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return nil, err
	}
	members := make([]decision.EnergyMember, 0, len(units))
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		members = append(members, decision.EnergyMember{EnergyUnitID: unit.ID})
		ids = append(ids, unit.ID)
	}
	caps, err := LoadCapabilityValues(h.nc, ids)
	if err != nil {
		return nil, err
	}
	allocations, err := decision.ApplyPeakShaving(shave, members, caps)
	if err != nil {
		return nil, err
	}
	for _, allocation := range allocations {
		if allocation.Excluded {
			continue
		}
		if err := h.writeEnergyUnitPower(allocation.EnergyUnitID, allocation.SetpointKW); err != nil {
			return allocations, err
		}
	}
	return allocations, nil
}

// savePCCFeedbackForPlant 供 API/seed 写入 Plant 的 PCC 反馈绑定。
func savePCCFeedbackForPlant(nc *nats.Conn, plantID string, binding decision.PCCFeedback) error {
	return savePCCFeedback(nc, plantID, binding)
}
