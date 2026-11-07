package appapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/alerts"
)

type AlertView struct {
	alerts.Alert
	NodeName string `json:"node_name"`
}

func (h *Handler) listAlerts(w http.ResponseWriter, r *http.Request, identity Identity) {
	nodes, err := h.visibleNodes(identity)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "alerts_unavailable")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}
	severity := r.URL.Query().Get("severity")
	keyword := strings.ToLower(r.URL.Query().Get("q"))
	result := []AlertView{}
	for _, node := range nodes {
		for _, point := range node.Points {
			if point.Type != "alert" || point.Tombstone%2 != 0 {
				continue
			}
			var alert alerts.Alert
			if json.Unmarshal([]byte(point.Txt()), &alert) != nil {
				continue
			}
			if state == "open" && !alert.Active || state == "resolved" && (alert.Active || alert.Status == "closed") || state == "closed" && alert.Status != "closed" {
				continue
			}
			if severity != "" && alert.Severity != severity {
				continue
			}
			if keyword != "" && !strings.Contains(strings.ToLower(node.Desc()+" "+alert.Title+" "+alert.Detail), keyword) {
				continue
			}
			result = append(result, AlertView{Alert: alert, NodeName: node.Desc()})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Active != result[j].Active {
			return result[i].Active
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	writeJSON(w, map[string]any{"alerts": result})
}

func (h *Handler) alertAction(w http.ResponseWriter, r *http.Request, identity Identity, nodeID, kind string) {
	if !canAction(identity, ActionAlarmAck) {
		writeError(w, http.StatusForbidden, "operation_not_allowed")
		return
	}
	node, ok := h.node(w, identity, nodeID)
	if !ok {
		return
	}
	point, found := node.Points.Find("alert", kind)
	if !found || point.Tombstone%2 != 0 {
		writeError(w, http.StatusNotFound, "alert_not_found")
		return
	}
	var input struct {
		Action   string `json:"action"`
		Assignee string `json:"assignee"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	var alert alerts.Alert
	if json.Unmarshal([]byte(point.Txt()), &alert) != nil {
		writeError(w, http.StatusConflict, "invalid_alert")
		return
	}
	switch input.Action {
	case "ack":
		if alert.Status == "closed" {
			writeError(w, http.StatusConflict, "alert_closed")
			return
		}
		if alert.Active {
			alert.Status = "acknowledged"
		}
		alert.AckedBy = identity.UserID
	case "assign":
		alert.Assignee = strings.TrimSpace(input.Assignee)
		if alert.Assignee == "" || len(alert.Assignee) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_assignee")
			return
		}
	case "close":
		if alert.Active {
			writeError(w, http.StatusConflict, "alert_still_active")
			return
		}
		alert.Status = "closed"
	default:
		writeError(w, http.StatusBadRequest, "invalid_alert_action")
		return
	}
	alert.UpdatedAt = time.Now()
	encoded, _ := json.Marshal(alert)
	point = data.NewPointString("alert", kind, string(encoded))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, nodeID, point, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, alert)
}
