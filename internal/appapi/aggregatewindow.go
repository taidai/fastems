package appapi

import (
	"fmt"
	"strings"
	"time"

	"github.com/simpleiot/simpleiot/data"
)

// Aggregate quality policies. They decide how a bad member (stale or missing)
// affects the result quality without changing which members supply the value.
const (
	qualityPolicyIgnore  = "ignore"  // 忽略坏成员：不降级、不标 partial
	qualityPolicyCount   = "count"   // 按坏值计：计入总数，混有坏成员时标 partial（默认）
	qualityPolicyDegrade = "degrade" // 整体降级：任一坏成员即整体降级
)

// normalizeAggregateQuality maps an empty/unset policy to the default.
func normalizeAggregateQuality(policy string) string {
	switch policy {
	case qualityPolicyIgnore, qualityPolicyDegrade:
		return policy
	default:
		return qualityPolicyCount
	}
}

// parseAggregateWindow parses a rolling-window duration. An empty string means
// instantaneous (no window). The window is bounded to at most 24 hours so a
// typo cannot trigger a pathological history scan.
func parseAggregateWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid_aggregate_window")
	}
	if d <= 0 || d > 24*time.Hour {
		return 0, fmt.Errorf("invalid_aggregate_window")
	}
	return d, nil
}

// windowMemberMean reduces a member's samples inside the window to one scalar:
// the mean of its finite samples, plus the latest sample time. A member with no
// finite sample in the window contributes nothing.
func windowMemberMean(samples []data.Point) (mean float64, latest time.Time, ok bool) {
	var sum float64
	count := 0
	for _, p := range samples {
		v, err := p.ValueFloat()
		if err != nil || !finite(v) {
			continue
		}
		sum += v
		count++
		if p.Time.After(latest) {
			latest = p.Time
		}
	}
	if count == 0 {
		return 0, time.Time{}, false
	}
	return sum / float64(count), latest, true
}

// applyAggregateQuality propagates good/bad member counts to a result quality
// and a partial flag. bad counts members that are stale or missing.
func applyAggregateQuality(policy string, good, bad int) (quality string, partial bool) {
	policy = normalizeAggregateQuality(policy)
	if good == 0 {
		return "offline", false
	}
	switch policy {
	case qualityPolicyIgnore:
		return "good", false
	case qualityPolicyDegrade:
		return boolQuality(bad > 0, "degraded", "good")
	default:
		return boolQuality(bad > 0, "partial", "good")
	}
}

func boolQuality(cond bool, yes, no string) (string, bool) {
	if cond {
		return yes, true
	}
	return no, false
}

// aggregateValues folds good-member values under the configured op. The caller
// guarantees values is non-empty.
func aggregateValues(op string, values []float64) float64 {
	switch op {
	case "count":
		return float64(len(values))
	case "sum":
		var v float64
		for _, x := range values {
			v += x
		}
		return v
	case "avg":
		var v float64
		for _, x := range values {
			v += x
		}
		return v / float64(len(values))
	case "min":
		v := values[0]
		for _, x := range values[1:] {
			if x < v {
				v = x
			}
		}
		return v
	case "max":
		v := values[0]
		for _, x := range values[1:] {
			if x > v {
				v = x
			}
		}
		return v
	default:
		return 0
	}
}

// evaluateAggregateAlarm reports whether value breaches an enabled threshold,
// and a short human-readable reason.
func evaluateAggregateAlarm(value float64, cfg *AggregateAlarm) (breach bool, reason string) {
	if cfg == nil || !cfg.Enabled {
		return false, ""
	}
	switch {
	case cfg.Min != nil && value < *cfg.Min:
		return true, fmt.Sprintf("低于下限 %g", *cfg.Min)
	case cfg.Max != nil && value > *cfg.Max:
		return true, fmt.Sprintf("超过上限 %g", *cfg.Max)
	default:
		return false, ""
	}
}

// validateAggregateAlarm rejects alarm configs that cannot be evaluated.
func validateAggregateAlarm(cfg *AggregateAlarm) error {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	if cfg.Severity != "" && cfg.Severity != "info" && cfg.Severity != "warning" && cfg.Severity != "critical" {
		return fmt.Errorf("invalid_aggregate_alarm_severity")
	}
	if cfg.Min == nil && cfg.Max == nil {
		return fmt.Errorf("aggregate_alarm_needs_threshold")
	}
	if cfg.Min != nil && !finite(*cfg.Min) {
		return fmt.Errorf("invalid_aggregate_alarm_threshold")
	}
	if cfg.Max != nil && !finite(*cfg.Max) {
		return fmt.Errorf("invalid_aggregate_alarm_threshold")
	}
	if cfg.Min != nil && cfg.Max != nil && *cfg.Min > *cfg.Max {
		return fmt.Errorf("invalid_aggregate_alarm_threshold")
	}
	return nil
}
