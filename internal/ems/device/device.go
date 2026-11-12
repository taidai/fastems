// Package device defines device profiles: typed points, named commands with
// readback, and the checks that prove a command took effect.
package device

import (
	"fmt"
	"time"

	"fastems/internal/ems/signal"
)

// Point is one value of a device model.
type Point struct {
	ID       string
	Kind     signal.Kind
	Unit     string
	ReadOnly bool
	HasRange bool
	Min      float64
	Max      float64
}

// Write is one point write inside a named command.
type Write struct {
	Point string
	Value signal.Value
}

// Readback is the condition that proves a command took effect.
type Readback struct {
	Point string
	Op    string
	Value signal.Value
}

// Parameter is one operator-supplied value of a named command.
type Parameter struct {
	Name     string
	Label    string
	Point    string
	Kind     signal.Kind
	Unit     string
	HasRange bool
	Min      float64
	Max      float64
	Default  float64
}

// Command is a named device action such as start, stop, standby or reset.
type Command struct {
	ID         string
	Name       string
	Writes     []Write
	Readback   []Readback
	Parameters []Parameter
	Timeout    time.Duration
}

// Profile is the model for one vendor and product.
type Profile struct {
	ID       string
	Vendor   string
	Model    string
	Points   []Point
	Commands []Command
}

// Validate rejects profiles that would produce ambiguous or unwritable
// commands.
func (p Profile) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("profile id is empty")
	}
	points := make(map[string]Point, len(p.Points))
	for _, point := range p.Points {
		if point.ID == "" {
			return fmt.Errorf("profile %s: point without id", p.ID)
		}
		if _, exists := points[point.ID]; exists {
			return fmt.Errorf("profile %s: duplicate point %s", p.ID, point.ID)
		}
		if point.HasRange && !(point.Min < point.Max) {
			return fmt.Errorf("profile %s: point %s has an empty range", p.ID, point.ID)
		}
		if point.HasRange && point.Kind != signal.Number {
			return fmt.Errorf("profile %s: point %s has a range but is not numeric", p.ID, point.ID)
		}
		points[point.ID] = point
	}
	commands := make(map[string]bool, len(p.Commands))
	for _, command := range p.Commands {
		if command.ID == "" {
			return fmt.Errorf("profile %s: command without id", p.ID)
		}
		if commands[command.ID] {
			return fmt.Errorf("profile %s: duplicate command %s", p.ID, command.ID)
		}
		commands[command.ID] = true
		if command.Timeout < 0 {
			return fmt.Errorf("profile %s: command %s has a negative timeout", p.ID, command.ID)
		}
		for _, write := range command.Writes {
			point, ok := points[write.Point]
			if !ok {
				return fmt.Errorf("command %s: writes unknown point %s", command.ID, write.Point)
			}
			if point.ReadOnly {
				return fmt.Errorf("command %s: writes read-only point %s", command.ID, write.Point)
			}
			if write.Value.Kind != point.Kind {
				return fmt.Errorf("command %s: writes %s to %s point %s", command.ID, write.Value.Kind, point.Kind, write.Point)
			}
			if !write.Value.Valid() {
				return fmt.Errorf("command %s: invalid value for %s", command.ID, write.Point)
			}
		}
		for _, readback := range command.Readback {
			point, ok := points[readback.Point]
			if !ok {
				return fmt.Errorf("command %s: readback on unknown point %s", command.ID, readback.Point)
			}
			if !point.Kind.ValidOperator(readback.Op) {
				return fmt.Errorf("command %s: operator %q is not defined for %s", command.ID, readback.Op, point.Kind)
			}
		}
		for _, parameter := range command.Parameters {
			point, ok := points[parameter.Point]
			if !ok {
				return fmt.Errorf("command %s: parameter %s uses unknown point %s", command.ID, parameter.Name, parameter.Point)
			}
			if point.ReadOnly {
				return fmt.Errorf("command %s: parameter %s writes read-only point %s", command.ID, parameter.Name, parameter.Point)
			}
			if parameter.Kind != point.Kind {
				return fmt.Errorf("command %s: parameter %s kind %s does not match point %s", command.ID, parameter.Name, parameter.Kind, parameter.Point)
			}
			if parameter.HasRange && !(parameter.Min <= parameter.Max) {
				return fmt.Errorf("command %s: parameter %s has an empty range", command.ID, parameter.Name)
			}
			if parameter.HasRange && (parameter.Default < parameter.Min || parameter.Default > parameter.Max) {
				return fmt.Errorf("command %s: parameter %s default is outside its range", command.ID, parameter.Name)
			}
		}
	}
	return nil
}

// Point returns the point with the given id.
func (p Profile) Point(id string) (Point, bool) {
	for _, point := range p.Points {
		if point.ID == id {
			return point, true
		}
	}
	return Point{}, false
}

// Command returns the command with the given id.
func (p Profile) Command(id string) (Command, bool) {
	for _, command := range p.Commands {
		if command.ID == id {
			return command, true
		}
	}
	return Command{}, false
}

// Check reports whether every readback condition of a command is satisfied by
// usable samples. This is how a command becomes "confirmed" rather than merely
// "accepted".
func Check(profile Profile, commandID string, samples map[string]signal.Sample, now time.Time, maxAge time.Duration) (bool, string) {
	command, ok := profile.Command(commandID)
	if !ok {
		return false, "unknown command " + commandID
	}
	if len(command.Readback) == 0 {
		return false, "command " + commandID + " has no readback"
	}
	return CheckReadback(command.Readback, samples, now, maxAge)
}

// CheckReadback checks an already resolved readback set (including dynamic
// parameter values).
func CheckReadback(readback []Readback, samples map[string]signal.Sample, now time.Time, maxAge time.Duration) (bool, string) {
	return CheckReadbackSince(readback, samples, now, maxAge, time.Time{})
}

// CheckReadbackSince additionally requires each usable sample to be observed
// after notBefore. A value that already matched before the command was
// submitted must not confirm it. SourceTime is preferred; ReceivedTime is the
// fallback because some adapters only stamp arrival.
func CheckReadbackSince(readback []Readback, samples map[string]signal.Sample, now time.Time, maxAge time.Duration, notBefore time.Time) (bool, string) {
	if len(readback) == 0 {
		return false, "command has no readback"
	}
	for _, condition := range readback {
		sample, found := samples[condition.Point]
		if !found {
			return false, "readback input " + condition.Point + " is missing"
		}
		if usable, why := sample.Usable(now, maxAge); !usable {
			return false, "readback input " + condition.Point + " is not usable: " + why
		}
		if !notBefore.IsZero() {
			observed := sample.SourceTime
			if observed.IsZero() {
				observed = sample.ReceivedTime
			}
			if !observed.After(notBefore) {
				return false, "readback input " + condition.Point + " predates submission"
			}
		}
		matched, err := sample.Value.Compare(condition.Op, condition.Value)
		if err != nil {
			return false, err.Error()
		}
		if !matched {
			return false, fmt.Sprintf("readback %s = %s does not satisfy %s %s", condition.Point, sample.Value, condition.Op, condition.Value)
		}
	}
	return true, ""
}
