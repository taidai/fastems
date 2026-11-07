package decision

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/ems/alloc"
	"easyems/internal/ems/rule"
	"easyems/internal/ems/safety"
	"easyems/internal/ems/signal"
)

// RuleTable converts the product document into the EMS rule engine table.
// The wire format stays compatible with the existing editor: numeric
// conditions and actions, plus optional hold and cooldown milliseconds.
func (d Document) RuleTable() (rule.Table, error) {
	if err := d.Validate(); err != nil {
		return rule.Table{}, err
	}
	rows := make([]rule.Row, 0, len(d.Table.Rows))
	for _, row := range d.Table.Rows {
		conditions := make([]rule.Condition, 0, len(row.Conditions))
		for _, condition := range row.Conditions {
			if condition.Kind == "time" {
				// The clock is a synthetic input; TimeSamples provides it.
				conditions = append(conditions, rule.Condition{
					Input: timeConditionID(condition), Op: "eq", Value: signal.Flag(true),
				})
				continue
			}
			value := signal.Num(condition.Value)
			switch condition.Kind {
			case "bool":
				value = signal.Flag(condition.Value != 0)
			case "enum":
				value = signal.Label(condition.Text)
			}
			conditions = append(conditions, rule.Condition{
				Input: condition.PointID, Op: condition.Operator, Value: value,
			})
		}
		actions := make([]rule.Action, 0, len(row.Actions))
		for _, action := range row.Actions {
			actions = append(actions, rule.Action{Output: action.ActionID, Value: signal.Num(action.Value)})
		}
		rows = append(rows, rule.Row{
			ID: row.ID, Conditions: conditions, Actions: actions,
			Hold:     time.Duration(row.HoldMS) * time.Millisecond,
			Cooldown: time.Duration(row.CooldownMS) * time.Millisecond,
		})
	}
	return rule.Table{ID: "decision", Version: 1, Rows: rows}, nil
}

// SamplesFromValues builds a snapshot for simulation. Simulated inputs are
// good and current; they never touch a device.
func SamplesFromValues(values map[string]float64, texts map[string]string, now time.Time) map[string]signal.Sample {
	samples := make(map[string]signal.Sample, len(values)+len(texts))
	for input, value := range values {
		samples[input] = signal.Sample{
			Input: input, Value: signal.Num(value), Quality: signal.Good,
			SourceTime: now, ReceivedTime: now,
		}
	}
	for input, text := range texts {
		if text == "" {
			continue
		}
		samples[input] = signal.Sample{
			Input: input, Value: signal.Label(text), Quality: signal.Good,
			SourceTime: now, ReceivedTime: now,
		}
	}
	return samples
}

// pointSignal preserves string/enum points as labels and everything else as
// numbers. This keeps typed conditions usable without a second point store.
func pointSignal(point data.Point) (signal.Value, error) {
	if point.DataType == data.PointDataTypeString {
		text := point.Txt()
		if text == "" {
			return signal.Value{}, fmt.Errorf("empty text value")
		}
		return signal.Label(text), nil
	}
	value, err := point.ValueFloat()
	if err != nil {
		return signal.Value{}, err
	}
	return signal.Num(value), nil
}

// targetLimiter enforces product safety and manual takeover before an action
// leaves the engine.
type targetLimiter struct {
	nc      *nats.Conn
	parent  string
	actions map[string]Target
	manual  func(actionID string) bool
	safety  map[string]safety.Output
	values  *map[string]signal.Sample
}

func (l targetLimiter) Limit(now time.Time, output string, requested signal.Value) rule.LimitResult {
	target, ok := l.actions[output]
	if !ok {
		return rule.LimitResult{Blocked: true, Reason: "action is not bound"}
	}
	if l.manual != nil && l.manual(output) {
		return rule.LimitResult{Blocked: true, Reason: "manual takeover"}
	}
	if requested.Kind != signal.Number {
		return rule.LimitResult{Blocked: true, Reason: "only numeric setpoints are supported"}
	}
	value := requested.Num
	if err := ValidateTarget(l.nc, l.parent, target, true, &value); err != nil {
		return rule.LimitResult{Blocked: true, Reason: err.Error()}
	}
	if spec, ok := l.safety[output]; ok {
		samples := map[string]signal.Sample{}
		if l.values != nil {
			samples = *l.values
		}
		limiter := safety.Limiter{
			MaxAge:  15 * time.Second,
			Outputs: map[string]safety.Output{output: spec},
			Samples: samples,
		}
		return limiter.Limit(now, output, requested)
	}
	return rule.LimitResult{Value: requested}
}

// CheckInput and CheckAction describe one evaluation for the lastCheck point.
type CheckInput struct {
	Input      string    `json:"input"`
	Value      float64   `json:"value"`
	Text       string    `json:"text,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Quality    string    `json:"quality"`
	SourceTime time.Time `json:"source_time"`
}

type CheckAction struct {
	Output    string        `json:"output"`
	Requested float64       `json:"requested"`
	Applied   float64       `json:"applied"`
	Blocked   bool          `json:"blocked"`
	Reason    string        `json:"reason"`
	Status    string        `json:"status,omitempty"`
	Shares    []alloc.Share `json:"shares,omitempty"`
}

type CheckReport struct {
	At      time.Time     `json:"at"`
	Row     string        `json:"row"`
	State   string        `json:"state"`
	Inputs  []CheckInput  `json:"inputs"`
	Actions []CheckAction `json:"actions"`
}

func buildCheck(outcome rule.Outcome, now time.Time) CheckReport {
	report := CheckReport{At: now, Row: outcome.MatchedRow, State: string(outcome.State)}
	for _, sample := range outcome.Snapshot {
		value := 0.0
		text := ""
		switch sample.Value.Kind {
		case signal.Number:
			value = sample.Value.Num
		case signal.Bool:
			if sample.Value.Bool {
				value = 1
			}
		case signal.Enum:
			text = sample.Value.Enum
		}
		report.Inputs = append(report.Inputs, CheckInput{
			Input: sample.Input, Value: value, Text: text, Kind: sample.Value.Kind.String(),
			Quality: sample.Quality.String(), SourceTime: sample.SourceTime,
		})
	}
	for _, action := range outcome.Actions {
		requested, applied := 0.0, 0.0
		if action.Requested.Kind == signal.Number {
			requested = action.Requested.Num
		}
		if action.Applied.Kind == signal.Number {
			applied = action.Applied.Num
		}
		report.Actions = append(report.Actions, CheckAction{
			Output: action.Output, Requested: requested, Applied: applied,
			Blocked: action.Blocked, Reason: action.Reason,
		})
	}
	return report
}

func marshalCheck(report CheckReport) (string, error) {
	encoded, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("encode check: %w", err)
	}
	return string(encoded), nil
}

// SimulationAction is the API shape the editor already consumes.
type SimulationAction struct {
	ActionID string  `json:"action_id"`
	Value    float64 `json:"value"`
	Blocked  bool    `json:"blocked,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

// TimeCheck reports one time condition's result at the simulated moment.
type TimeCheck struct {
	Start     string `json:"start,omitempty"`
	End       string `json:"end,omitempty"`
	Weekdays  int    `json:"weekday_mask,omitempty"`
	Satisfied bool   `json:"satisfied"`
}

// SimulationResult reports the matching row and what would be requested. It
// never creates a command.
type SimulationResult struct {
	RowID   string             `json:"row_id,omitempty"`
	Actions []SimulationAction `json:"actions,omitempty"`
	State   string             `json:"state"`
	Blocked []string           `json:"blocked,omitempty"`
	Times   []TimeCheck        `json:"time_checks,omitempty"`
}

// Simulate evaluates the document with the real engine at now, which may be a
// simulated moment. Held and cooldown state are ignored: it answers "which row
// matches at that moment".
func (d Document) Simulate(inputs map[string]float64, texts map[string]string, now time.Time) (SimulationResult, error) {
	table, err := d.RuleTable()
	if err != nil {
		return SimulationResult{}, err
	}
	samples := SamplesFromValues(inputs, texts, now)
	for id, sample := range TimeSamples(d.Table, now) {
		samples[id] = sample
	}
	outcome, err := rule.Simulate(table, now, samples, nil)
	if err != nil {
		return SimulationResult{}, err
	}
	result := SimulationResult{RowID: outcome.MatchedRow, State: string(outcome.State)}
	seenTimes := make(map[string]bool)
	for _, row := range d.Table.Rows {
		for _, condition := range row.Conditions {
			if condition.Kind != "time" || seenTimes[timeConditionID(condition)] {
				continue
			}
			seenTimes[timeConditionID(condition)] = true
			result.Times = append(result.Times, TimeCheck{
				Start: condition.Start, End: condition.End, Weekdays: condition.Weekdays,
				Satisfied: timeMatches(condition, now),
			})
		}
	}
	for _, action := range outcome.Actions {
		value := 0.0
		if action.Applied.Kind == signal.Number {
			value = action.Applied.Num
		}
		result.Actions = append(result.Actions, SimulationAction{
			ActionID: action.Output, Value: value,
			Blocked: action.Blocked, Reason: action.Reason,
		})
	}
	for _, blocked := range outcome.Blocked {
		result.Blocked = append(result.Blocked, blocked.RowID+": "+blocked.Reason)
	}
	return result, nil
}
