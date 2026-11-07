package decision

import (
	"fmt"
	"strconv"
	"time"

	"easyems/internal/ems/signal"
)

// timeConditionID names the synthetic engine input for one time condition, so
// the snapshot and the match explanation show the time check like any input.
func timeConditionID(c Condition) string {
	return fmt.Sprintf("time:%s-%s/%d", c.Start, c.End, c.Weekdays)
}

func validateTimeCondition(c Condition) error {
	if (c.Start == "") != (c.End == "") {
		return fmt.Errorf("time condition needs both start and end")
	}
	if c.Start != "" {
		start, err := parseClock(c.Start)
		if err != nil {
			return err
		}
		end, err := parseClock(c.End)
		if err != nil {
			return err
		}
		if start == end {
			return fmt.Errorf("time condition start and end must differ")
		}
	}
	if c.Weekdays < 0 || c.Weekdays > 127 {
		return fmt.Errorf("weekday mask must be between 0 and 127")
	}
	if c.Start == "" && c.Weekdays == 0 {
		return fmt.Errorf("time condition needs a window or weekdays")
	}
	return nil
}

// parseClock converts "HH:MM" to minutes since midnight.
func parseClock(s string) (int, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("time %q must be HH:MM", s)
	}
	hour, err1 := strconv.Atoi(s[:2])
	minute, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("time %q must be HH:MM", s)
	}
	return hour*60 + minute, nil
}

// timeMatches reports whether now falls in the condition's window and weekday
// mask. The window is half-open [start, end); start after end crosses
// midnight, and the weekday of a cross-midnight window is the day it started.
func timeMatches(c Condition, now time.Time) bool {
	minute := now.Hour()*60 + now.Minute()
	day := now.Weekday()
	if c.Start != "" {
		start, err := parseClock(c.Start)
		if err != nil {
			return false
		}
		end, err := parseClock(c.End)
		if err != nil {
			return false
		}
		if start > end {
			switch {
			case minute >= start:
				// Window started today.
			case minute < end:
				day = (day + 6) % 7
			default:
				return false
			}
		} else if minute < start || minute >= end {
			return false
		}
	}
	return c.Weekdays == 0 || c.Weekdays&(1<<uint(day)) != 0
}

// TimeSamples builds the synthetic inputs for every time condition in the
// table, evaluated at now. Identical conditions share one input.
func TimeSamples(t Table, now time.Time) map[string]signal.Sample {
	var samples map[string]signal.Sample
	for _, row := range t.Rows {
		for _, condition := range row.Conditions {
			if condition.Kind != "time" {
				continue
			}
			id := timeConditionID(condition)
			if _, ok := samples[id]; ok {
				continue
			}
			if samples == nil {
				samples = make(map[string]signal.Sample)
			}
			samples[id] = signal.Sample{
				Input: id, Value: signal.Flag(timeMatches(condition, now)), Quality: signal.Good,
				SourceTime: now, ReceivedTime: now,
			}
		}
	}
	return samples
}
