package appapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

const aggregatePoint = "easyemsAggregate"

type AggregateConfig struct {
	Group   string   `json:"group"`
	Op      string   `json:"op"`
	Members []string `json:"members"`
	Units   string   `json:"units,omitempty"`
}

type AggregateMember struct {
	NodeID  string     `json:"node_id"`
	Name    string     `json:"name"`
	Value   *float64   `json:"value"`
	Quality string     `json:"quality"`
	Time    *time.Time `json:"time,omitempty"`
}

type AggregateResult struct {
	Configured bool              `json:"configured"`
	Config     AggregateConfig   `json:"config"`
	Value      *float64          `json:"value"`
	Quality    string            `json:"quality"`
	Partial    bool              `json:"partial"`
	Valid      int               `json:"valid"`
	Total      int               `json:"total"`
	Time       *time.Time        `json:"time,omitempty"`
	Members    []AggregateMember `json:"members"`
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
	writeJSON(w, result)
}

func (h *Handler) validateAggregate(config AggregateConfig, identity Identity) error {
	if config.Group == "" || !h.allows(identity, config.Group) {
		return errText("invalid_group")
	}
	if !aggregateOps[config.Op] {
		return errText("invalid_aggregate_op")
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
	result := AggregateResult{Configured: true, Config: config, Total: len(config.Members), Members: make([]AggregateMember, 0, len(config.Members))}
	values := make([]float64, 0, len(config.Members))
	var latest time.Time
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
		result.Members = append(result.Members, member)
	}
	sort.Slice(result.Members, func(i, j int) bool { return result.Members[i].Name < result.Members[j].Name })
	result.Valid = len(values)
	if result.Valid == 0 {
		result.Quality = "offline"
		return result, nil
	}
	if !latest.IsZero() {
		result.Time = &latest
	}
	var value float64
	switch config.Op {
	case "count":
		value = float64(result.Valid)
	case "sum", "avg":
		for _, v := range values {
			value += v
		}
		if config.Op == "avg" {
			value /= float64(len(values))
		}
	case "min":
		value = values[0]
		for _, v := range values[1:] {
			if v < value {
				value = v
			}
		}
	case "max":
		value = values[0]
		for _, v := range values[1:] {
			if v > value {
				value = v
			}
		}
	}
	result.Value = &value
	result.Partial = result.Valid < result.Total
	if result.Partial {
		result.Quality = "partial"
	} else {
		result.Quality = "good"
	}
	return result, nil
}

type errText string

func (e errText) Error() string { return string(e) }
