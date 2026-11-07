package rule

import (
	"fmt"
	"sort"
	"time"

	"easyems/internal/ems/signal"
)

// LimitResult is what the safety layer allows for one requested action.
// A blocked action must not be dispatched; its value is irrelevant.
type LimitResult struct {
	Value   signal.Value
	Blocked bool
	Reason  string
}

// Limiter is the last step before an action becomes a request: range, device
// capability, SOC, temperature, ownership and freshness checks belong here.
type Limiter interface {
	Limit(now time.Time, output string, requested signal.Value) LimitResult
}

// State describes why an evaluation did or did not fire.
type State string

const (
	StateNoMatch      State = "no-match"
	StateWaitingHold  State = "waiting-hold"
	StateCooldown     State = "cooldown"
	StateAlreadyFired State = "already-fired"
	StateFired        State = "fired"
)

// AppliedAction records the requested value and what the limiter allowed.
type AppliedAction struct {
	Output    string
	Requested signal.Value
	Applied   signal.Value
	Blocked   bool
	Reason    string
}

// BlockedRow explains why a row did not match.
type BlockedRow struct {
	RowID  string
	Reason string
}

// Outcome is one evaluation, including the input snapshot used.
type Outcome struct {
	MatchedRow  string
	MatchedName string
	State       State
	Fired       bool
	Actions     []AppliedAction
	Snapshot    []signal.Sample
	Blocked     []BlockedRow
}

// Engine holds the hold/cooldown state of one table version.
type Engine struct {
	table     Table
	limiter   Limiter
	current   string
	heldSince time.Time
	fired     bool
	lastFired map[string]time.Time
}

// New validates the table and its output ownership, then returns an engine.
// A nil owned set disables ownership checks; a nil limiter applies no limits.
func New(t Table, owned map[string]bool, limiter Limiter) (*Engine, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if err := t.ValidateOutputs(owned); err != nil {
		return nil, err
	}
	return &Engine{table: t, limiter: limiter, lastFired: make(map[string]time.Time)}, nil
}

// Reset clears hold, cooldown and match state. Call it when a new version is
// published or bindings change.
func (e *Engine) Reset() {
	e.current = ""
	e.heldSince = time.Time{}
	e.fired = false
	e.lastFired = make(map[string]time.Time)
}

// Prime restores the state after a restart: the named row is treated as
// already fired, so a still-matching row is not replayed.
func (e *Engine) Prime(rowID string) {
	if rowID == "" {
		return
	}
	e.current = rowID
	e.heldSince = time.Now()
	e.fired = true
}

// Evaluate matches the table against a snapshot. The same matched row does not
// fire twice; it must stop matching and pass its cooldown to fire again.
func (e *Engine) Evaluate(now time.Time, samples map[string]signal.Sample) (Outcome, error) {
	row, snapshot, blocked := matchTable(e.table, now, samples)
	out := Outcome{Snapshot: snapshot, Blocked: blocked}
	if row == nil {
		e.current = ""
		e.heldSince = time.Time{}
		e.fired = false
		out.State = StateNoMatch
		return out, nil
	}
	out.MatchedRow = row.ID
	out.MatchedName = row.Name

	if e.current != row.ID {
		e.current = row.ID
		e.heldSince = now
		e.fired = false
	}

	if e.fired {
		out.State = StateAlreadyFired
		return out, nil
	}
	if now.Sub(e.heldSince) < row.Hold {
		out.State = StateWaitingHold
		return out, nil
	}
	if last, ok := e.lastFired[row.ID]; ok && row.Cooldown > 0 && now.Sub(last) < row.Cooldown {
		out.State = StateCooldown
		return out, nil
	}

	out.Actions = e.apply(now, row.Actions)
	e.fired = true
	e.lastFired[row.ID] = now
	out.Fired = true
	out.State = StateFired
	return out, nil
}

func (e *Engine) apply(now time.Time, actions []Action) []AppliedAction {
	applied := make([]AppliedAction, 0, len(actions))
	for _, a := range actions {
		action := AppliedAction{Output: a.Output, Requested: a.Value, Applied: a.Value}
		if e.limiter != nil {
			limit := e.limiter.Limit(now, a.Output, a.Value)
			action.Applied = limit.Value
			action.Blocked = limit.Blocked
			action.Reason = limit.Reason
		}
		applied = append(applied, action)
	}
	return applied
}

// Simulate reports which row matches and what would be requested. It has no
// state, ignores hold and cooldown, and never creates a command.
func Simulate(t Table, now time.Time, samples map[string]signal.Sample, limiter Limiter) (Outcome, error) {
	if err := t.Validate(); err != nil {
		return Outcome{}, err
	}
	row, snapshot, blocked := matchTable(t, now, samples)
	out := Outcome{Snapshot: snapshot, Blocked: blocked}
	if row == nil {
		out.State = StateNoMatch
		return out, nil
	}
	out.MatchedRow = row.ID
	out.MatchedName = row.Name
	out.Actions = make([]AppliedAction, 0, len(row.Actions))
	for _, a := range row.Actions {
		action := AppliedAction{Output: a.Output, Requested: a.Value, Applied: a.Value}
		if limiter != nil {
			limit := limiter.Limit(now, a.Output, a.Value)
			action.Applied = limit.Value
			action.Blocked = limit.Blocked
			action.Reason = limit.Reason
		}
		out.Actions = append(out.Actions, action)
	}
	out.Fired = true
	out.State = StateFired
	return out, nil
}

func matchTable(t Table, now time.Time, samples map[string]signal.Sample) (*Row, []signal.Sample, []BlockedRow) {
	snapshot := snapshotOf(samples)
	blocked := make([]BlockedRow, 0)
	for i := range t.Rows {
		row := &t.Rows[i]
		ok := true
		reason := ""
		for _, c := range row.Conditions {
			sample, found := samples[c.Input]
			if !found {
				ok = false
				reason = fmt.Sprintf("input %s is missing", c.Input)
				break
			}
			usable, why := sample.Usable(now, t.MaxAge)
			if !usable {
				ok = false
				reason = fmt.Sprintf("input %s is not usable: %s", c.Input, why)
				break
			}
			matched, err := compareCondition(sample.Value, c.Op, c.Value)
			if err != nil {
				ok = false
				reason = fmt.Sprintf("input %s: %s", c.Input, err)
				break
			}
			if !matched {
				ok = false
				reason = fmt.Sprintf("input %s = %s does not satisfy %s %s", c.Input, sample.Value, c.Op, c.Value)
				break
			}
		}
		if ok {
			return row, snapshot, blocked
		}
		blocked = append(blocked, BlockedRow{RowID: row.ID, Reason: reason})
	}
	return nil, snapshot, blocked
}

func compareCondition(sample signal.Value, op string, target signal.Value) (bool, error) {
	coerced, err := signal.Coerce(sample, target.Kind)
	if err != nil {
		return false, err
	}
	return coerced.Compare(op, target)
}

func snapshotOf(samples map[string]signal.Sample) []signal.Sample {	inputs := make([]string, 0, len(samples))
	for input := range samples {
		inputs = append(inputs, input)
	}
	sort.Strings(inputs)
	snapshot := make([]signal.Sample, 0, len(inputs))
	for _, input := range inputs {
		snapshot = append(snapshot, samples[input])
	}
	return snapshot
}
