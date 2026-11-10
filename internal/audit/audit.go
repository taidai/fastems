// Package audit keeps a small append-only local audit log.
package audit

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// Store appends JSON lines to a local file, rotating into numbered backups.
// Rotation size, backup count and retention are configurable.
type Store struct {
	Path string
	mu   sync.Mutex
	// MaxBytes rotates the active file when it grows past this size.
	MaxBytes int64
	// Backups is how many rotated files to keep (.1 ... .N).
	Backups int
	// Retention drops records older than this many days at rotation; zero
	// keeps everything.
	RetentionDays int
}

func New(path string) *Store { return &Store{Path: path} }

// Configure applies persisted retention settings.
func (s *Store) Configure(maxBytes int64, backups, retentionDays int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxBytes > 0 {
		s.MaxBytes = maxBytes
	}
	if backups > 0 {
		s.Backups = backups
	}
	if retentionDays >= 0 {
		s.RetentionDays = retentionDays
	}
}

func (s *Store) maxBytes() int64 {
	if s.MaxBytes > 0 {
		return s.MaxBytes
	}
	return 5 << 20
}

func (s *Store) backups() int {
	if s.Backups > 0 {
		return s.Backups
	}
	return 1
}

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
	if info, err := os.Stat(s.Path); err == nil && info.Size() > s.maxBytes() {
		s.rotateLocked()
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

// rotateLocked shifts .1→.2, ... and current→.1, dropping the oldest backup.
func (s *Store) rotateLocked() {
	keep := s.backups()
	for index := keep - 1; index >= 1; index-- {
		_ = os.Rename(s.Path+"."+strconv.Itoa(index), s.Path+"."+strconv.Itoa(index+1))
	}
	if keep >= 1 {
		_ = os.Rename(s.Path, s.Path+".1")
	}
	if s.RetentionDays > 0 {
		s.pruneLocked()
	}
}

// pruneLocked rewrites the active file and backups without expired records.
func (s *Store) pruneLocked() {
	cutoff := time.Now().Add(-time.Duration(s.RetentionDays) * 24 * time.Hour)
	for index := 0; index <= s.backups(); index++ {
		path := s.Path
		if index > 0 {
			path = s.Path + "." + strconv.Itoa(index)
		}
		records, err := readRecords(path)
		if err != nil {
			continue
		}
		kept := records[:0]
		for _, record := range records {
			if record.Time.After(cutoff) {
				kept = append(kept, record)
			}
		}
		if len(kept) == len(records) {
			continue
		}
		rewriteRecords(path, kept)
	}
}

func readRecords(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
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
		records = append(records, record)
	}
	return records, scanner.Err()
}

func rewriteRecords(path string, records []Record) {
	temp := path + ".tmp"
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	writer := bufio.NewWriter(file)
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			continue
		}
		_, _ = writer.Write(append(encoded, '\n'))
	}
	_ = writer.Flush()
	_ = file.Close()
	_ = os.Rename(temp, path)
}

// Query returns newest records matching filter, including rotated backups so
// an export does not silently lose history after rotation.
func (s *Store) Query(filter Filter) ([]Record, error) {
	if s == nil || s.Path == "" {
		return []Record{}, nil
	}
	records := []Record{}
	sequence := 0
	type ordered struct {
		record Record
		seq    int
	}
	orderedRecords := []ordered{}
	for index := 0; index <= s.backups(); index++ {
		path := s.Path
		if index > 0 {
			path = s.Path + "." + strconv.Itoa(index)
		}
		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
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
			orderedRecords = append(orderedRecords, ordered{record: record, seq: sequence})
			sequence++
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil {
			return nil, scanErr
		}
	}
	sort.SliceStable(orderedRecords, func(i, j int) bool {
		if orderedRecords[i].record.Time.Equal(orderedRecords[j].record.Time) {
			return orderedRecords[i].seq > orderedRecords[j].seq
		}
		return orderedRecords[i].record.Time.After(orderedRecords[j].record.Time)
	})
	for _, item := range orderedRecords {
		records = append(records, item.record)
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
