package alarmwatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/ems/alarm"
	"fastiot/internal/ems/alarmtype"
)

// Watcher scans product nodes and feeds the alarm engine.
type Watcher struct {
	NC       *nats.Conn
	Alarms   *alarm.Engine
	Interval time.Duration
	raised   map[string]bool
	pending  map[string]time.Time
	known    map[string]bool
}

// NewWatcher returns a watcher for the given engine.
func NewWatcher(nc *nats.Conn, alarms *alarm.Engine) *Watcher {
	return &Watcher{
		NC: nc, Alarms: alarms, Interval: 2 * time.Second,
		raised: map[string]bool{}, pending: map[string]time.Time{}, known: map[string]bool{},
	}
}

// Run scans until the context ends.
func (w *Watcher) Run(ctx context.Context) error {
	interval := w.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if err := w.Scan(time.Now()); err != nil {
		log.Println("alarm watch: initial scan:", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.Scan(time.Now()); err != nil {
				log.Println("alarm watch: scan:", err)
			}
		}
	}
}

// Scan evaluates all nodes and applies the findings.
func (w *Watcher) Scan(now time.Time) error {
	nodes := make([]data.NodeEdge, 0)
	for _, nodeType := range []string{"decisionTable", "modbus", "variable", "modbusIo", "device", "group"} {
		list, err := client.GetNodes(w.NC, "all", "all", nodeType, false)
		if err != nil {
			return err
		}
		nodes = append(nodes, list...)
	}
	views := make([]NodeView, 0, len(nodes))
	byID := make(map[string]NodeView, len(nodes))
	for _, node := range nodes {
		view := NodeView{ID: node.ID, Type: node.Type, Text: map[string]string{}, Num: map[string]float64{}}
		for _, point := range node.Points {
			if point.Tombstone%2 != 0 {
				continue
			}
			view.Text[point.Type] = point.Txt()
			if value, err := point.ValueFloat(); err == nil {
				view.Num[point.Type] = value
				if point.Type == data.PointTypeValue {
					view.Value = value
					view.HasValue = true
					view.ValueTime = point.Time
				}
			}
			if point.Type == "description" {
				view.Created = point.Time
			}
		}
		if node.Type == data.NodeTypeModbus {
			view.Offline = channelOffline(view, now)
		}
		views = append(views, view)
		byID[node.ID] = view
	}
	findings := Evaluate(views, now)
	active := make(map[string]bool, len(findings))
	for _, finding := range findings {
		active[finding.Key] = true
		_, _ = w.Alarms.Raise(now, finding.Key, finding.Severity, finding.Title, finding.Detail, finding.DeviceID, "")
	}
	if err := w.evaluateTypes(nodes, byID, now, active); err != nil {
		return err
	}
	for key := range w.raised {
		if !active[key] {
			_, _ = w.Alarms.Clear(now, key)
		}
	}
	w.raised = active
	return nil
}

type typeEntry struct {
	root   string
	config alarmtype.Config
}

func (w *Watcher) evaluateTypes(nodes []data.NodeEdge, byID map[string]NodeView, now time.Time, active map[string]bool) error {
	configs := make(map[string]typeEntry)
	for _, node := range nodes {
		for _, point := range node.Points {
			if point.Type != "easyemsAlarmType" || point.Tombstone%2 != 0 {
				continue
			}
			var config alarmtype.Config
			if json.Unmarshal([]byte(point.Txt()), &config) != nil || config.Validate() != nil {
				continue
			}
			key := "alarm-type:" + node.ID + ":" + config.Code
			configs[key] = typeEntry{root: node.ID, config: config}
			w.known[key] = true
		}
	}
	for key, entry := range configs {
		config := entry.config
		if !config.Enabled {
			delete(w.pending, key)
			w.clearType(now, key, config.RecordRecovery)
			continue
		}
		source, ok := byID[config.PointID]
		sample := alarmtype.Sample{}
		if ok {
			sample = alarmtype.Sample{
				Present: source.HasValue, Offline: source.Offline,
				Value: source.Value, ValueTime: source.ValueTime,
			}
		}
		if !config.Active(now, sample) {
			delete(w.pending, key)
			w.clearType(now, key, config.RecordRecovery)
			continue
		}
		active[key] = true
		first, seen := w.pending[key]
		if !seen {
			w.pending[key] = now
			first = now
		}
		if now.Sub(first) < time.Duration(config.DelaySeconds)*time.Second {
			continue
		}
		detail := config.Summary()
		if ok {
			name := source.Text["description"]
			if name == "" {
				name = config.PointID
			}
			detail = fmt.Sprintf("%s · 当前 %g · 条件 %s", name, source.Value, config.Summary())
		}
		_, _ = w.Alarms.Raise(now, key, alarm.Severity(config.Severity), config.Name, detail, config.PointID, "")
		if config.Confirm == "auto" {
			_, _ = w.Alarms.Acknowledge(now, key, "system")
		}
		delete(w.pending, key)
	}
	for key := range w.known {
		if _, ok := configs[key]; ok {
			continue
		}
		_, _ = w.Alarms.Clear(now, key)
		_, _ = w.Alarms.Close(now, key)
		delete(w.known, key)
	}
	return nil
}

func (w *Watcher) clearType(now time.Time, key string, recordRecovery bool) {
	if _, err := w.Alarms.Clear(now, key); err != nil {
		return
	}
	if !recordRecovery {
		_, _ = w.Alarms.Close(now, key)
	}
}

func channelOffline(view NodeView, now time.Time) bool {
	if view.Num["disabled"] != 0 {
		return false
	}
	since, found := view.Num["lastGoodPoll"]
	poll := view.Num["pollPeriod"]
	timeout := view.Num["timeout"]
	maxAge := 5 * time.Second
	if value := time.Duration(poll) * 5 * time.Second / 1000; value > maxAge {
		maxAge = value
	}
	if value := time.Duration(timeout) * 3 * time.Second / 1000; value > maxAge {
		maxAge = value
	}
	if !found || since <= 0 {
		if !view.Created.IsZero() && now.Sub(view.Created) < maxAge {
			return false
		}
		return true
	}
	age := now.Sub(time.Unix(int64(since), 0))
	if age < -5*time.Second {
		age = -age
	}
	return age > maxAge
}
