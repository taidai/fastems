package rule

import (
	"strings"
	"testing"
	"time"

	"easyems/internal/ems/signal"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func good(input string, value signal.Value, at time.Time, age time.Duration) signal.Sample {
	return signal.Sample{
		Input:        input,
		Value:        value,
		Quality:      signal.Good,
		SourceTime:   at.Add(-age),
		ReceivedTime: at,
	}
}

func baseTable(rows ...Row) Table {
	return Table{ID: "demo", Version: 1, MaxAge: 5 * time.Second, Rows: rows}
}

func mustEngine(t *testing.T, table Table, owned map[string]bool, limiter Limiter) *Engine {
	t.Helper()
	engine, err := New(table, owned, limiter)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return engine
}

func TestFirstMatchingRowWins(t *testing.T) {
	table := baseTable(
		Row{ID: "high", Conditions: []Condition{{Input: "temp", Op: "gt", Value: signal.Num(30)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}},
		Row{ID: "warm", Conditions: []Condition{{Input: "temp", Op: "gt", Value: signal.Num(20)}}, Actions: []Action{{Output: "out", Value: signal.Num(2)}}},
		Row{ID: "default", Actions: []Action{{Output: "out", Value: signal.Num(0)}}},
	)
	engine := mustEngine(t, table, nil, nil)

	out, err := engine.Evaluate(t0, map[string]signal.Sample{"temp": good("temp", signal.Num(35), t0, time.Second)})
	if err != nil || out.MatchedRow != "high" || !out.Fired {
		t.Fatalf("expected high row to fire, got %+v err=%v", out, err)
	}
	engine.Reset()

	out, _ = engine.Evaluate(t0, map[string]signal.Sample{"temp": good("temp", signal.Num(25), t0, time.Second)})
	if out.MatchedRow != "warm" {
		t.Fatalf("expected warm row, got %s", out.MatchedRow)
	}
	engine.Reset()

	out, _ = engine.Evaluate(t0, map[string]signal.Sample{"temp": good("temp", signal.Num(10), t0, time.Second)})
	if out.MatchedRow != "default" {
		t.Fatalf("expected default row, got %s", out.MatchedRow)
	}
}

func TestConditionsAreAnded(t *testing.T) {
	table := baseTable(Row{
		ID: "combo",
		Conditions: []Condition{
			{Input: "temp", Op: "gt", Value: signal.Num(30)},
			{Input: "person", Op: "eq", Value: signal.Flag(true)},
			{Input: "mode", Op: "eq", Value: signal.Label("auto")},
		},
		Actions: []Action{{Output: "out", Value: signal.Num(1)}},
	})
	samples := map[string]signal.Sample{
		"temp":   good("temp", signal.Num(35), t0, time.Second),
		"person": good("person", signal.Flag(true), t0, time.Second),
		"mode":   good("mode", signal.Label("auto"), t0, time.Second),
	}
	out, err := Simulate(table, t0, samples, nil)
	if err != nil || out.MatchedRow != "combo" {
		t.Fatalf("expected combo match, got %+v err=%v", out, err)
	}
	samples["person"] = good("person", signal.Flag(false), t0, time.Second)
	out, _ = Simulate(table, t0, samples, nil)
	if out.MatchedRow != "" || out.State != StateNoMatch {
		t.Fatalf("expected no match when one condition fails, got %+v", out)
	}
}

func TestHoldDelaysFiring(t *testing.T) {
	table := baseTable(Row{ID: "hold", Hold: 10 * time.Second, Conditions: []Condition{{Input: "temp", Op: "gt", Value: signal.Num(30)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}})
	engine := mustEngine(t, table, nil, nil)
	samples := map[string]signal.Sample{"temp": good("temp", signal.Num(35), t0, time.Second)}

	out, _ := engine.Evaluate(t0, samples)
	if out.Fired || out.State != StateWaitingHold {
		t.Fatalf("expected waiting hold, got %+v", out)
	}
	out, _ = engine.Evaluate(t0.Add(9*time.Second), map[string]signal.Sample{"temp": good("temp", signal.Num(35), t0.Add(9*time.Second), time.Second)})
	if out.Fired {
		t.Fatalf("fired before hold elapsed: %+v", out)
	}
	out, _ = engine.Evaluate(t0.Add(10*time.Second), map[string]signal.Sample{"temp": good("temp", signal.Num(35), t0.Add(10*time.Second), time.Second)})
	if !out.Fired || out.State != StateFired {
		t.Fatalf("expected fire after hold, got %+v", out)
	}
}

func TestHoldResetsWhenRowChanges(t *testing.T) {
	table := baseTable(
		Row{ID: "a", Hold: 10 * time.Second, Conditions: []Condition{{Input: "x", Op: "gt", Value: signal.Num(10)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}},
		Row{ID: "b", Conditions: []Condition{{Input: "x", Op: "le", Value: signal.Num(10)}}, Actions: []Action{{Output: "out", Value: signal.Num(2)}}},
	)
	engine := mustEngine(t, table, nil, nil)
	engine.Evaluate(t0, map[string]signal.Sample{"x": good("x", signal.Num(20), t0, time.Second)})
	engine.Evaluate(t0.Add(5*time.Second), map[string]signal.Sample{"x": good("x", signal.Num(5), t0.Add(5*time.Second), time.Second)})
	out, _ := engine.Evaluate(t0.Add(9*time.Second), map[string]signal.Sample{"x": good("x", signal.Num(20), t0.Add(9*time.Second), time.Second)})
	if out.Fired || out.State != StateWaitingHold {
		t.Fatalf("hold should restart after row change, got %+v", out)
	}
}

func TestSameRowDoesNotFireTwice(t *testing.T) {
	table := baseTable(Row{ID: "on", Conditions: []Condition{{Input: "x", Op: "gt", Value: signal.Num(0)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}})
	engine := mustEngine(t, table, nil, nil)
	samples := map[string]signal.Sample{"x": good("x", signal.Num(1), t0, time.Second)}
	if out, _ := engine.Evaluate(t0, samples); !out.Fired {
		t.Fatalf("expected first fire")
	}
	out, _ := engine.Evaluate(t0.Add(time.Second), map[string]signal.Sample{"x": good("x", signal.Num(1), t0.Add(time.Second), time.Second)})
	if out.Fired || out.State != StateAlreadyFired {
		t.Fatalf("expected already fired, got %+v", out)
	}
}

func TestCooldownBlocksRefire(t *testing.T) {
	table := baseTable(Row{ID: "pulse", Cooldown: 60 * time.Second, Conditions: []Condition{{Input: "x", Op: "gt", Value: signal.Num(0)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}})
	engine := mustEngine(t, table, nil, nil)
	match := func(at time.Time, value float64) map[string]signal.Sample {
		return map[string]signal.Sample{"x": good("x", signal.Num(value), at, time.Second)}
	}
	if out, _ := engine.Evaluate(t0, match(t0, 1)); !out.Fired {
		t.Fatalf("expected first fire")
	}
	engine.Evaluate(t0.Add(time.Second), match(t0.Add(time.Second), 0))
	out, _ := engine.Evaluate(t0.Add(5*time.Second), match(t0.Add(5*time.Second), 1))
	if out.Fired || out.State != StateCooldown {
		t.Fatalf("expected cooldown, got %+v", out)
	}
	out, _ = engine.Evaluate(t0.Add(61*time.Second), match(t0.Add(61*time.Second), 1))
	if !out.Fired {
		t.Fatalf("expected fire after cooldown, got %+v", out)
	}
}

func TestUnusableInputBlocksRowAndFallsThrough(t *testing.T) {
	table := baseTable(
		Row{ID: "guard", Conditions: []Condition{{Input: "soc", Op: "gt", Value: signal.Num(20)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}},
		Row{ID: "default", Actions: []Action{{Output: "out", Value: signal.Num(0)}}},
	)
	engine := mustEngine(t, table, nil, nil)
	stale := signal.Sample{Input: "soc", Value: signal.Num(80), Quality: signal.Stale, SourceTime: t0, ReceivedTime: t0}
	out, _ := engine.Evaluate(t0, map[string]signal.Sample{"soc": stale})
	if out.MatchedRow != "default" {
		t.Fatalf("stale input must not match guard, got %s", out.MatchedRow)
	}
	if len(out.Blocked) != 1 || !strings.Contains(out.Blocked[0].Reason, "quality") {
		t.Fatalf("expected quality block reason, got %+v", out.Blocked)
	}
}

func TestMissingAndOldInputsAreUnusable(t *testing.T) {
	table := baseTable(Row{ID: "r", Conditions: []Condition{{Input: "x", Op: "gt", Value: signal.Num(0)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}})
	engine := mustEngine(t, table, nil, nil)
	out, _ := engine.Evaluate(t0, nil)
	if out.MatchedRow != "" || len(out.Blocked) != 1 || !strings.Contains(out.Blocked[0].Reason, "missing") {
		t.Fatalf("expected missing input block, got %+v", out)
	}
	old := signal.Sample{Input: "x", Value: signal.Num(1), Quality: signal.Good, SourceTime: t0.Add(-10 * time.Second), ReceivedTime: t0}
	out, _ = engine.Evaluate(t0, map[string]signal.Sample{"x": old})
	if out.MatchedRow != "" || !strings.Contains(out.Blocked[0].Reason, "stale") {
		t.Fatalf("expected stale block, got %+v", out)
	}
}

func TestSnapshotIsSortedAndCaptured(t *testing.T) {
	table := baseTable(Row{ID: "r", Conditions: []Condition{{Input: "b", Op: "gt", Value: signal.Num(0)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}})
	engine := mustEngine(t, table, nil, nil)
	out, _ := engine.Evaluate(t0, map[string]signal.Sample{
		"b": good("b", signal.Num(1), t0, time.Second),
		"a": good("a", signal.Num(2), t0, time.Second),
	})
	if len(out.Snapshot) != 2 || out.Snapshot[0].Input != "a" || out.Snapshot[1].Input != "b" {
		t.Fatalf("snapshot not sorted: %+v", out.Snapshot)
	}
}

type capLimiter struct{ cap float64 }

func (l capLimiter) Limit(_ time.Time, _ string, requested signal.Value) LimitResult {
	if requested.Kind != signal.Number {
		return LimitResult{Value: requested}
	}
	if requested.Num > l.cap {
		return LimitResult{Value: signal.Num(l.cap), Reason: "device capability limit"}
	}
	return LimitResult{Value: requested}
}

type blockingLimiter struct{}

func (blockingLimiter) Limit(_ time.Time, _ string, requested signal.Value) LimitResult {
	return LimitResult{Value: requested, Blocked: true, Reason: "device offline"}
}

func TestLimiterIsAppliedAndExplained(t *testing.T) {
	table := baseTable(Row{ID: "r", Conditions: []Condition{{Input: "x", Op: "gt", Value: signal.Num(0)}}, Actions: []Action{{Output: "pcs", Value: signal.Num(500)}}})
	engine := mustEngine(t, table, map[string]bool{"pcs": true}, capLimiter{cap: 300})
	out, err := engine.Evaluate(t0, map[string]signal.Sample{"x": good("x", signal.Num(1), t0, time.Second)})
	if err != nil || !out.Fired || len(out.Actions) != 1 {
		t.Fatalf("expected fire, got %+v err=%v", out, err)
	}
	action := out.Actions[0]
	if action.Blocked || action.Applied.Num != 300 || action.Requested.Num != 500 || action.Reason == "" {
		t.Fatalf("limiter not applied: %+v", action)
	}
}

func TestBlockedActionIsMarked(t *testing.T) {
	table := baseTable(Row{ID: "r", Actions: []Action{{Output: "pcs", Value: signal.Num(500)}}})
	engine := mustEngine(t, table, map[string]bool{"pcs": true}, blockingLimiter{})
	out, err := engine.Evaluate(t0, map[string]signal.Sample{})
	if err != nil || !out.Fired {
		t.Fatalf("expected fire, got %+v err=%v", out, err)
	}
	if !out.Actions[0].Blocked || out.Actions[0].Reason != "device offline" {
		t.Fatalf("expected blocked action, got %+v", out.Actions[0])
	}
}

func TestOwnershipIsEnforced(t *testing.T) {
	table := baseTable(Row{ID: "r", Actions: []Action{{Output: "other", Value: signal.Num(1)}}})
	if _, err := New(table, map[string]bool{"mine": true}, nil); err == nil {
		t.Fatal("expected ownership error")
	}
	if _, err := New(table, map[string]bool{"other": true}, nil); err != nil {
		t.Fatalf("owned output rejected: %v", err)
	}
}

func TestPrimeSuppressesReplayAfterRestart(t *testing.T) {
	table := baseTable(
		Row{ID: "hot", Conditions: []Condition{{Input: "temp", Op: "gt", Value: signal.Num(30)}}, Actions: []Action{{Output: "out", Value: signal.Num(1)}}},
		Row{ID: "cold", Conditions: []Condition{{Input: "temp", Op: "le", Value: signal.Num(30)}}, Actions: []Action{{Output: "out", Value: signal.Num(0)}}},
	)
	engine := mustEngine(t, table, nil, nil)
	engine.Prime("hot")
	out, _ := engine.Evaluate(t0, map[string]signal.Sample{"temp": good("temp", signal.Num(35), t0, time.Second)})
	if out.Fired || out.State != StateAlreadyFired {
		t.Fatalf("primed row replayed: %+v", out)
	}
	out, _ = engine.Evaluate(t0.Add(time.Second), map[string]signal.Sample{"temp": good("temp", signal.Num(20), t0.Add(time.Second), time.Second)})
	if !out.Fired || out.MatchedRow != "cold" {
		t.Fatalf("a different row must still fire: %+v", out)
	}
}

func TestValidationRejectsBadTables(t *testing.T) {
	cases := map[string]Table{
		"default not last": {
			ID: "t", Rows: []Row{{ID: "default"}, {ID: "after", Conditions: []Condition{{Input: "x", Op: "gt", Value: signal.Num(0)}}}},
		},
		"duplicate row": {
			ID: "t", Rows: []Row{{ID: "a"}, {ID: "a"}},
		},
		"bad operator": {
			ID: "t", Rows: []Row{{ID: "a", Conditions: []Condition{{Input: "x", Op: "contains", Value: signal.Label("a")}}}},
		},
		"duplicate action": {
			ID: "t", Rows: []Row{{ID: "a", Actions: []Action{{Output: "o", Value: signal.Num(1)}, {Output: "o", Value: signal.Num(2)}}}},
		},
	}
	for name, table := range cases {
		if err := table.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestBoolConditionAcceptsNumericCoil(t *testing.T) {
	table := Table{ID: "bool", Rows: []Row{{
		ID:         "row",
		Conditions: []Condition{{Input: "run", Op: "eq", Value: signal.Flag(true)}},
		Actions:    []Action{{Output: "set", Value: signal.Num(1)}},
	}}}
	engine, err := New(table, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	out, err := engine.Evaluate(now, map[string]signal.Sample{
		"run": {Input: "run", Value: signal.Num(1), Quality: signal.Good, SourceTime: now, ReceivedTime: now},
	})
	if err != nil || !out.Fired {
		t.Fatalf("numeric coil should satisfy bool condition: %+v %v", out, err)
	}
}
