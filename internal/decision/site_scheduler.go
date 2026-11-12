package decision

import "fastems/internal/ems/alloc"

// SitePlant 是站级调度的成员（一个电站 Plant）。能力是该电站下所有
// EnergyUnit 能力的汇总，充/放分开，离线或互锁的成员不计入。
type SitePlant struct {
	PlantID              string  `json:"plant_id"`
	AvailablePowerKW     float64 `json:"available_power_kw"` // 旧版双向合并值
	AvailableChargeKW    float64 `json:"available_charge_kw"`
	AvailableDischargeKW float64 `json:"available_discharge_kw"`
	RatedPowerKW         float64 `json:"rated_power_kw"`
	RatedChargeKW        float64 `json:"rated_charge_kw"`
	RatedDischargeKW     float64 `json:"rated_discharge_kw"`
	Online               bool    `json:"online"`
}

// SiteAllocation 是给一个电站的分配结果。
type SiteAllocation struct {
	PlantID    string  `json:"plant_id"`
	SetpointKW float64 `json:"setpoint_kw"`
	Excluded   bool    `json:"excluded"`
	Reason     string  `json:"reason,omitempty"`
}

// AllocateSite 把站级功率目标按各电站能力分配（Q10=B 的站级调度层）。
// 复用 alloc.Allocate 水填充算法，保留充/放方向与在线状态。
func AllocateSite(requestKW float64, plants []SitePlant) ([]SiteAllocation, error) {
	members := make([]alloc.Member, 0, len(plants))
	for _, plant := range plants {
		online := plant.Online
		if !plant.Online && plant.AvailablePowerKW > 0 {
			// 旧调用方只填 available_power 时按可用处理。
			online = true
		}
		charge, discharge := plant.AvailableChargeKW, plant.AvailableDischargeKW
		if charge == 0 && discharge == 0 {
			charge = plant.AvailablePowerKW
			discharge = plant.AvailablePowerKW
		}
		maxCharge, maxDischarge := plant.RatedChargeKW, plant.RatedDischargeKW
		if maxCharge == 0 && maxDischarge == 0 {
			maxCharge = plant.RatedPowerKW
			maxDischarge = plant.RatedPowerKW
		}
		members = append(members, alloc.Member{
			ID:                 plant.PlantID,
			Online:             online,
			MaxCharge:          maxCharge,
			MaxDischarge:       maxDischarge,
			AvailableCharge:    charge,
			AvailableDischarge: discharge,
		})
	}
	result, err := alloc.Allocate(requestKW, members)
	if err != nil {
		return nil, err
	}
	allocations := make([]SiteAllocation, 0, len(result.Shares))
	for _, share := range result.Shares {
		allocations = append(allocations, SiteAllocation{
			PlantID:    share.MemberID,
			SetpointKW: share.Setpoint,
			Excluded:   share.Excluded,
			Reason:     share.Reason,
		})
	}
	return allocations, nil
}
