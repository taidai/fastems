// Package audit keeps a small append-only local audit log.
package audit

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Record is one audit entry. It never carries passwords, tokens or request bodies.
type Record struct {
	ID     string    `json:"id"`
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Object string    `json:"object,omitempty"`
	Detail string    `json:"detail,omitempty"`
	Result string    `json:"result"`
	IP     string    `json:"ip,omitempty"`
}

// Filter selects audit records.
type Filter struct {
	Actor  string
	Action string
	Result string
	From   time.Time
	To     time.Time
	Limit  int
}

// Store appends JSON lines to a local file and keeps one rotated backup.
type Store struct {
	Path string
	mu   sync.Mutex
}

func New(path string) *Store { return &Store{Path: path} }

func (s *Store) Append(record Record) error {
	if s == nil || s.Path == "" {
		return nil
	}
	if record.ID == "" {
		record.ID = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if record.Time.IsZero() {
		record.Time = time.Now()
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	if info, err := os.Stat(s.Path); err == nil && info.Size() > 5<<20 {
		_ = os.Remove(s.Path + ".1")
		_ = os.Rename(s.Path, s.Path+".1")
	}
	file, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// Query returns newest records matching filter.
func (s *Store) Query(filter Filter) ([]Record, error) {
	if s == nil || s.Path == "" {
		return []Record{}, nil
	}
	file, err := os.Open(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}
	defer file.Close()
	records := []Record{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record Record
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		if filter.Actor != "" && !strings.Contains(strings.ToLower(record.Actor), strings.ToLower(filter.Actor)) {
			continue
		}
		if filter.Action != "" && record.Action != filter.Action {
			continue
		}
		if filter.Result != "" && record.Result != filter.Result {
			continue
		}
		if !filter.From.IsZero() && record.Time.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && record.Time.After(filter.To) {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	if filter.Limit > 0 && len(records) > filter.Limit {
		records = records[:filter.Limit]
	}
	return records, nil
}

// WriteCSV writes filtered records as CSV.
func (s *Store) WriteCSV(w *csv.Writer, records []Record) error {
	if err := w.Write([]string{"时间", "操作人", "动作", "对象", "详情", "结果", "IP", "ID"}); err != nil {
		return err
	}
	for _, record := range records {
		if err := w.Write([]string{
			record.Time.Format("2006-01-02 15:04:05"), record.Actor, record.Action,
			record.Object, record.Detail, record.Result, record.IP, record.ID,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// ParseRange parses a date or RFC3339 value; an empty value is zero time.
func ParseRange(value string, endOfDay bool) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	t, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q", value)
	}
	if endOfDay {
		return t.Add(24*time.Hour - time.Nanosecond), nil
	}
	return t, nil
}
