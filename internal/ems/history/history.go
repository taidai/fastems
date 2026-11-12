// Package history keeps a bounded, per-point sample history. It is the
// in-memory front of the product history; persistence is wired in M4.
package history

import (
	"sort"
	"strings"
	"sync"
	"time"

	"fastems/internal/ems/signal"
)

// Sample is one recorded reading.
type Sample struct {
	DeviceID     string         `json:"device_id"`
	PointID      string         `json:"point_id"`
	Value        signal.Value   `json:"value"`
	Quality      signal.Quality `json:"quality"`
	SourceTime   time.Time      `json:"source_time"`
	ReceivedTime time.Time      `json:"received_time"`
}

// Store keeps the newest Limit samples per point.
type Store struct {
	Limit  int
	mu     sync.RWMutex
	points map[string][]Sample
}

// New returns a store with a per-point cap.
func New(limit int) *Store {
	if limit <= 0 {
		limit = 10000
	}
	return &Store{Limit: limit, points: make(map[string][]Sample)}
}

func key(deviceID, pointID string) string { return deviceID + "\x00" + pointID }

// SetLimit changes the per-point cap and trims existing samples.
func (s *Store) SetLimit(limit int) {
	if limit <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Limit = limit
	for k, list := range s.points {
		if len(list) > limit {
			s.points[k] = append([]Sample(nil), list[len(list)-limit:]...)
		}
	}
}

// Record appends samples, dropping the oldest when a point is full.
func (s *Store) Record(samples []Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sample := range samples {
		k := key(sample.DeviceID, sample.PointID)
		list := s.points[k]
		list = append(list, sample)
		if len(list) > s.Limit {
			list = append([]Sample(nil), list[len(list)-s.Limit:]...)
		}
		s.points[k] = list
	}
}

// Query returns samples of one point whose source time falls inside
// [start, end]. Zero bounds mean unbounded. Results are ascending; when the
// range holds more than limit, the most recent limit samples are returned.
func (s *Store) Query(deviceID, pointID string, start, end time.Time, limit int) []Sample {
	s.mu.RLock()
	list := s.points[key(deviceID, pointID)]
	filtered := make([]Sample, 0, len(list))
	for _, sample := range list {
		if !start.IsZero() && sample.SourceTime.Before(start) {
			continue
		}
		if !end.IsZero() && sample.SourceTime.After(end) {
			continue
		}
		filtered = append(filtered, sample)
	}
	s.mu.RUnlock()
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].SourceTime.Before(filtered[j].SourceTime) })
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

// QueryByPoint 跨设备查询一个点位的历史，供站级计算（如滑动需量）使用。
func (s *Store) QueryByPoint(pointID string, start, end time.Time, limit int) []Sample {
	s.mu.RLock()
	filtered := make([]Sample, 0)
	for key, list := range s.points {
		if !strings.HasSuffix(key, "\x00"+pointID) {
			continue
		}
		for _, sample := range list {
			if !start.IsZero() && sample.SourceTime.Before(start) {
				continue
			}
			if !end.IsZero() && sample.SourceTime.After(end) {
				continue
			}
			filtered = append(filtered, sample)
		}
	}
	s.mu.RUnlock()
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].SourceTime.Before(filtered[j].SourceTime) })
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

// Points lists the points that have history, sorted.
func (s *Store) Points() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	points := make([]string, 0, len(s.points))
	for k := range s.points {
		points = append(points, k)
	}
	sort.Strings(points)
	return points
}

// Len returns how many samples are stored for one point.
func (s *Store) Len(deviceID, pointID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.points[key(deviceID, pointID)])
}
