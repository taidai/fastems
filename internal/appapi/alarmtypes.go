package appapi

import (
	"encoding/json"
	"net/http"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/decision"
	"fastiot/internal/ems/alarmtype"
)

const alarmTypePoint = "easyemsAlarmType"

func (h *Handler) alarmTypes(w http.ResponseWriter, r *http.Request, identity Identity) {
	root := r.URL.Query().Get("root")
	if root == "" || !h.allows(identity, root) {
		writeError(w, http.StatusBadRequest, "invalid_root")
		return
	}
	switch r.Method {
	case http.MethodGet:
		nodes, err := client.GetNodes(h.nc, "all", root, "", false)
		if err != nil || len(nodes) == 0 {
			writeError(w, http.StatusNotFound, "invalid_root")
			return
		}
		types := make([]alarmtype.Config, 0)
		for _, point := range nodes[0].Points {
			if point.Type != alarmTypePoint || point.Tombstone%2 != 0 {
				continue
			}
			var config alarmtype.Config
			if json.Unmarshal([]byte(point.Txt()), &config) == nil {
				types = append(types, config)
			}
		}
		writeJSON(w, map[string]any{"types": types})
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input struct {
			Root string            `json:"root"`
			Type alarmtype.Config  `json:"type"`
		}
		if !decodeJSON(w, r, &input) || input.Root != root {
			writeError(w, http.StatusBadRequest, "invalid_alarm_type")
			return
		}
		if err := input.Type.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_alarm_type")
			return
		}
		if err := h.validateAlarmSource(input.Root, input.Type); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_alarm_source")
			return
		}
		encoded, _ := json.Marshal(input.Type)
		point := data.NewPointString(alarmTypePoint, input.Type.Code, string(encoded))
		point.Origin = identity.UserID
		if err := client.SendNodePoint(h.nc, input.Root, point, true); err != nil {
			writeError(w, http.StatusBadRequest, "save_failed")
			return
		}
		writeJSON(w, map[string]any{"saved": true})
	case http.MethodDelete:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			writeError(w, http.StatusBadRequest, "invalid_alarm_type")
			return
		}
		point := data.NewPointString(alarmTypePoint, code, "")
		point.Tombstone = 1
		point.Origin = identity.UserID
		if err := client.SendNodePoint(h.nc, root, point, true); err != nil {
			writeError(w, http.StatusBadRequest, "save_failed")
			return
		}
		writeJSON(w, map[string]any{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) validateAlarmSource(root string, config alarmtype.Config) error {
	if config.Trigger == "offline" {
		nodes, err := client.GetNodes(h.nc, "all", config.PointID, "", false)
		if err != nil || len(nodes) == 0 || nodes[0].Type != data.NodeTypeModbus {
			return errText("alarm offline source must be a channel")
		}
		return h.withinRoot(root, config.PointID)
	}
	return decision.ValidateTarget(h.nc, root, decision.Target{NodeID: config.PointID, PointType: data.PointTypeValue}, false, nil)
}

func (h *Handler) withinRoot(root, id string) error {
	seen := map[string]bool{}
	frontier := []string{id}
	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]
		if current == root {
			return nil
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		nodes, err := client.GetNodes(h.nc, "all", current, "", false)
		if err != nil || len(nodes) == 0 {
			continue
		}
		frontier = append(frontier, nodes[0].Parent)
	}
	return errText("alarm source is outside the station")
}
