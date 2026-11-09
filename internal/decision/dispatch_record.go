package decision

import (
	"time"

	"github.com/simpleiot/simpleiot/data"
)

// 统一追溯时间线（OBS-01）：每次能力级调度生成一个 dispatchID，把
// "能力快照 → 分配结果 → 设备设定"串成一条可复盘记录。值班员按
// dispatchID 能回答"当时谁受限、分了什么、下发了多少"（T18）。

// DispatchRecord 是一次调度的完整快照。
type DispatchRecord struct {
	DispatchID   string           `json:"dispatch_id"`
	EnergyUnitID string           `json:"energy_unit_id"`
	At           time.Time        `json:"at"`
	CabinetState CabinetState     `json:"cabinet_state"`
	RuleID       string           `json:"rule_id"`
	RequestKW    float64          `json:"request_kw"`   // 规则请求的功率
	Capability   CapabilityValues `json:"capability"`   // 当时能力快照
	Dispatches   []DeviceDispatch `json:"dispatches"`   // 每台设备设定/剔除原因
	DeliveredKW  float64          `json:"delivered_kw"` // 实际下发合计（未剔除）
	ShortfallKW  float64          `json:"shortfall_kw"` // 请求与下发差额（能力不足）
}

// NewDispatchID 生成一个调度关联 ID（复用 SimpleIoT 的 ID 生成）。
func NewDispatchID() string {
	return data.NewID()
}

// BuildRecord 汇总一次调度的记录。delivered/shortfall 由 dispatches 算出。
func BuildRecord(energyUnitID, ruleID string, requestKW float64, values CapabilityValues, state CabinetState, dispatches []DeviceDispatch) DispatchRecord {
	delivered := 0.0
	for _, d := range dispatches {
		if !d.Excluded {
			delivered += d.SetpointKW
		}
	}
	return DispatchRecord{
		DispatchID:   NewDispatchID(),
		EnergyUnitID: energyUnitID,
		At:           time.Now().UTC(),
		CabinetState: state,
		RuleID:       ruleID,
		RequestKW:    requestKW,
		Capability:   values,
		Dispatches:   dispatches,
		DeliveredKW:  delivered,
		ShortfallKW:  requestKW - delivered,
	}
}
