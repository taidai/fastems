package decision

// 预测接入：基于历史样本对负载/光伏出力做短期预测，供调度提前决策
// （如预测到高峰提前放电）。用移动平均 + 线性趋势外推的最小实现，
// 预测值带来源与置信说明，不冒充精确预测。

// ForecastSample 是一个历史样本点（按时间顺序）。
type ForecastSample struct {
	KW float64 // 功率 kW
}

// Forecast 对一组历史样本做 periods 个周期的短期预测。返回每周期的预测
// 功率与趋势说明。样本不足时返回最近值（保守）。
func Forecast(samples []ForecastSample, periods int) []float64 {
	n := len(samples)
	if n == 0 || periods <= 0 {
		return nil
	}
	// 移动平均窗口（取最近 min(4,n) 个样本）。
	window := 4
	if n < window {
		window = n
	}
	recent := samples[n-window:]
	mean := 0.0
	for _, s := range recent {
		mean += s.KW
	}
	mean /= float64(window)

	// 线性趋势：最近窗口首尾的斜率。
	slope := 0.0
	if window >= 2 {
		slope = (recent[window-1].KW - recent[0].KW) / float64(window-1)
	}

	out := make([]float64, periods)
	for i := 0; i < periods; i++ {
		out[i] = mean + slope*float64(i+1)
		if out[i] < 0 {
			out[i] = 0 // 功率不为负
		}
	}
	return out
}

// ForecastSummary 返回预测的可解释说明（来源与趋势）。
func ForecastSummary(samples []ForecastSample) string {
	if len(samples) < 2 {
		return "样本不足，按最近值估计"
	}
	return "移动平均 + 线性趋势外推（短期估计，非精确预测）"
}
