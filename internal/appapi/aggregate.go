package appapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
	"easyems/internal/ems/alarm"
)

const aggregatePoint = "easyemsAggregate"

type AggregateConfig struct {
	Group   string          `json:"group"`
	Op      string          `json:"op"`
	Members []string        `json:"members"`
	Units   string          `json:"units,omitempty"`
	Window  string          `json:"window,omitempty"`  // "" = 瞬时；"5m"/"15m"/"1h" = 滚动窗口
	Quality string          `json:"quality,omitempty"` // ignore | count | degrade（默认 count）
	Alarm   *AggregateAlarm `json:"alarm,omitempty"`
}

// AggregateAlarm configures a threshold alarm on the computed aggregate value.
type AggregateAlarm struct {
	Enabled  bool     `json:"enabled"`
	Min      *float64 `json:"min,omitempty"` // 低于下限告警
	Max      *float64 `json:"max,omitempty"` // 超过上限告警
	Severity string   `json:"severity,omitempty"`
}

// AggregateWindow explains a rolling-window result: its bounds and how many
// members contributed a sample inside the window.
type AggregateWindow struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Members int       `json:"members"`
}

// AggregateAlarmState reflects the alarm engine's current view of the
// aggregate threshold, read back for the result panel.
type AggregateAlarmState struct {
	Active   bool   `json:"active"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitempty"`
}

type AggregateMember struct {
	NodeID  string     `json:"node_id"`
	Name    string     `json:"name"`
	Value   *float64   `json:"value"`
	Quality string     `json:"quality"`
	Time    *time.Time `json:"time,omitempty"`
}

type AggregateResult struct {
	Configured bool                 `json:"configured"`
	Config     AggregateConfig      `json:"config"`
	Value      *float64             `json:"value"`
	Quality    string               `json:"quality"`
	Partial    bool                 `json:"partial"`
	Valid      int                  `json:"valid"`
	Total      int                  `json:"total"`
	Time       *time.Time           `json:"time,omitempty"`
	Members    []AggregateMember    `json:"members"`
	Window     *AggregateWindow     `json:"window,omitempty"`
	Alarm      *AggregateAlarmState `json:"alarm,omitempty"`
}

var aggregateOps = map[string]bool{"sum": true, "avg": true, "min": true, "max": true, "count": true}

func (h *Handler) aggregate(w http.ResponseWriter, r *http.Request, identity Identity) {
	if r.Method == http.MethodPut {
		if !canConfigure(identity) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var config AggregateConfig
		if !decodeJSON(w, r, &config) {
			return
		}
		if err := h.validateAggregate(config, identity); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		encoded, _ := json.Marshal(config)
		point := data.NewPointString(aggregatePoint, "", string(encoded))
		point.Origin = identity.UserID
		if err := client.SendNodePoint(h.nc, config.Group, point, true); err != nil {
			writeError(w, http.StatusBadRequest, "save_failed")
			return
		}
		result, err := h.computeAggregate(config)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "aggregate_unavailable")
			return
		}
		h.applyAggregateSideEffects(&result)
		writeJSON(w, result)
		return
	}
	group := r.URL.Query().Get("group")
	if group == "" {
		writeError(w, http.StatusBadRequest, "missing_group")
		return
	}
	if !h.allows(identity, group) {
		writeError(w, http.StatusForbidden, "no_access")
		return
	}
	config, ok := h.loadAggregate(group)
	if !ok {
		writeJSON(w, AggregateResult{Configured: false, Config: AggregateConfig{Group: group}, Quality: "unconfigured", Members: []AggregateMember{}})
		return
	}
	result, err := h.computeAggregate(config)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "aggregate_unavailable")
		return
	}
	h.applyAggregateSideEffects(&result)
	writeJSON(w, result)
}

func (h *Handler) validateAggregate(config AggregateConfig, identity Identity) error {
	if config.Group == "" || !h.allows(identity, config.Group) {
		return errText("invalid_group")
	}
	if !aggregateOps[config.Op] {
		return errText("invalid_aggregate_op")
	}
	if _, err := parseAggregateWindow(config.Window); err != nil {
		return errText("invalid_aggregate_window")
	}
	if config.Quality != "" && config.Quality != qualityPolicyIgnore && config.Quality != qualityPolicyCount && config.Quality != qualityPolicyDegrade {
		return errText("invalid_aggregate_quality")
	}
	if err := validateAggregateAlarm(config.Alarm); err != nil {
		return errText("invalid_aggregate_alarm")
	}
	if len(config.Members) == 0 || len(config.Members) > 64 {
		return errText("invalid_members")
	}
	nodes, err := client.GetNodes(h.nc, "all", config.Group, "", false)
	if err != nil || len(nodes) == 0 {
		return errText("invalid_group")
	}
	isRoot := false
	for _, anchor := range identity.Anchors {
		if anchor == config.Group {
			isRoot = true
			break
		}
	}
	if nodes[0].Type != data.NodeTypeGroup && !isRoot {
		return errText("invalid_group")
	}
	seen := map[string]bool{}
	for _, member := range config.Members {
		if member == "" || seen[member] {
			return errText("invalid_members")
		}
		seen[member] = true
		if err := decision.ValidateTarget(h.nc, config.Group, decision.Target{NodeID: member, PointType: data.PointTypeValue}, false, nil); err != nil {
			return errText("invalid_members")
		}
	}
	return nil
}

func (h *Handler) loadAggregate(group string) (AggregateConfig, bool) {
	nodes, err := client.GetNodes(h.nc, "all", group, "", false)
	if err != nil || len(nodes) == 0 {
		return AggregateConfig{}, false
	}
	point, ok := nodes[0].Points.Find(aggregatePoint, "")
	if !ok || point.Tombstone%2 != 0 {
		return AggregateConfig{}, false
	}
	var config AggregateConfig
	if json.Unmarshal([]byte(point.Txt()), &config) != nil {
		return AggregateConfig{}, false
	}
	config.Group = group
	return config, true
}

func (h *Handler) computeAggregate(config AggregateConfig) (AggregateResult, error) {
	now := time.Now()
	window, err := parseAggregateWindow(config.Window)
	if err != nil {
		return AggregateResult{}, err
	}
	policy := normalizeAggregateQuality(config.Quality)
	result := AggregateResult{
		Configured: true,
		Config:     config,
		Total:      len(config.Members),
		Members:    make([]AggregateMember, 0, len(config.Members)),
	}
	var reader historyReader
	var windowStart, windowEnd time.Time
	if window > 0 {
		if r, ok := h.users.(historyReader); ok {
			reader = r
		}
		windowStart = now.Add(-window)
		windowEnd = now
	}
	values := make([]float64, 0, len(config.Members))
	var latest time.Time
	windowMembers := 0
	for _, id := range config.Members {
		member := AggregateMember{NodeID: id, Quality: "missing"}
		nodes, err := client.GetNodes(h.nc, "all", id, "", false)
		if err == nil && len(nodes) == 1 {
			node := nodes[0]
			member.Name, _ = node.Points.Text("description", "")
			if member.Name == "" {
				member.Name = node.ID
			}
			disabled, _ := node.Points.Value("disabled", "")
			point, ok := node.Points.Find("value", "")
			if disabled == 0 && ok && point.Tombstone%2 == 0 {
				if reader != nil {
					samples, herr := reader.PointHistory(id, data.PointTypeValue, "", windowStart, windowEnd, 2000)
					if herr == nil {
						if v, t, has := windowMemberMean(samples); has {
							member.Value = &v
							member.Time = &t
							member.Quality = "good"
							values = append(values, v)
							windowMembers++
							if t.After(latest) {
								latest = t
							}
						} else {
							member.Quality = "stale"
						}
					} else {
						member.Quality = "stale"
					}
				} else {
					value, valueErr := point.ValueFloat()
					if valueErr == nil && finite(value) {
						t := point.Time
						member.Value = &value
						member.Time = &t
						if t.IsZero() || now.Sub(t) > 15*time.Second {
							member.Quality = "stale"
						} else {
							member.Quality = "good"
							values = append(values, value)
							if t.After(latest) {
								latest = t
							}
						}
					}
				}
			}
		}
		result.Members = append(result.Members, member)
	}
	sort.Slice(result.Members, func(i, j int) bool { return result.Members[i].Name < result.Members[j].Name })

	good := len(values)
	result.Valid = good
	result.Quality, result.Partial = applyAggregateQuality(policy, good, len(config.Members)-good)

	if window > 0 && reader != nil {
		result.Window = &AggregateWindow{Start: windowStart, End: windowEnd, Members: windowMembers}
	}

	if good == 0 {
		return result, nil
	}
	if !latest.IsZero() {
		result.Time = &latest
	}
	value := aggregateValues(config.Op, values)
	result.Value = &value
	return result, nil
}

// applyAggregateSideEffects materializes the aggregate value (报表绑定) and
// evaluates the threshold alarm (虚拟告警) after each computation.
func (h *Handler) applyAggregateSideEffects(result *AggregateResult) {
	h.persistAggregateValue(*result)
	result.Alarm = h.applyAggregateAlarm(*result)
}

// persistAggregateValue writes the computed value onto the group node so
// history and reports can reference it by node_id.
func (h *Handler) persistAggregateValue(result AggregateResult) {
	if result.Value == nil {
		return
	}
	point := data.NewPointFloat("easyemsAggregateValue", "", *result.Value)
	_ = client.SendNodePoint(h.nc, result.Config.Group, point, false)
}

// applyAggregateAlarm raises or clears the aggregate threshold alarm using the
// existing alarm engine (read-only calls; the engine implementation is not
// modified here).
func (h *Handler) applyAggregateAlarm(result AggregateResult) *AggregateAlarmState {
	group := result.Config.Group
	cfg := result.Config.Alarm
	if h.ems == nil || h.ems.Service == nil || h.ems.Service.Alarms == nil {
		return nil
	}
	engine := h.ems.Service.Alarms
	key := "aggregate:" + group
	now := time.Now()
	severity := alarm.Severity("warning")
	if cfg != nil && cfg.Severity != "" {
		severity = alarm.Severity(cfg.Severity)
	}
	if cfg == nil || !cfg.Enabled || result.Value == nil || result.Quality == "offline" {
		clearAggregateAlarm(engine, key, now)
		return nil
	}
	breach, reason := evaluateAggregateAlarm(*result.Value, cfg)
	if !breach {
		clearAggregateAlarm(engine, key, now)
		return nil
	}
	detail := fmt.Sprintf("聚合值 %g · %s · 有效 %d/%d", *result.Value, reason, result.Valid, result.Total)
	raised, _ := engine.Raise(now, key, severity, "虚拟聚合超限", detail, group, "")
	return &AggregateAlarmState{Active: raised.Active, Severity: string(severity), Detail: detail}
}

func clearAggregateAlarm(engine *alarm.Engine, key string, now time.Time) {
	if a, ok := engine.Get(key); ok && a.Active {
		_, _ = engine.Clear(now, key)
	}
}

type errText string

func (e errText) Error() string { return string(e) }
