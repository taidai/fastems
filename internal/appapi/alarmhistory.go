package appapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"easyems/internal/ems/alarm"
)

const alarmEventsFile = "alarm-events.jsonl"

type alarmEvent struct {
	Time       time.Time `json:"time"`
	Key        string    `json:"key"`
	Transition string    `json:"transition"`
	Severity   string    `json:"severity"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail,omitempty"`
	DeviceID   string    `json:"device_id,omitempty"`
	State      string    `json:"state"`
}

type AlarmDayStats struct {
	Date      string `json:"date"`
	Raised    int    `json:"raised"`
	Unhandled int    `json:"unhandled"`
	Handled   int    `json:"handled"`
}

type AlarmRanking struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	Count    int    `json:"count"`
}

type AlarmStats struct {
	Today     AlarmDayStats   `json:"today"`
	Yesterday AlarmDayStats   `json:"yesterday"`
	Trend     []AlarmDayStats `json:"trend"`
	Ranking   []AlarmRanking  `json:"ranking"`
	Since     string          `json:"since,omitempty"`
}

func (h *Handler) recordAlarmChange(value alarm.Alarm, transition string) {
	if h.dataDir == "" {
		return
	}
	event := alarmEvent{
		Time: value.UpdatedAt, Key: value.Key, Transition: transition,
		Severity: string(value.Severity), Title: value.Title, Detail: value.Detail,
		DeviceID: value.DeviceID, State: string(value.State),
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.alarmEventsMu.Lock()
	defer h.alarmEventsMu.Unlock()
	if err := os.MkdirAll(h.dataDir, 0700); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(h.dataDir, alarmEventsFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(line, '\n'))
}

func (h *Handler) alarmStats(w http.ResponseWriter, r *http.Request, identity Identity) {
	days := 7
	if value, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && value >= 1 && value <= 30 {
		days = value
	}
	writeJSON(w, h.alarmStatsData(identity, days))
}

func (h *Handler) alarmStatsData(identity Identity, days int) AlarmStats {
	events := h.readAlarmEvents()
	now := time.Now()
	location := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -29)
	byDate := map[string][]alarmEvent{}
	ranking := map[string]int{}
	since := ""
	for _, event := range events {
		at := event.Time.In(location)
		if at.Before(start) {
			continue
		}
		date := at.Format("2006-01-02")
		byDate[date] = append(byDate[date], event)
		if since == "" || at.Format("2006-01-02") < since {
			since = date
		}
		if event.Transition == "raised" && event.DeviceID != "" {
			ranking[event.DeviceID]++
		}
	}
	day := func(date time.Time) AlarmDayStats {
		key := date.Format("2006-01-02")
		agg := AlarmDayStats{Date: key}
		raised := map[string]bool{}
		state := map[string]string{}
		for _, event := range byDate[key] {
			if event.Transition == "raised" {
				raised[event.Key] = true
			}
			state[event.Key] = event.State
		}
		agg.Raised = len(raised)
		for key := range raised {
			if state[key] == string(alarm.Active) {
				agg.Unhandled++
			} else {
				agg.Handled++
			}
		}
		return agg
	}
	trend := make([]AlarmDayStats, 0, days)
	for i := days - 1; i >= 0; i-- {
		trend = append(trend, day(now.AddDate(0, 0, -i)))
	}
	names := map[string]string{}
	if nodes, err := h.visibleNodes(identity); err == nil {
		for _, node := range nodes {
			names[node.ID] = node.Desc()
		}
	}
	ranked := make([]AlarmRanking, 0, len(ranking))
	for id, count := range ranking {
		name := names[id]
		if name == "" {
			name = id
		}
		ranked = append(ranked, AlarmRanking{DeviceID: id, Name: name, Count: count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		return ranked[i].Name < ranked[j].Name
	})
	if len(ranked) > 10 {
		ranked = ranked[:10]
	}
	return AlarmStats{
		Today: day(now), Yesterday: day(now.AddDate(0, 0, -1)),
		Trend: trend, Ranking: ranked, Since: since,
	}
}

func (h *Handler) readAlarmEvents() []alarmEvent {
	if h.dataDir == "" {
		return nil
	}
	file, err := os.Open(filepath.Join(h.dataDir, alarmEventsFile))
	if err != nil {
		return nil
	}
	defer file.Close()
	events := []alarmEvent{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event alarmEvent
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			events = append(events, event)
		}
	}
	return events
}
