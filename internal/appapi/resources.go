package appapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/decision"
)

func (h *Handler) updateNode(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	if node.Type != data.NodeTypeGroup && node.Type != data.NodeTypeVariable &&
		node.Type != "decisionTable" && node.Type != "deviceTemplate" {
		writeError(w, http.StatusBadRequest, "use_specific_editor")
		return
	}
	var input struct {
		Description string `json:"description"`
		Disabled    *bool  `json:"disabled"`
	}
	if !decodeJSON(w, r, &input) || strings.TrimSpace(input.Description) == "" {
		writeError(w, http.StatusBadRequest, "invalid_node")
		return
	}
	points := data.Points{data.NewPointString(data.PointTypeDescription, "", strings.TrimSpace(input.Description))}
	if input.Disabled != nil {
		if node.Type != data.NodeTypeVariable {
			writeError(w, http.StatusBadRequest, "unsupported_disable")
			return
		}
		points = append(points, data.NewPointFloat(data.PointTypeDisabled, "", data.BoolToFloat(*input.Disabled)))
	}
	for i := range points {
		points[i].Origin = identity.UserID
	}
	if err := client.SendNodePoints(h.nc, id, points, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (h *Handler) moveNode(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	var input struct {
		OldParent string `json:"old_parent"`
		NewParent string `json:"new_parent"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if node.Parent != input.OldParent {
		writeError(w, http.StatusConflict, "source_parent_mismatch")
		return
	}
	if !h.allows(identity, input.OldParent) {
		writeError(w, http.StatusForbidden, "source_forbidden")
		return
	}
	if !h.allows(identity, input.NewParent) {
		writeError(w, http.StatusForbidden, "destination_forbidden")
		return
	}
	if input.NewParent == id || h.users.IsUnder(input.NewParent, id) {
		writeError(w, http.StatusForbidden, "move_cycle")
		return
	}
	destination, ok := h.node(w, identity, input.NewParent)
	if !ok {
		return
	}
	if destination.Type != data.NodeTypeGroup && destination.Type != data.NodeTypeDevice {
		writeError(w, http.StatusBadRequest, "invalid_destination")
		return
	}
	if node.Type == data.NodeTypeModbusIO || node.Type == data.NodeTypeUser || node.Type == data.NodeTypeDevice {
		writeError(w, http.StatusForbidden, "invalid_move")
		return
	}
	if input.OldParent == input.NewParent {
		writeJSON(w, map[string]bool{"moved": true})
		return
	}
	if node.Type == "decisionTable" {
		applied, _ := node.Points.Text("applied", "")
		if applied != "" {
			var document decision.Document
			check := node
			check.Parent = input.NewParent
			if json.Unmarshal([]byte(applied), &document) != nil || !h.documentInScope(check, document) {
				writeError(w, http.StatusConflict, "rule_targets_outside_destination")
				return
			}
		}
	} else {
		rules, err := h.allRules()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_check_failed")
			return
		}
		for _, rule := range rules {
			if rule.ID == id || h.users.IsUnder(rule.ID, id) {
				continue
			}
			if h.referencesSubtree(rule, id) && input.NewParent != rule.Parent && !h.users.IsUnder(input.NewParent, rule.Parent) {
				writeError(w, http.StatusConflict, "move_would_break_rule: "+rule.Desc())
				return
			}
		}
	}
	if err := client.MoveNode(h.nc, id, input.OldParent, input.NewParent, identity.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "move_failed")
		return
	}
	writeJSON(w, map[string]bool{"moved": true})
}

func (h *Handler) deleteNode(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionDelete) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	var input struct {
		Parent string `json:"parent"`
	}
	if !decodeJSON(w, r, &input) || input.Parent != node.Parent || !h.allows(identity, input.Parent) {
		writeError(w, http.StatusBadRequest, "invalid_parent")
		return
	}
	for _, root := range identity.Anchors {
		if id == root {
			writeError(w, http.StatusForbidden, "cannot_delete_root")
			return
		}
	}
	children, err := client.GetNodes(h.nc, id, "all", "", false)
	if err == nil && len(children) > 0 {
		writeError(w, http.StatusConflict, "has_children")
		return
	}
	if node.Type != "decisionTable" {
		rules, err := h.allRules()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_check_failed")
			return
		}
		for _, rule := range rules {
			if h.referencesSubtree(rule, id) {
				writeError(w, http.StatusConflict, "used_by_rule: "+rule.Desc())
				return
			}
		}
	}
	if err := client.DeleteNode(h.nc, id, input.Parent, identity.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "delete_failed")
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

func (h *Handler) allRules() ([]data.NodeEdge, error) {
	return client.GetNodes(h.nc, "all", "all", "decisionTable", false)
}

func (h *Handler) referencesSubtree(rule data.NodeEdge, id string) bool {
	for _, pointType := range []string{"draft", "applied"} {
		text, _ := rule.Points.Text(pointType, "")
		if text == "" {
			continue
		}
		var document decision.Document
		if json.Unmarshal([]byte(text), &document) != nil {
			continue
		}
		for _, bindings := range []map[string]decision.Target{document.Bindings.Points, document.Bindings.Actions} {
			for _, target := range bindings {
				if target.NodeID == id || h.users.IsUnder(target.NodeID, id) {
					return true
				}
			}
		}
	}
	return false
}
