package decision

// ResourceGroup 资源集群（G02/T27）：把多个 EnergyUnit 编组，承载共享约束
// —— 一组柜共用一条馈线、一台变压器或一个功率配额上限。组上限是硬约束：
// 分配先把目标按组上限截断，再在组内按比例水填充，缺口如实上报，不重复
// 下发。无组上限时组只是操作视图，按正常分配。

// ResourceGroup 是一组能源节点及其共享功率上限。
type ResourceGroup struct {
	GroupID       string         `json:"group_id"`
	Members       []EnergyMember `json:"members"`
	SharedLimitKW float64        `json:"shared_limit_kw"` // 共享功率上限 kW；0 = 无约束
}

// GroupAllocation 是组分配的结果：各成员设定 + 组级缺口。
type GroupAllocation struct {
	Allocations []EnergyAllocation `json:"allocations"`
	DeliveredKW float64            `json:"delivered_kw"` // 实际分配合计（受组上限约束）
	ShortfallKW float64            `json:"shortfall_kw"` // 目标与合计差额
	LimitedBy   string             `json:"limited_by,omitempty"` // 限制来源（组上限）
}

// AllocateWithGroupLimit 在组共享上限下分配目标功率。放电为正、充电为负。
// 组上限对目标幅值封顶：|目标| > 组上限时，只能分到组上限，缺口上报。
func AllocateWithGroupLimit(requestKW float64, group ResourceGroup, caps CapabilityValues) (GroupAllocation, error) {
	target := requestKW
	limitedBy := ""
	if group.SharedLimitKW > 0 {
		if target > group.SharedLimitKW {
			target = group.SharedLimitKW
			limitedBy = "group_shared_limit"
		} else if target < -group.SharedLimitKW {
			target = -group.SharedLimitKW
			limitedBy = "group_shared_limit"
		}
	}
	allocations, err := AllocateEnergy(target, group.Members, caps)
	if err != nil {
		return GroupAllocation{}, err
	}
	delivered := 0.0
	for _, a := range allocations {
		if !a.Excluded {
			delivered += a.SetpointKW
		}
	}
	shortfall := requestKW - delivered
	if shortfall < 0 {
		shortfall = 0 // 缺口只报未满足的正差额（放电方向）
	}
	return GroupAllocation{
		Allocations: allocations,
		DeliveredKW: delivered,
		ShortfallKW: shortfall,
		LimitedBy:   limitedBy,
	}, nil
}
