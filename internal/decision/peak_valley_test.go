package decision

import "testing"

// 削峰填谷：谷时充电、峰时放电、平时保持，跨午夜正确处理。
func TestPeakValleyAction(t *testing.T) {
	periods := []TariffPeriod{
		{StartMin: 0, EndMin: 480, Kind: "valley", Price: 0.3},    // 00:00–08:00 谷
		{StartMin: 480, EndMin: 1080, Kind: "flat", Price: 0.6},  // 08:00–18:00 平
		{StartMin: 1080, EndMin: 1320, Kind: "peak", Price: 1.0}, // 18:00–22:00 峰
		{StartMin: 1320, EndMin: 480, Kind: "valley", Price: 0.3}, // 22:00–08:00 跨午夜谷
	}
	if got := PeakValleyAction(periods, 120, 50); got != -50 {
		t.Fatalf("谷时应充电 -50，实测 %.1f", got)
	}
	if got := PeakValleyAction(periods, 600, 50); got != 0 {
		t.Fatalf("平时应保持 0，实测 %.1f", got)
	}
	if got := PeakValleyAction(periods, 1200, 50); got != 50 {
		t.Fatalf("峰时应放电 +50，实测 %.1f", got)
	}
	// 跨午夜：23:00（1380）落谷
	if got := PeakValleyAction(periods, 1380, 50); got != -50 {
		t.Fatalf("跨午夜谷时应充电，实测 %.1f", got)
	}
}

func TestScheduleTarget(t *testing.T) {
	periods := []TariffPeriod{{StartMin: 1080, EndMin: 1320, Kind: "peak"}}
	targets := map[string]float64{"peak": 80, "valley": -40}
	if got := ScheduleTarget(periods, 1200, targets); got != 80 {
		t.Fatalf("峰时计划目标应为 80，实测 %.1f", got)
	}
	if got := ScheduleTarget(periods, 600, targets); got != 0 {
		t.Fatalf("无匹配时段应返回 0，实测 %.1f", got)
	}
}
