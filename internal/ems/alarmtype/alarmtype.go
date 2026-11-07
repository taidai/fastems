// Package alarmtype defines user-configurable alarm types. A type is stored on
// its station root and evaluated by alarmwatch against current point values.
package alarmtype

import (
	"fmt"
	"regexp"
	"time"
)

var codePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{2,32}$`)

// Config is one user-defined alarm type.
type Config struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Group          string  `json:"group,omitempty"`
	Severity       string  `json:"severity"` // critical | warning | info
	Prompt         string  `json:"prompt,omitempty"`
	Trigger        string  `json:"trigger"` // gt | lt | eq | ne | offline
	PointID        string  `json:"point_id"`
	Value          float64 `json:"value,omitempty"`
	DelaySeconds   int     `json:"delay_seconds"`
	Confirm        string  `json:"confirm"` // manual | auto
	RecordRecovery bool    `json:"record_recovery"`
	Enabled        bool    `json:"enabled"`
}

// Sample is the source value seen by one evaluation.
type Sample struct {
	Present   bool
	Offline   bool
	Value     float64
	ValueTime time.Time
}

// Validate rejects ambiguous or unsupported alarm types.
func (c Config) Validate() error {
	if !codePattern.MatchString(c.Code) {
		return fmt.Errorf("alarm code must be 2-32 letters, digits, _ or -")
	}
	if c.Name == "" {
		return fmt.Errorf("alarm name is required")
	}
	switch c.Severity {
	case "critical", "warning", "info":
	default:
		return fmt.Errorf("unsupported alarm severity %q", c.Severity)
	}
	switch c.Trigger {
	case "gt", "lt", "eq", "ne", "offline":
	default:
		return fmt.Errorf("unsupported alarm trigger %q", c.Trigger)
	}
	if c.PointID == "" {
		return fmt.Errorf("alarm trigger point is required")
	}
	if c.DelaySeconds < 0 || c.DelaySeconds > 86400 {
		return fmt.Errorf("alarm delay must be between 0 and 86400 seconds")
	}
	switch c.Confirm {
	case "manual", "auto":
	default:
		return fmt.Errorf("unsupported confirm mode %q", c.Confirm)
	}
	return nil
}

// Active reports whether the trigger condition currently holds. A missing or
// stale source never triggers a threshold alarm; an offline trigger uses the
// source's health instead.
func (c Config) Active(now time.Time, sample Sample) bool {
	if c.Trigger == "offline" {
		return sample.Offline
	}
	if !sample.Present || sample.Offline {
		return false
	}
	if sample.ValueTime.IsZero() || now.Sub(sample.ValueTime) > 15*time.Second || now.Sub(sample.ValueTime) < -5*time.Second {
		return false
	}
	switch c.Trigger {
	case "gt":
		return sample.Value > c.Value
	case "lt":
		return sample.Value < c.Value
	case "eq":
		return sample.Value == c.Value
	case "ne":
		return sample.Value != c.Value
	}
	return false
}

// Summary is a short human-readable trigger description used by the API/UI.
func (c Config) Summary() string {
	switch c.Trigger {
	case "gt":
		return fmt.Sprintf("> %g", c.Value)
	case "lt":
		return fmt.Sprintf("< %g", c.Value)
	case "eq":
		return fmt.Sprintf("= %g", c.Value)
	case "ne":
		return fmt.Sprintf("≠ %g", c.Value)
	case "offline":
		return "离线"
	}
	return c.Trigger
}
