package decision

import "easyems/internal/ems/alloc"

// EnergyMember 是能力级分配的一个成员（一个 EnergyUnit）。能力不从成员
// 声明，而是运行时从 EnergyUnit 的 CapabilityValues 读取——这就是 Q11=A
// 的核心：分配器认 EnergyUnit 的能力，不认具体 PCS 型号。
type EnergyMember struct {
	EnergyUnitID string  `json:"energy_unit_id"`
	MinPower     float64 `json:"min_power,omitempty"`
	Priority     int     `json:"priority,omitempty"`
	Weight       float64 `json:"weight,omitempty"`
}

// EnergyAllocation 是给一个 EnergyUnit 的分配结果。
type EnergyAllocation struct {
	EnergyUnitID string  `json:"energy_unit_id"`
	SetpointKW   float64 `json:"setpoint_kw"`
	Excluded     bool    `json:"excluded"`
	Reason       string  `json:"reason,omitempty"`
}

// AllocateEnergy 把请求功率按能力分配到 EnergyUnit。能力值来自 caps：
// FieldAvailablePower 是当前可用能力，FieldRatedPower 是额定上限。复用
// alloc.Allocate 的水填充算法，只做能力值 → alloc.Member 的适配。
func AllocateEnergy(requestKW float64, members []EnergyMember, caps CapabilityValues) ([]EnergyAllocation, error) {
	allocMembers := make([]alloc.Member, 0, len(members))
	for _, m := range members {
		fields := caps[m.EnergyUnitID]
		rated := fields[FieldRatedPower]
		available := fields[FieldAvailablePower]
		// 方向约束：光伏只放电、充电桩只充电、储能双向。
		availableCharge, availableDischarge := available, available
		switch int(fields[FieldEnergyType]) {
		case EnergyPVDischarge:
			availableCharge = 0
		case EnergyEVCharge:
			availableDischarge = 0
		}
		allocMembers = append(allocMembers, alloc.Member{
			ID:                 m.EnergyUnitID,
			Online:             true,
			MinPower:           m.MinPower,
			MaxCharge:          rated,
			MaxDischarge:       rated,
			AvailableCharge:    availableCharge,
			AvailableDischarge: availableDischarge,
		})
	}
	result, err := alloc.Allocate(requestKW, allocMembers)
	if err != nil {
		return nil, err
	}
	allocations := make([]EnergyAllocation, 0, len(result.Shares))
	for _, share := range result.Shares {
		allocations = append(allocations, EnergyAllocation{
			EnergyUnitID: share.MemberID,
			SetpointKW:   share.Setpoint,
			Excluded:     share.Excluded,
			Reason:       share.Reason,
		})
	}
	return allocations, nil
}
