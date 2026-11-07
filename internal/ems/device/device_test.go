package device

import (
	"strings"
	"testing"
	"time"

	"easyems/internal/ems/signal"
)

var now = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func profile() Profile {
	return Profile{
		ID: "enjo-pcs", Vendor: "恩玖", Model: "PCS",
		Points: []Point{
			{ID: "power", Kind: signal.Number, Unit: "kW", HasRange: true, Min: -500, Max: 500},
			{ID: "run", Kind: signal.Bool},
			{ID: "state", Kind: signal.Enum, ReadOnly: true},
		},
		Commands: []Command{{
			ID: "start", Name: "启动",
			Writes:   []Write{{Point: "run", Value: signal.Flag(true)}},
			Readback: []Readback{{Point: "run", Op: "eq", Value: signal.Flag(true)}},
			Timeout:  5 * time.Second,
		}},
	}
}

func TestProfileValidates(t *testing.T) {
	if err := profile().Validate(); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
}

func TestProfileRejectsUnsafeDefinitions(t *testing.T) {
	cases := map[string]func(*Profile){
		"duplicate point":         func(p *Profile) { p.Points = append(p.Points, p.Points[0]) },
		"empty range":             func(p *Profile) { p.Points[0].Min = 10; p.Points[0].Max = 10 },
		"write read-only":         func(p *Profile) { p.Commands[0].Writes[0].Point = "state" },
		"unknown write point":     func(p *Profile) { p.Commands[0].Writes[0].Point = "missing" },
		"duplicate command":       func(p *Profile) { p.Commands = append(p.Commands, p.Commands[0]) },
		"unknown readback point":  func(p *Profile) { p.Commands[0].Readback[0].Point = "missing" },
		"operator for wrong kind": func(p *Profile) { p.Commands[0].Readback[0].Op = "gt" },
		"write kind mismatch":     func(p *Profile) { p.Commands[0].Writes[0].Value = signal.Num(1) },
	}
	for name, mutate := range cases {
		p := profile()
		mutate(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestCheckConfirmsFromReadback(t *testing.T) {
	good := signal.Sample{Input: "run", Value: signal.Flag(true), Quality: signal.Good, SourceTime: now, ReceivedTime: now}
	ok, reason := Check(profile(), "start", map[string]signal.Sample{"run": good}, now, 5*time.Second)
	if !ok {
		t.Fatalf("expected confirmation, got %q", reason)
	}

	wrong := good
	wrong.Value = signal.Flag(false)
	if ok, reason := Check(profile(), "start", map[string]signal.Sample{"run": wrong}, now, 5*time.Second); ok {
		t.Fatal("mismatched readback must not confirm")
	} else if !strings.Contains(reason, "does not satisfy") {
		t.Fatalf("unexpected reason %q", reason)
	}

	stale := good
	stale.Quality = signal.Stale
	if ok, reason := Check(profile(), "start", map[string]signal.Sample{"run": stale}, now, 5*time.Second); ok || !strings.Contains(reason, "not usable") {
		t.Fatalf("stale readback must not confirm: %q", reason)
	}
}
