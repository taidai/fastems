package decision

import (
	"sort"
	"time"
)

// 15 分钟滑动需量：窗口 (now-15min, now]，滑动步长 1 分钟，切成 15 个一分钟 bin。
// 每个 bin 至少一个新鲜样本才有效；任一 bin 缺失时需量不可用，
// 不做"猜一个值"的预测性削峰——这是定案里明确的口径。

const demandWindow = 15 * time.Minute

type DemandSample struct {
	Time time.Time
	KW   float64
}

type DemandWindow struct {
	Available   bool      `json:"available"`
	DemandKW    float64   `json:"demand_kw"`
	ValidBins   int       `json:"valid_bins"`
	MissingBins []string  `json:"missing_bins,omitempty"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

// SlidingDemand 计算当前时刻的 15 分钟滑动需量。
// 先求每个一分钟 bin 的平均功率，再对 15 个 bin 求平均。
func SlidingDemand(samples []DemandSample, now time.Time) DemandWindow {
	start := now.Add(-demandWindow)
	window := DemandWindow{WindowStart: start, WindowEnd: now}
	bins := make([][]float64, 15)
	for _, sample := range samples {
		if !sample.Time.After(start) || sample.Time.After(now) {
			continue
		}
		index := int(sample.Time.Sub(start) / time.Minute)
		if index >= len(bins) {
			index = len(bins) - 1
		}
		if index < 0 {
			continue
		}
		bins[index] = append(bins[index], sample.KW)
	}
	sum := 0.0
	valid := 0
	for index, values := range bins {
		if len(values) == 0 {
			window.MissingBins = append(window.MissingBins, start.Add(time.Duration(index)*time.Minute).Format("15:04"))
			continue
		}
		binSum := 0.0
		for _, value := range values {
			binSum += value
		}
		sum += binSum / float64(len(values))
		valid++
	}
	sort.Strings(window.MissingBins)
	window.ValidBins = valid
	if valid == len(bins) {
		window.Available = true
		window.DemandKW = sum / float64(valid)
	}
	return window
}
