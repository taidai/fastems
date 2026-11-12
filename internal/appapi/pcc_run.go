package appapi

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"

	"fastiot/internal/decision"
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
	// 只算分配，不写设备——由统一仲裁（RunUnitArbitratedDispatch）决定最终
	// 目标后统一写，避免与单柜规则等多目标源冲突。
	return allocations, nil
}

// RunPlantPeakShaving 执行一次 PCC 需量削减闭环。
func (h *Handler) RunPlantPeakShaving(plantID string) ([]decision.EnergyAllocation, error) {
	binding, _, ok := ReadPlantPCC(h.nc, plantID)
	if !ok {
		return nil, fmt.Errorf("plant %s has no PCC feedback binding", plantID)
	}
	// 预测性削峰只使用 15 分钟滑动需量；缺 bin 时不做，避免用瞬时值冒充窗口需量。
	window, err := h.plantDemandWindow(binding, time.Now())
	if err != nil {
		return nil, err
	}
	if !window.Available {
		return nil, fmt.Errorf("demand window unavailable: %d missing bins", len(window.MissingBins))
	}
	if binding.UpperLimitKW <= 0 || window.DemandKW <= binding.UpperLimitKW {
		return nil, nil
	}
	shave := window.DemandKW - binding.UpperLimitKW
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
	// 只算分配，不写设备——由统一仲裁决定最终目标后统一写。
	return decision.ApplyPeakShaving(shave, members, caps)
}

// savePCCFeedbackForPlant 供 API/seed 写入 Plant 的 PCC 反馈绑定。
func savePCCFeedbackForPlant(nc *nats.Conn, plantID string, binding decision.PCCFeedback) error {
	return savePCCFeedback(nc, plantID, binding)
}
