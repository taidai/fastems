// Package alarm tracks alarm lifecycles: raise, deduplicate, acknowledge,
// resolve and close, with history-friendly timestamps.
package alarm

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// defaultFloodWindow is how long a rapid re-activation of the same key is
// treated as flapping rather than a new occurrence.
const defaultFloodWindow = 60 * time.Second

// Severity is the operational weight of an alarm.
type Severity string

const (
	Info     Severity = "info"
	Warning  Severity = "warning"
	Critical Severity = "critical"
)

// State is the lifecycle state. A resolved alarm stays visible until closed.
type State string

const (
	Active       State = "active"
	Acknowledged State = "acknowledged"
	Resolved     State = "resolved"
	Closed       State = "closed"
)

// Alarm is one deduplicated issue on one object.
type Alarm struct {
	ID                string    `json:"id"`  // stable key
	Key               string    `json:"key"` // dedupe key, same as ID
	Severity          Severity  `json:"severity"`
	Title             string    `json:"title"`
	Detail            string    `json:"detail"`
	DeviceID          string    `json:"device_id,omitempty"`
	PointID           string    `json:"point_id,omitempty"`
	RootCause         string    `json:"root_cause,omitempty"`
	GroupID           string    `json:"group_id,omitempty"`
	Active            bool      `json:"active"`
	State             State     `json:"state"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Count             int       `json:"count,omitempty"`
	LastRaisedAt      time.Time `json:"last_raised_at,omitempty"`
	AckedBy           string    `json:"acked_by,omitempty"`
	AckNote           string    `json:"ack_note,omitempty"`
	MaintenanceUntil  time.Time `json:"maintenance_until,omitempty"`
	MaintenanceReason string    `json:"maintenance_reason,omitempty"`
	MaintainedBy      string    `json:"maintained_by,omitempty"`
}

// Engine holds alarms by key. Safe for concurrent use.
type Maintenance struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`
	By     string    `json:"by"`
}

type Engine struct {
	OnChange    func(Alarm, string)
	FloodWindow time.Duration
	mu          sync.Mutex
	alarms      map[string]*Alarm
	maintenance map[string]Maintenance
	roots       map[string]string
	order       []string
}

// New returns an empty alarm engine.
func New() *Engine {
	return &Engine{
		alarms:      make(map[string]*Alarm),
		maintenance: make(map[string]Maintenance),
		roots:       make(map[string]string),
		FloodWindow: defaultFloodWindow,
	}
}

// Raise creates or refreshes an alarm. Repeating the same condition does not
// create duplicates; a resolved alarm that recurs becomes active again. Alarms
// sharing a root cause (point or device) are grouped under the first one raised
// for that cause. A rapid re-activation of the same key within the flood window
// is suppressed: the occurrence count and last-raised time are updated instead
// of starting a new record.
func (e *Engine) Raise(now time.Time, key string, severity Severity, title, detail, deviceID, pointID string) (Alarm, error) {
	if key == "" {
		return Alarm{}, errors.New("alarm key is empty")
	}
	if severity != Info && severity != Warning && severity != Critical {
		return Alarm{}, fmt.Errorf("invalid severity %q", severity)
	}
	rootCause := pointID
	if rootCause == "" {
		rootCause = deviceID
	}
	window := e.FloodWindow
	if window <= 0 {
		window = defaultFloodWindow
	}
	var notify Alarm
	var transition string
	notifyChange := false
	defer func() {
		if notifyChange && e.OnChange != nil {
			e.OnChange(notify, transition)
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	existing, found := e.alarms[key]
	if e.maintained(now, key) {
		if found {
			return *existing, nil
		}
		return Alarm{Key: key}, nil
	}
	if !found {
		created := &Alarm{
			ID: key, Key: key, Severity: severity, Title: title, Detail: detail,
			DeviceID: deviceID, PointID: pointID, RootCause: rootCause,
			Active: true, State: Active, Count: 1, LastRaisedAt: now,
			CreatedAt: now, UpdatedAt: now,
		}
		e.alarms[key] = created
		e.order = append(e.order, key)
		if rootCause != "" {
			if primary, grouped := e.roots[rootCause]; grouped && primary != key {
				created.GroupID = primary
			} else {
				e.roots[rootCause] = key
			}
		}
		notify, transition, notifyChange = *created, "raised", true
		return *created, nil
	}
	changed := existing.Severity != severity || existing.Title != title || existing.Detail != detail
	if !existing.Active {
		if !existing.LastRaisedAt.IsZero() && now.Sub(existing.LastRaisedAt) <= window {
			existing.Active = true
			existing.State = Active
			existing.UpdatedAt = now
			existing.LastRaisedAt = now
			existing.Count++
			return *existing, nil
		}
		existing.Active = true
		existing.State = Active
		existing.CreatedAt = now
		existing.AckedBy = ""
		existing.Count = 1
		existing.LastRaisedAt = now
		changed = true
		notify, transition, notifyChange = *existing, "raised", true
	}
	if changed {
		existing.Severity = severity
		existing.Title = title
		existing.Detail = detail
		existing.UpdatedAt = now
	}
	return *existing, nil
}

// maintained reports whether the key is inside a maintenance window.
func (e *Engine) maintained(now time.Time, key string) bool {
	window, ok := e.maintenance[key]
	return ok && window.Until.After(now)
}

// ExportMaintenance returns a copy of all active maintenance windows.
func (e *Engine) ExportMaintenance() map[string]Maintenance {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]Maintenance, len(e.maintenance))
	for key, window := range e.maintenance {
		out[key] = window
	}
	return out
}

// ImportMaintenance restores persisted maintenance windows.
func (e *Engine) ImportMaintenance(windows map[string]Maintenance) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for key, window := range windows {
		if window.Until.After(time.Now()) {
			e.maintenance[key] = window
		}
	}
}

// Shelve suppresses one known alarm until the given time. A maintained alarm
// remains listed but is not re-raised or newly activated while the window lasts.
func (e *Engine) Shelve(now time.Time, key, reason, user string, until time.Time) (Alarm, error) {
	if key == "" {
		return Alarm{}, errors.New("alarm key is empty")
	}
	if !until.After(now) {
		return Alarm{}, errors.New("maintenance end must be in the future")
	}
	if strings.TrimSpace(reason) == "" {
		return Alarm{}, errors.New("maintenance reason is required")
	}
	var notify Alarm
	notifyChange := false
	defer func() {
		if notifyChange && e.OnChange != nil {
			e.OnChange(notify, "maintenance")
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.maintenance[key] = Maintenance{Until: until, Reason: strings.TrimSpace(reason), By: user}
	alarm, found := e.alarms[key]
	if !found {
		return Alarm{Key: key, MaintenanceUntil: until, MaintenanceReason: strings.TrimSpace(reason), MaintainedBy: user}, nil
	}
	alarm.MaintenanceUntil = until
	alarm.MaintenanceReason = strings.TrimSpace(reason)
	alarm.MaintainedBy = user
	alarm.UpdatedAt = now
	notify, notifyChange = *alarm, true
	return *alarm, nil
}

// Release ends a maintenance window.
func (e *Engine) Release(now time.Time, key string) (Alarm, error) {
	var notify Alarm
	notifyChange := false
	defer func() {
		if notifyChange && e.OnChange != nil {
			e.OnChange(notify, "released")
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.maintenance, key)
	alarm, found := e.alarms[key]
	if !found {
		return Alarm{Key: key}, nil
	}
	alarm.MaintenanceUntil = time.Time{}
	alarm.MaintenanceReason = ""
	alarm.MaintainedBy = ""
	alarm.UpdatedAt = now
	notify, notifyChange = *alarm, true
	return *alarm, nil
}

// Clear marks an alarm as resolved. It stays listed until closed.
func (e *Engine) Clear(now time.Time, key string) (Alarm, error) {
	var notify Alarm
	notifyChange := false
	defer func() {
		if notifyChange && e.OnChange != nil {
			e.OnChange(notify, "resolved")
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	alarm, found := e.alarms[key]
	if !found {
		return Alarm{}, fmt.Errorf("unknown alarm %s", key)
	}
	if !alarm.Active {
		return *alarm, nil
	}
	alarm.Active = false
	alarm.State = Resolved
	alarm.UpdatedAt = now
	notify, notifyChange = *alarm, true
	return *alarm, nil
}

// Acknowledge records who accepted an active alarm. It does not clear it.
func (e *Engine) Acknowledge(now time.Time, key, user string) (Alarm, error) {
	return e.AcknowledgeWithNote(now, key, user, "")
}

// AcknowledgeWithNote records the operator and an optional handling note.
func (e *Engine) AcknowledgeWithNote(now time.Time, key, user, note string) (Alarm, error) {
	var notify Alarm
	notifyChange := false
	defer func() {
		if notifyChange && e.OnChange != nil {
			e.OnChange(notify, "acknowledged")
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	alarm, found := e.alarms[key]
	if !found {
		return Alarm{}, fmt.Errorf("unknown alarm %s", key)
	}
	if user == "" {
		return Alarm{}, errors.New("acknowledging user is required")
	}
	if !alarm.Active || alarm.State == Acknowledged {
		return Alarm{}, fmt.Errorf("alarm %s is not active", key)
	}
	alarm.State = Acknowledged
	alarm.AckedBy = user
	alarm.AckNote = strings.TrimSpace(note)
	alarm.UpdatedAt = now
	notify, notifyChange = *alarm, true
	return *alarm, nil
}

// Close removes a resolved alarm from the active list. An active alarm must be
// cleared first; closing is not a way to hide a live condition.
func (e *Engine) Close(now time.Time, key string) (Alarm, error) {
	var notify Alarm
	notifyChange := false
	defer func() {
		if notifyChange && e.OnChange != nil {
			e.OnChange(notify, "closed")
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	alarm, found := e.alarms[key]
	if !found {
		return Alarm{}, fmt.Errorf("unknown alarm %s", key)
	}
	if alarm.Active {
		return Alarm{}, fmt.Errorf("alarm %s is still active", key)
	}
	if alarm.State == Closed {
		return *alarm, nil
	}
	alarm.State = Closed
	alarm.UpdatedAt = now
	notify, notifyChange = *alarm, true
	return *alarm, nil
}

// Get returns one alarm.
func (e *Engine) Get(key string) (Alarm, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	alarm, ok := e.alarms[key]
	if !ok {
		return Alarm{}, false
	}
	return *alarm, true
}

// List returns alarms with active ones first, then by creation time.
func (e *Engine) List() []Alarm {
	e.mu.Lock()
	defer e.mu.Unlock()
	alarms := make([]Alarm, 0, len(e.order))
	for _, key := range e.order {
		alarms = append(alarms, *e.alarms[key])
	}
	sort.SliceStable(alarms, func(i, j int) bool {
		if alarms[i].Active != alarms[j].Active {
			return alarms[i].Active
		}
		return alarms[i].CreatedAt.Before(alarms[j].CreatedAt)
	})
	return alarms
}

// Active returns the number of alarms that still describe a live condition.
func (e *Engine) Active() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	count := 0
	for _, alarm := range e.alarms {
		if alarm.Active {
			count++
		}
	}
	return count
}
