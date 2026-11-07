package safety

import (
	"testing"
	"time"

	"easyems/internal/ems/signal"
)

func TestConfigOutputAndConditions(t *testing.T) {
	min, max, discharge, charge := -500.0, 500.0, 200.0, 150.0
	config := Config{
		Min: &min, Max: &max, MaxDischarge: &discharge, MaxCharge: &charge,
		Conditions: []ConditionConfig{
			{PointID: "soc", Operator: "ge", Value: 10},
			{PointID: "running", Operator: "eq", Kind: "bool", Value: 1},
		},
	}
	spec, err := config.Output()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	samples := map[string]signal.Sample{
		"soc":     {Value: signal.Num(50), Quality: signal.Good, SourceTime: now, ReceivedTime: now},
		"running": {Value: signal.Flag(true), Quality: signal.Good, SourceTime: now, ReceivedTime: now},
	}
	limiter := Limiter{MaxAge: 15 * time.Second, Outputs: map[string]Output{"pcs": spec}, Samples: samples}
	if result := limiter.Limit(now, "pcs", signal.Num(400)); result.Blocked || result.Value.Num != 200 {
		t.Fatalf("expected clamp to discharge cap, got %+v", result)
	}
	if result := limiter.Limit(now, "pcs", signal.Num(-400)); result.Blocked || result.Value.Num != -150 {
		t.Fatalf("expected clamp to charge cap, got %+v", result)
	}
	samples["soc"] = signal.Sample{Value: signal.Num(5), Quality: signal.Good, SourceTime: now, ReceivedTime: now}
	if result := limiter.Limit(now, "pcs", signal.Num(100)); !result.Blocked {
		t.Fatalf("expected SOC condition to block, got %+v", result)
	}
}

func TestConfigValidation(t *testing.T) {
	min, max := 100.0, 100.0
	if _, err := (Config{Min: &min, Max: &max}).Output(); err == nil {
		t.Fatal("expected empty range to fail")
	}
	if _, err := (Config{Conditions: []ConditionConfig{{PointID: "soc", Operator: "gt", Kind: "bool", Value: 1}}}).Output(); err == nil {
		t.Fatal("expected bool gt to fail")
	}
	if _, err := (Config{Conditions: []ConditionConfig{{PointID: "mode", Operator: "eq", Kind: "enum"}}}).Output(); err == nil {
		t.Fatal("expected enum without label to fail")
	}
}

func TestRampStep(t *testing.T) {
	if got := RampStep(0, 100, 10, time.Second); got != 10 {
		t.Fatalf("ramp step = %v, want 10", got)
	}
	if got := RampStep(95, 100, 10, time.Second); got != 100 {
		t.Fatalf("ramp should reach target, got %v", got)
	}
	if got := RampStep(100, 0, 25, 2*time.Second); got != 50 {
		t.Fatalf("negative ramp = %v, want 50", got)
	}
	if got := RampStep(5, 10, 0, time.Second); got != 5 {
		t.Fatalf("zero rate must not move, got %v", got)
	}
}
