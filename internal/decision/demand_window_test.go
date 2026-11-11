package decision

import (
	"testing"
	"time"
)

func TestSlidingDemand(t *testing.T) {
	now := time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC)
	// 现场是秒级采集；测试用 30 秒间隔，保证每个一分钟 bin 都有样本。
	samples := make([]DemandSample, 0, 40)
	for i := 0; i < 40; i++ {
		samples = append(samples, DemandSample{Time: now.Add(-time.Duration(i) * 30 * time.Second), KW: 120})
	}
	window := SlidingDemand(samples, now)
	if !window.Available || window.ValidBins != 15 {
		t.Fatalf("window = %#v", window)
	}
	if window.DemandKW < 119.99 || window.DemandKW > 120.01 {
		t.Fatalf("demand = %.2f, want 120", window.DemandKW)
	}

	// 缺一个 bin：需量不可用，并明确列出缺失区间。
	missingFrom := now.Add(-7 * time.Minute)
	missingTo := now.Add(-6 * time.Minute)
	without := make([]DemandSample, 0, len(samples))
	for _, sample := range samples {
		if !sample.Time.Before(missingFrom) && sample.Time.Before(missingTo) {
			continue
		}
		without = append(without, sample)
	}
	window = SlidingDemand(without, now)
	if window.Available || window.ValidBins != 14 {
		t.Fatalf("missing window = %#v", window)
	}
	if len(window.MissingBins) != 1 || window.MissingBins[0] != "14:53" {
		t.Fatalf("missing bins = %#v", window.MissingBins)
	}

	// 窗口左端点 (t-15min) 不计入。
	boundary := []DemandSample{{Time: now.Add(-15 * time.Minute), KW: 999}}
	window = SlidingDemand(boundary, now)
	if window.ValidBins != 0 || window.Available {
		t.Fatalf("boundary window = %#v", window)
	}

	// 同一 bin 多个样本先求 bin 平均，再对 bin 求平均。
	mixed := make([]DemandSample, 0, 32)
	for i := 1; i <= 15; i++ {
		at := now.Add(-time.Duration(i) * time.Minute)
		value := 100.0
		if i == 1 {
			value = 160
		}
		mixed = append(mixed, DemandSample{Time: at, KW: value})
		mixed = append(mixed, DemandSample{Time: at.Add(30 * time.Second), KW: 100})
	}
	window = SlidingDemand(mixed, now)
	if !window.Available {
		t.Fatalf("mixed window = %#v", window)
	}
	// 14 个 bin 为 100，最后一个 bin 为 (160+100)/2 = 130。
	want := (100*14 + 130) / 15.0
	if window.DemandKW < want-0.01 || window.DemandKW > want+0.01 {
		t.Fatalf("mixed demand = %.2f, want %.2f", window.DemandKW, want)
	}
}
