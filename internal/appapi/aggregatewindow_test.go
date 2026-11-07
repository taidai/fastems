package appapi

import (
	"math"
	"testing"
	"time"

	"github.com/simpleiot/simpleiot/data"
)

func floatPtr(v float64) *float64 { return &v }

func TestParseAggregateWindow(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"5m", 5 * time.Minute, false},
		{"15m", 15 * time.Minute, false},
		{"1h", time.Hour, false},
		{"300s", 300 * time.Second, false},
		{"0s", 0, true},
		{"-5m", 0, true},
		{"25h", 0, true},
		{"bogus", 0, true},
	}
	for _, c := range cases {
		got, err := parseAggregateWindow(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("parseAggregateWindow(%q) expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("parseAggregateWindow(%q) = %v, %v; want %v, nil", c.in, got, err, c.want)
		}
	}
}

func sample(value float64, at time.Time) data.Point {
	p := data.NewPointFloat(data.PointTypeValue, "", value)
	p.Time = at
	return p
}

func TestWindowMemberMean(t *testing.T) {
	base := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	samples := []data.Point{
		sample(2, base.Add(-20*time.Second)),
		sample(4, base.Add(-10*time.Second)),
		sample(6, base.Add(-5*time.Second)),
	}
	mean, latest, ok := windowMemberMean(samples)
	if !ok || mean != 4 || !latest.Equal(base.Add(-5*time.Second)) {
		t.Fatalf("windowMemberMean = %v, %v, %v; want 4, latest sample, true", mean, latest, ok)
	}

	// A non-finite sample is skipped; finite samples still average.
	withNaN := append([]data.Point{sample(math.NaN(), base)}, samples...)
	mean, _, ok = windowMemberMean(withNaN)
	if !ok || mean != 4 {
		t.Fatalf("windowMemberMean with NaN = %v, %v; want 4, true", mean, ok)
	}

	if _, _, ok := windowMemberMean(nil); ok {
		t.Fatal("windowMemberMean(nil) should report no sample")
	}
}

func TestApplyAggregateQuality(t *testing.T) {
	cases := []struct {
		policy  string
		good    int
		bad     int
		quality string
		partial bool
	}{
		{"ignore", 3, 0, "good", false},
		{"ignore", 3, 2, "good", false},
		{"ignore", 0, 5, "offline", false},
		{"count", 3, 0, "good", false},
		{"count", 3, 2, "partial", true},
		{"count", 0, 5, "offline", false},
		{"degrade", 3, 0, "good", false},
		{"degrade", 3, 2, "degraded", true},
		{"degrade", 0, 5, "offline", false},
		// Empty policy falls back to count (default).
		{"", 3, 2, "partial", true},
		{"", 3, 0, "good", false},
	}
	for _, c := range cases {
		quality, partial := applyAggregateQuality(c.policy, c.good, c.bad)
		if quality != c.quality || partial != c.partial {
			t.Fatalf("applyAggregateQuality(%q, %d, %d) = %q, %v; want %q, %v",
				c.policy, c.good, c.bad, quality, partial, c.quality, c.partial)
		}
	}
}

func TestAggregateValues(t *testing.T) {
	values := []float64{2, 4, 6}
	if got := aggregateValues("sum", values); got != 12 {
		t.Fatalf("sum = %v; want 12", got)
	}
	if got := aggregateValues("avg", values); got != 4 {
		t.Fatalf("avg = %v; want 4", got)
	}
	if got := aggregateValues("min", values); got != 2 {
		t.Fatalf("min = %v; want 2", got)
	}
	if got := aggregateValues("max", values); got != 6 {
		t.Fatalf("max = %v; want 6", got)
	}
	if got := aggregateValues("count", values); got != 3 {
		t.Fatalf("count = %v; want 3", got)
	}
}

func TestEvaluateAggregateAlarm(t *testing.T) {
	cfg := &AggregateAlarm{Enabled: true, Min: floatPtr(5), Max: floatPtr(10)}
	cases := []struct {
		value  float64
		breach bool
	}{
		{3, true},
		{7, false},
		{12, true},
		{5, false},  // inclusive at the min boundary
		{10, false}, // inclusive at the max boundary
	}
	for _, c := range cases {
		breach, _ := evaluateAggregateAlarm(c.value, cfg)
		if breach != c.breach {
			t.Fatalf("evaluateAggregateAlarm(%v) = %v; want %v", c.value, breach, c.breach)
		}
	}
	if breach, _ := evaluateAggregateAlarm(0, &AggregateAlarm{Enabled: false, Max: floatPtr(1)}); breach {
		t.Fatal("disabled alarm should never breach")
	}
	if breach, _ := evaluateAggregateAlarm(0, nil); breach {
		t.Fatal("nil alarm should never breach")
	}
}

func TestValidateAggregateAlarm(t *testing.T) {
	valid := []*AggregateAlarm{
		nil,
		{Enabled: false},
		{Enabled: true, Max: floatPtr(10)},
		{Enabled: true, Min: floatPtr(5), Max: floatPtr(10)},
		{Enabled: true, Max: floatPtr(10), Severity: "critical"},
	}
	for _, cfg := range valid {
		if err := validateAggregateAlarm(cfg); err != nil {
			t.Fatalf("validateAggregateAlarm(%+v) unexpected error: %v", cfg, err)
		}
	}
	invalid := []*AggregateAlarm{
		{Enabled: true},
		{Enabled: true, Max: floatPtr(10), Severity: "fatal"},
		{Enabled: true, Min: floatPtr(10), Max: floatPtr(5)},
		{Enabled: true, Min: floatPtr(math.NaN()), Max: floatPtr(10)},
	}
	for _, cfg := range invalid {
		if err := validateAggregateAlarm(cfg); err == nil {
			t.Fatalf("validateAggregateAlarm(%+v) expected error", cfg)
		}
	}
}
