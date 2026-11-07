// Package signal defines the typed point values, quality and samples shared by
// the decision table engine, the allocator and the runtime.
package signal

import (
	"fmt"
	"math"
	"time"
)

// Kind is the type of a point value. Rules compare values of the same kind.
type Kind uint8

const (
	Number Kind = iota
	Bool
	Enum
)

func (k Kind) String() string {
	switch k {
	case Number:
		return "number"
	case Bool:
		return "bool"
	case Enum:
		return "enum"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

// Value is a typed point value. Only the field matching Kind is meaningful.
type Value struct {
	Kind Kind
	Num  float64
	Bool bool
	Enum string
}

func Num(v float64) Value  { return Value{Kind: Number, Num: v} }
func Flag(v bool) Value    { return Value{Kind: Bool, Bool: v} }
func Label(v string) Value { return Value{Kind: Enum, Enum: v} }

func (v Value) String() string {
	switch v.Kind {
	case Number:
		return fmt.Sprintf("%g", v.Num)
	case Bool:
		return fmt.Sprintf("%t", v.Bool)
	case Enum:
		return v.Enum
	default:
		return fmt.Sprintf("invalid(kind=%d)", v.Kind)
	}
}

// Valid reports whether the value can participate in a comparison. A number
// must be finite; a missing number is never treated as zero.
func (v Value) Valid() bool {
	switch v.Kind {
	case Number:
		return !math.IsNaN(v.Num) && !math.IsInf(v.Num, 0)
	case Bool, Enum:
		return true
	default:
		return false
	}
}

// ValidOperator reports whether op is defined for the kind.
func (k Kind) ValidOperator(op string) bool {
	switch k {
	case Number:
		switch op {
		case "eq", "ne", "gt", "ge", "lt", "le":
			return true
		}
	case Bool, Enum:
		switch op {
		case "eq", "ne":
			return true
		}
	}
	return false
}

// ValidOperator reports whether op is defined for the value kind.
func (v Value) ValidOperator(op string) bool { return v.Kind.ValidOperator(op) }

// Coerce adapts a numeric 0/1 sample to a bool condition, which is how coils
// and status variables are represented by the point store. Other kind
// mismatches remain errors rather than being guessed.
func Coerce(v Value, target Kind) (Value, error) {
	if v.Kind == target {
		return v, nil
	}
	if target == Bool && v.Kind == Number && (v.Num == 0 || v.Num == 1) {
		return Flag(v.Num != 0), nil
	}
	return v, fmt.Errorf("cannot compare %s with %s", v.Kind, target)
}

// Compare applies op to v and other. Both values must have the same kind.
func (v Value) Compare(op string, other Value) (bool, error) {
	if v.Kind != other.Kind {
		return false, fmt.Errorf("cannot compare %s with %s", v.Kind, other.Kind)
	}
	if !v.Valid() || !other.Valid() {
		return false, fmt.Errorf("invalid value")
	}
	if !v.ValidOperator(op) {
		return false, fmt.Errorf("operator %q is not defined for %s", op, v.Kind)
	}
	switch v.Kind {
	case Number:
		switch op {
		case "eq":
			return v.Num == other.Num, nil
		case "ne":
			return v.Num != other.Num, nil
		case "gt":
			return v.Num > other.Num, nil
		case "ge":
			return v.Num >= other.Num, nil
		case "lt":
			return v.Num < other.Num, nil
		case "le":
			return v.Num <= other.Num, nil
		}
	case Bool:
		switch op {
		case "eq":
			return v.Bool == other.Bool, nil
		case "ne":
			return v.Bool != other.Bool, nil
		}
	case Enum:
		switch op {
		case "eq":
			return v.Enum == other.Enum, nil
		case "ne":
			return v.Enum != other.Enum, nil
		}
	}
	return false, fmt.Errorf("operator %q is not defined for %s", op, v.Kind)
}

// Quality describes how much a sample can be trusted.
type Quality uint8

const (
	Uninitialized Quality = iota
	Good
	Stale
	Invalid
)

func (q Quality) String() string {
	switch q {
	case Uninitialized:
		return "uninitialized"
	case Good:
		return "good"
	case Stale:
		return "stale"
	case Invalid:
		return "invalid"
	default:
		return fmt.Sprintf("quality(%d)", q)
	}
}

// Sample is one reading of one input, with its source time. SourceTime is the
// device or rule time; ReceivedTime is when this instance accepted it.
type Sample struct {
	Input        string
	Value        Value
	Quality      Quality
	SourceTime   time.Time
	ReceivedTime time.Time
}

// Usable reports whether the sample may take part in a new decision at now.
// The returned reason is empty when usable.
func (s Sample) Usable(now time.Time, maxAge time.Duration) (bool, string) {
	if s.Quality != Good {
		return false, "quality is " + s.Quality.String()
	}
	if !s.Value.Valid() {
		return false, "value is not finite"
	}
	if s.SourceTime.IsZero() {
		return false, "source time is missing"
	}
	if maxAge > 0 && now.Sub(s.SourceTime) > maxAge {
		return false, "stale by " + now.Sub(s.SourceTime).Round(time.Millisecond).String()
	}
	return true, ""
}
