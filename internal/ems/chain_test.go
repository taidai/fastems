package ems_test

import (
	"testing"
	"time"

	"fastiot/internal/ems/alloc"
	"fastiot/internal/ems/rule"
	"fastiot/internal/ems/safety"
	"fastiot/internal/ems/signal"
)

var now = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func good(input string, value signal.Value, quality signal.Quality) signal.Sample {
	return signal.Sample{Input: input, Value: value, Quality: quality, SourceTime: now, ReceivedTime: now}
}

func dispatchTable() rule.Table {
	return rule.Table{
		ID: "ess-dispatch", Version: 1, MaxAge: 5 * time.Second,
		Rows: []rule.Row{{
			ID: "discharge", Name: "储能放电",
			Conditions: []rule.Condition{{Input: "grid", Op: "gt", Value: signal.Num(0)}},
			Actions:    []rule.Action{{Output: "ess.dispatch", Value: signal.Num(800)}},
		}},
	}
}

func fourCabinets() []alloc.Member {
	return []alloc.Member{
		{ID: "CAB01", Online: true, MaxDischarge: 200, MaxCharge: 200, AvailableDischarge: 200, AvailableCharge: 200},
		{ID: "CAB02", Online: true, MaxDischarge: 200, MaxCharge: 200, AvailableDischarge: 200, AvailableCharge: 200},
		{ID: "CAB03", Online: true, MaxDischarge: 150, MaxCharge: 150, AvailableDischarge: 150, AvailableCharge: 150},
		{ID: "CAB04", Online: false, MaxDischarge: 200, MaxCharge: 200, AvailableDischarge: 200, AvailableCharge: 200},
	}
}

func TestFourCabinetDispatchChain(t *testing.T) {
	grid := good("grid", signal.Num(50), signal.Good)
	limiter := safety.Limiter{
		MaxAge:  5 * time.Second,
		Outputs: map[string]safety.Output{"ess.dispatch": {Kind: signal.Number, CapPositive: 800, Requires: []string{"grid"}}},
		Samples: map[string]signal.Sample{"grid": grid},
	}
	engine, err := rule.New(dispatchTable(), map[string]bool{"ess.dispatch": true}, limiter)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	out, err := engine.Evaluate(now, map[string]signal.Sample{"grid": grid})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !out.Fired || len(out.Actions) != 1 || out.Actions[0].Blocked {
		t.Fatalf("expected a dispatch request, got %+v", out)
	}
	if out.Actions[0].Applied.Num != 800 {
		t.Fatalf("requested value = %v, want 800", out.Actions[0].Applied.Num)
	}

	result, err := alloc.Allocate(out.Actions[0].Applied.Num, fourCabinets())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	want := map[string]float64{"CAB01": 200, "CAB02": 200, "CAB03": 150, "CAB04": 0}
	for _, share := range result.Shares {
		if share.Setpoint != want[share.MemberID] {
			t.Errorf("%s setpoint = %v, want %v", share.MemberID, share.Setpoint, want[share.MemberID])
		}
	}
	if result.Delivered != 550 || result.Shortfall != 250 {
		t.Errorf("delivered/shortfall = %v/%v, want 550/250", result.Delivered, result.Shortfall)
	}
	if len(result.Notes) == 0 {
		t.Error("shortfall must be explained")
	}
}

func TestSafetyBlocksDispatchWhenCapabilityInputIsStale(t *testing.T) {
	grid := good("grid", signal.Num(50), signal.Good)
	stale := good("bms", signal.Num(1), signal.Stale)
	limiter := safety.Limiter{
		MaxAge:  5 * time.Second,
		Outputs: map[string]safety.Output{"ess.dispatch": {Kind: signal.Number, CapPositive: 800, Requires: []string{"bms"}}},
		Samples: map[string]signal.Sample{"grid": grid, "bms": stale},
	}
	engine, err := rule.New(dispatchTable(), map[string]bool{"ess.dispatch": true}, limiter)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	out, err := engine.Evaluate(now, map[string]signal.Sample{"grid": grid})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !out.Fired || !out.Actions[0].Blocked {
		t.Fatalf("expected a blocked action, got %+v", out)
	}
	if out.Actions[0].Reason == "" {
		t.Fatal("blocked action must carry a reason")
	}
}
