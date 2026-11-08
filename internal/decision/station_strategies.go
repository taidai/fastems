package decision

// 站级调度策略（Q10=B 的 V2 范围）：防逆流与需量控制。它们复用能力级
// 分配（AllocateEnergy/AllocateSite），把站级约束转化为对各能源节点的
// 功率削减/放电请求。

// AntiReverse 计算防止向电网反送所需的放电削减量。pccKW 是关口表功率，
// 负值表示向电网反送。返回需要削减的放电功率（正值 = 削减量），使 PCC
// 回到 0 或正。
func AntiReverse(pccKW float64) float64 {
	if pccKW >= 0 {
		return 0
	}
	return -pccKW
}

// ApplyAntiReverse 把防逆流削减量按各能源节点的可削减能力分配。削减对
// 能力级模型是一次负的放电请求（或充电），复用 AllocateEnergy。
func ApplyAntiReverse(reductionKW float64, members []EnergyMember, caps CapabilityValues) ([]EnergyAllocation, error) {
	return AllocateEnergy(-reductionKW, members, caps)
}

// PeakShaving 计算超出需量上限的削峰量。demandKW 是当前负荷/关口功率，
// limitKW 是需量上限。返回需要由储能放电（或削减充电）补充的功率，正值
// 表示储能应放电削峰。
func PeakShaving(demandKW, limitKW float64) float64 {
	if demandKW <= limitKW {
		return 0
	}
	return demandKW - limitKW
}

// ApplyPeakShaving 把削峰量按各能源节点的可放电能力分配，复用能力级分配。
func ApplyPeakShaving(shaveKW float64, members []EnergyMember, caps CapabilityValues) ([]EnergyAllocation, error) {
	return AllocateEnergy(shaveKW, members, caps)
}
