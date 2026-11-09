package decision

// 削峰填谷与计划调度（分时电价策略）：按分时电价表在低谷充电、高峰放电，
// 平时保持。这是 EMS 经济性调度的核心——利用峰谷价差降低成本，且每个
// 时段的充放决策可解释。

// TariffPeriod 是一个电价时段。StartMin/EndMin 为一天内分钟数（0–1439），
// 支持跨午夜（StartMin > EndMin 表示跨天）。
type TariffPeriod struct {
	StartMin int     // 起始分钟（含）
	EndMin   int     // 结束分钟（不含）
	Kind     string  // peak（峰）/ flat（平）/ valley（谷）
	Price    float64 // 电价 元/kWh
}

// Contains 判断 minuteOfDay 是否落在该时段内（支持跨午夜）。
func (p TariffPeriod) Contains(minuteOfDay int) bool {
	if p.StartMin <= p.EndMin {
		return minuteOfDay >= p.StartMin && minuteOfDay < p.EndMin
	}
	// 跨午夜：>= start 或 < end
	return minuteOfDay >= p.StartMin || minuteOfDay < p.EndMin
}

// PeakValleyAction 返回当前时刻应执行的功率请求：谷时充电（负值）、峰时
// 放电（正值）、平时 0。powerKW 为幅值。无匹配时段返回 0（不动作）。
func PeakValleyAction(periods []TariffPeriod, minuteOfDay int, powerKW float64) float64 {
	for _, period := range periods {
		if !period.Contains(minuteOfDay) {
			continue
		}
		switch period.Kind {
		case "valley":
			return -powerKW // 谷时充电
		case "peak":
			return powerKW // 峰时放电
		default:
			return 0 // 平时保持
		}
	}
	return 0
}

// ScheduleTarget 返回当前时段的计划功率目标（计划调度）。无匹配返回 0。
func ScheduleTarget(periods []TariffPeriod, minuteOfDay int, targets map[string]float64) float64 {
	for _, period := range periods {
		if period.Contains(minuteOfDay) {
			return targets[period.Kind]
		}
	}
	return 0
}
