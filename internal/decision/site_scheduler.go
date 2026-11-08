package decision

import "easyems/internal/ems/alloc"

// SitePlant 是站级调度的成员（一个电站 Plant）。能力是该电站下所有
// EnergyUnit 能力的汇总。
type SitePlant struct {
	PlantID         string  `json:"plant_id"`
	AvailablePowerKW float64 `json:"available_power_kw"`
	RatedPowerKW    float64 `json:"rated_power_kw"`
}

// SiteAllocation 是给一个电站的分配结果。
type SiteAllocation struct {
	PlantID    string  `json:"plant_id"`
	SetpointKW float64 `json:"setpoint_kw"`
	Excluded   bool    `json:"excluded"`
	Reason     string  `json:"reason,omitempty"`
}

// AllocateSite 把站级功率目标按各电站能力分配（Q10=B 的站级调度层）。
// 复用 alloc.Allocate 水填充算法，只做电站能力 → 分配成员的适配。
func AllocateSite(requestKW float64, plants []SitePlant) ([]SiteAllocation, error) {
	members := make([]alloc.Member, 0, len(plants))
	for _, plant := range plants {
		members = append(members, alloc.Member{
			ID:                  plant.PlantID,
			Online:              true,
			MaxCharge:           plant.RatedPowerKW,
			MaxDischarge:        plant.RatedPowerKW,
			AvailableCharge:     plant.AvailablePowerKW,
			AvailableDischarge:  plant.AvailablePowerKW,
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
