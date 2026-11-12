// Package state keeps the latest value, quality and time of every point, so
// the rule engine, allocator, command manager and UI read one consistent view.
package state

import (
	"sort"
	"sync"
	"time"

	"fastiot/internal/ems/channel"
	"fastiot/internal/ems/signal"
)

// Key identifies one point on one device.
type Key struct {
	DeviceID string
	PointID  string
}

// Value is the latest accepted reading of one point.
type Value struct {
	DeviceID     string
	PointID      string
	Value        signal.Value
	Quality      signal.Quality
	SourceTime   time.Time
	ReceivedTime time.Time
}

// Store is safe for concurrent readers and one writer.
type Store struct {
	mu      sync.RWMutex
	values  map[Key]Value
	health  map[string]channel.Health
	updated map[string]time.Time
}

// New returns an empty store.
func New() *Store {
	return &Store{
		values:  make(map[Key]Value),
		health:  make(map[string]channel.Health),
		updated: make(map[string]time.Time),
	}
}

// Apply records successful reads.
func (s *Store) Apply(now time.Time, results []channel.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, result := range results {
		s.values[Key{result.DeviceID, result.PointID}] = Value{
			DeviceID: result.DeviceID, PointID: result.PointID,
			Value: result.Value, Quality: result.Quality,
			SourceTime: result.SourceTime, ReceivedTime: now,
		}
	}
}

// SetHealth records the communication health of one device.
func (s *Store) SetHealth(deviceID string, health channel.Health, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health[deviceID] = health
	s.updated[deviceID] = now
}

// Health returns the last recorded health, or unknown.
func (s *Store) Health(deviceID string) channel.Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if health, ok := s.health[deviceID]; ok {
		return health
	}
	return channel.HealthUnknown
}

// Get returns the effective value: a value whose device is offline, or whose
// source time is older than maxAge, is downgraded to stale.
func (s *Store) Get(now time.Time, deviceID, pointID string, maxAge time.Duration) (Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[Key{deviceID, pointID}]
	if !ok {
		return Value{}, false
	}
	value.Quality = s.effective(value, now, maxAge)
	return value, true
}

// Samples returns every point of a device keyed by point id, ready for the
// rule engine and safety layer.
func (s *Store) Samples(deviceID string, now time.Time, maxAge time.Duration) map[string]signal.Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	samples := make(map[string]signal.Sample)
	for key, value := range s.values {
		if key.DeviceID != deviceID {
			continue
		}
		samples[key.PointID] = signal.Sample{
			Input: key.PointID, Value: value.Value,
			Quality:    s.effective(value, now, maxAge),
			SourceTime: value.SourceTime, ReceivedTime: value.ReceivedTime,
		}
	}
	return samples
}

// Snapshot returns all values sorted by device and point.
func (s *Store) Snapshot(now time.Time, maxAge time.Duration) []Value {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := make([]Value, 0, len(s.values))
	for _, value := range s.values {
		value.Quality = s.effective(value, now, maxAge)
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].DeviceID != values[j].DeviceID {
			return values[i].DeviceID < values[j].DeviceID
		}
		return values[i].PointID < values[j].PointID
	})
	return values
}

func (s *Store) effective(value Value, now time.Time, maxAge time.Duration) signal.Quality {
	if s.health[value.DeviceID] == channel.HealthOffline {
		return signal.Stale
	}
	if value.Quality != signal.Good {
		return value.Quality
	}
	if maxAge > 0 && now.Sub(value.SourceTime) > maxAge {
		return signal.Stale
	}
	return signal.Good
}
