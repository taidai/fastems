// Package safety applies range, capability and freshness limits to actions
// requested by a decision table before they become device commands.
package safety

import (
	"fmt"
	"math"
	"time"

	"easyems/internal/ems/rule"
	"easyems/internal/ems/signal"
)

// Condition must hold before an output is written. Conditions reuse the typed
// signal model so SOC/temperature/status checks are explicit and explainable.
type Condition struct {
	Input string
	Op    string
	Value signal.Value
}

// Output is the safe envelope of one writable output. Capabilities are
// magnitudes: CapPositive applies to discharge, CapNegative to charge. Zero
// means no capability limit.
type Output struct {
	Kind        signal.Kind
	HasRange    bool
	Min         float64
	Max         float64
	CapPositive float64
	CapNegative float64
	// Requires lists inputs that must be usable before this output is written.
	Requires []string
	// Conditions must all compare true; unusable inputs block the write.
	Conditions []Condition
	// Ramp is the maximum change per second. The command layer applies it.
	Ramp float64
}

// Limiter checks requests against outputs and the current samples. Create one
// per evaluation with a snapshot of samples; do not share it across goroutines.
type Limiter struct {
	MaxAge  time.Duration
	Outputs map[string]Output
	Samples map[string]signal.Sample
}

// Limit implements rule.Limiter.
func (l Limiter) Limit(now time.Time, output string, requested signal.Value) rule.LimitResult {
	spec, ok := l.Outputs[output]
	if !ok {
		return rule.LimitResult{Value: requested, Blocked: true, Reason: "output " + output + " has no safety envelope"}
	}
	if spec.Kind != requested.Kind {
		return rule.LimitResult{Value: requested, Blocked: true,
			Reason: fmt.Sprintf("output %s expects %s but got %s", output, spec.Kind, requested.Kind)}
	}
	for _, input := range spec.Requires {
		sample, found := l.Samples[input]
		if !found {
			return rule.LimitResult{Value: requested, Blocked: true, Reason: "required input " + input + " is missing"}
		}
		if usable, why := sample.Usable(now, l.MaxAge); !usable {
			return rule.LimitResult{Value: requested, Blocked: true, Reason: "required input " + input + " is not usable: " + why}
		}
	}
	for _, condition := range spec.Conditions {
		sample, found := l.Samples[condition.Input]
		if !found {
			return rule.LimitResult{Value: requested, Blocked: true, Reason: "safety input " + condition.Input + " is missing"}
		}
		if usable, why := sample.Usable(now, l.MaxAge); !usable {
			return rule.LimitResult{Value: requested, Blocked: true, Reason: "safety input " + condition.Input + " is not usable: " + why}
		}
		coerced, coerceErr := signal.Coerce(sample.Value, condition.Value.Kind)
		if coerceErr != nil {
			return rule.LimitResult{Value: requested, Blocked: true, Reason: "safety input " + condition.Input + ": " + coerceErr.Error()}
		}
		matched, err := coerced.Compare(condition.Op, condition.Value)
		if err != nil {
			return rule.LimitResult{Value: requested, Blocked: true, Reason: "safety input " + condition.Input + ": " + err.Error()}
		}
		if !matched {
			return rule.LimitResult{Value: requested, Blocked: true,
				Reason: fmt.Sprintf("safety condition %s = %s does not satisfy %s %s", condition.Input, sample.Value, condition.Op, condition.Value)}
		}
	}
	if requested.Kind != signal.Number {
		return rule.LimitResult{Value: requested}
	}

	value := requested.Num
	reason := ""
	if spec.HasRange {
		if value < spec.Min {
			value, reason = spec.Min, fmt.Sprintf("clamped to minimum %g", spec.Min)
		} else if value > spec.Max {
			value, reason = spec.Max, fmt.Sprintf("clamped to maximum %g", spec.Max)
		}
	}
	capability := spec.CapPositive
	label := "discharge"
	if value < 0 {
		capability, label = spec.CapNegative, "charge"
	}
	if capability > 0 && math.Abs(value) > capability {
		value = math.Copysign(capability, value)
		reason = fmt.Sprintf("clamped to available %s power %g", label, capability)
	}
	return rule.LimitResult{Value: signal.Num(value), Reason: reason}
}

// RampStep moves current toward target by at most rate per second. It is
// deliberately a pure function so the command layer can persist its own last
// applied value and the behavior stays testable.
func RampStep(current, target, rate float64, elapsed time.Duration) float64 {
	if rate <= 0 || elapsed <= 0 || current == target {
		return current
	}
	delta := rate * elapsed.Seconds()
	if target > current {
		if current+delta >= target {
			return target
		}
		return current + delta
	}
	if current-delta <= target {
		return target
	}
	return current - delta
}
