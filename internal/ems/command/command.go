// Package command runs named device commands: idempotent submission, ordered
// writes, readback confirmation, timeout and "result unknown".
package command

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"easyems/internal/ems/device"
	"easyems/internal/ems/signal"
)

// State is the lifecycle of one command request.
type State string

const (
	Accepted  State = "accepted"
	Executing State = "executing"
	Confirmed State = "confirmed"
	Failed    State = "failed"
	Unknown   State = "unknown"
)

// Request is one submission. ID is supplied by the caller and deduplicates
// retries.
type Request struct {
	ID         string
	DeviceID   string
	CommandID  string
	Actor      string
	Parameters map[string]float64
}

// Outcome is the tracked result of one request.
type Outcome struct {
	RequestID string
	DeviceID  string
	CommandID string
	Actor     string
	State     State
	StartedAt time.Time
	UpdatedAt time.Time
	Writes    []device.Write
	Readback  []device.Readback
	Reason    string
}

// Dispatcher writes command values to a device. It must not retry internally.
// requestID is carried through so queued writes and logs can be correlated.
type Dispatcher interface {
	Write(deviceID, requestID string, writes []device.Write) error
}

// Manager tracks commands until they confirm, fail or time out. A timeout is
// reported as Unknown: the device may or may not have executed the command.
type Manager struct {
	Mu       sync.Mutex
	Default  time.Duration
	Profiles func(deviceID string) (device.Profile, bool)
	Samples  func(deviceID string, now time.Time) map[string]signal.Sample
	MaxAge   time.Duration
	Writer   Dispatcher
	orders   map[string]Outcome
	order    []string
}

// New returns an empty manager with a default command timeout.
func New(writer Dispatcher, profiles func(string) (device.Profile, bool), samples func(string, time.Time) map[string]signal.Sample, maxAge, timeout time.Duration) *Manager {
	return &Manager{
		Default:  timeout,
		Profiles: profiles,
		Samples:  samples,
		MaxAge:   maxAge,
		Writer:   writer,
		orders:   make(map[string]Outcome),
	}
}

// Submit accepts a command once per request id. A repeated id returns the
// existing outcome without writing again.
func (m *Manager) Submit(now time.Time, request Request) (Outcome, error) {
	if request.ID == "" || request.DeviceID == "" || request.CommandID == "" {
		return Outcome{}, errors.New("request id, device and command are required")
	}
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if existing, ok := m.orders[request.ID]; ok {
		if existing.DeviceID != request.DeviceID || existing.CommandID != request.CommandID {
			return Outcome{}, fmt.Errorf("request id %s already used for another command", request.ID)
		}
		return existing, nil
	}
	profile, ok := m.Profiles(request.DeviceID)
	if !ok {
		return Outcome{}, fmt.Errorf("unknown device %s", request.DeviceID)
	}
	command, ok := profile.Command(request.CommandID)
	if !ok {
		return Outcome{}, fmt.Errorf("device %s has no command %s", request.DeviceID, request.CommandID)
	}
	if err := profile.Validate(); err != nil {
		return Outcome{}, err
	}

	writes := append([]device.Write(nil), command.Writes...)
	readback := append([]device.Readback(nil), command.Readback...)
	allowed := make(map[string]bool, len(command.Parameters))
	for _, parameter := range command.Parameters {
		allowed[parameter.Name] = true
		raw, supplied := request.Parameters[parameter.Name]
		if !supplied {
			raw = parameter.Default
		}
		if parameter.HasRange && (raw < parameter.Min || raw > parameter.Max) {
			return Outcome{}, fmt.Errorf("parameter %s is outside [%g, %g]", parameter.Name, parameter.Min, parameter.Max)
		}
		var value signal.Value
		if parameter.Kind == signal.Bool {
			if raw != 0 && raw != 1 {
				return Outcome{}, fmt.Errorf("parameter %s must be 0 or 1", parameter.Name)
			}
			value = signal.Flag(raw != 0)
		} else {
			value = signal.Num(raw)
		}
		writes = append(writes, device.Write{Point: parameter.Point, Value: value})
		hasReadback := false
		for _, condition := range readback {
			if condition.Point == parameter.Point {
				hasReadback = true
				break
			}
		}
		if !hasReadback {
			readback = append(readback, device.Readback{Point: parameter.Point, Op: "eq", Value: value})
		}
	}
	for name := range request.Parameters {
		if !allowed[name] {
			return Outcome{}, fmt.Errorf("unknown parameter %s", name)
		}
	}

	outcome := Outcome{
		RequestID: request.ID, DeviceID: request.DeviceID, CommandID: request.CommandID,
		Actor: request.Actor, State: Executing, StartedAt: now, UpdatedAt: now,
		Writes: writes, Readback: readback,
	}
	if m.Writer == nil {
		outcome.State = Failed
		outcome.Reason = "no device writer"
		m.store(outcome)
		return outcome, errors.New("no device writer")
	}
	if err := m.Writer.Write(request.DeviceID, request.ID, writes); err != nil {
		outcome.State = Failed
		outcome.Reason = "write failed: " + err.Error()
		outcome.UpdatedAt = now
		m.store(outcome)
		return outcome, nil
	}
	m.store(outcome)
	return outcome, nil
}

func (m *Manager) store(outcome Outcome) {
	if _, exists := m.orders[outcome.RequestID]; !exists {
		m.order = append(m.order, outcome.RequestID)
	}
	m.orders[outcome.RequestID] = outcome
}

// SetProfiles replaces the profile resolver used by submissions and ticks.
func (m *Manager) SetProfiles(profiles func(deviceID string) (device.Profile, bool)) {
	m.Mu.Lock()
	m.Profiles = profiles
	m.Mu.Unlock()
}

// Tick advances executing commands: readback confirms, timeout becomes unknown.
func (m *Manager) Tick(now time.Time) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	for _, id := range m.order {
		outcome := m.orders[id]
		if outcome.State != Executing {
			continue
		}
		profile, ok := m.Profiles(outcome.DeviceID)
		if !ok {
			outcome.State = Failed
			outcome.Reason = "device disappeared"
			outcome.UpdatedAt = now
			m.orders[id] = outcome
			continue
		}
		timeout := m.Default
		if command, found := profile.Command(outcome.CommandID); found && command.Timeout > 0 {
			timeout = command.Timeout
		}
		if confirmed, reason := device.CheckReadback(outcome.Readback, m.Samples(outcome.DeviceID, now), now, m.MaxAge); confirmed {
			outcome.State = Confirmed
			outcome.Reason = ""
			outcome.UpdatedAt = now
			m.orders[id] = outcome
			continue
		} else if now.Sub(outcome.StartedAt) >= timeout {
			outcome.State = Unknown
			outcome.Reason = "no readback before timeout: " + reason
			outcome.UpdatedAt = now
			m.orders[id] = outcome
		}
	}
}

// Get returns a tracked outcome.
func (m *Manager) Get(id string) (Outcome, bool) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	outcome, ok := m.orders[id]
	return outcome, ok
}

// List returns outcomes in submission order.
func (m *Manager) List() []Outcome {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	outcomes := make([]Outcome, 0, len(m.order))
	for _, id := range m.order {
		outcomes = append(outcomes, m.orders[id])
	}
	return outcomes
}
