package decision

// 动态能力计算（Q6/Q7）：可分配功率 = min(额定, BMS 允许, SOC/温度约束)，
// 充放分开，SOC 越界剔除方向，温度越限降额、超限剔除。这是控制链路
// Capability Engine 的核心语义。

// CapabilityLimit 是计算一个方向动态能力的输入。
type CapabilityLimit struct {
	Rated      float64 // 额定功率 kW
	Available  float64 // BMS 允许功率 kW（该方向的允许值）
	SOC        float64 // 当前 SOC %
	SOCMin     float64 // SOC 下限：低于此禁放
	SOCMax     float64 // SOC 上限：高于此禁充
	Temp       float64 // 当前温度 ℃
	TempDerate float64 // 温度降额起点：超过此线性降额
	TempLimit  float64 // 温度上限：超过此剔除
}

// DynamicDischarge 计算放电方向的可分配能力。返回能力值（kW）与是否可用。
func DynamicDischarge(limit CapabilityLimit) (float64, bool) {
	if limit.SOC <= limit.SOCMin {
		return 0, false // SOC 低，禁放
	}
	if limit.Temp >= limit.TempLimit {
		return 0, false // 温度超限，剔除
	}
	capacity := min2(limit.Rated, limit.Available)
	if capacity <= 0 {
		return 0, false
	}
	if limit.TempDerate > 0 && limit.Temp > limit.TempDerate && limit.TempLimit > limit.TempDerate {
		// 线性降额：从 TempDerate 到 TempLimit，能力从 100% 降到 0。
		ratio := 1 - (limit.Temp-limit.TempDerate)/(limit.TempLimit-limit.TempDerate)
		if ratio < 0 {
			ratio = 0
		}
		capacity *= ratio
	}
	return capacity, true
}

// DynamicCharge 计算充电方向的可分配能力。
func DynamicCharge(limit CapabilityLimit) (float64, bool) {
	if limit.SOC >= limit.SOCMax {
		return 0, false // SOC 高，禁充
	}
	if limit.Temp >= limit.TempLimit {
		return 0, false
	}
	capacity := min2(limit.Rated, limit.Available)
	if capacity <= 0 {
		return 0, false
	}
	if limit.TempDerate > 0 && limit.Temp > limit.TempDerate && limit.TempLimit > limit.TempDerate {
		ratio := 1 - (limit.Temp-limit.TempDerate)/(limit.TempLimit-limit.TempDerate)
		if ratio < 0 {
			ratio = 0
		}
		capacity *= ratio
	}
	return capacity, true
}

func min2(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
