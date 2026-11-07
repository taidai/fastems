package appapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

type RuleVersion struct {
	ID          string            `json:"id"`
	PublishedAt time.Time         `json:"published_at"`
	PublishedBy string            `json:"published_by"`
	Document    decision.Document `json:"document"`
}

func (h *Handler) copyRule(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	rule, ok := h.rule(w, identity, id)
	if !ok {
		return
	}
	var input struct {
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Description == "" {
		input.Description = rule.Desc() + " 副本"
	}
	draft, _ := rule.Points.Text("draft", "")
	newID := data.NewID()
	if err := client.SendNodeType(h.nc, decision.DecisionTable{
		ID: newID, Parent: rule.Parent, Description: input.Description, Draft: draft,
	}, identity.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "copy_failed")
		return
	}
	writeJSON(w, map[string]string{"id": newID})
}

func (h *Handler) publishRule(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	rule, ok := h.rule(w, identity, id)
	if !ok {
		return
	}
	var document decision.Document
	if !decodeJSON(w, r, &document) {
		return
	}
	h.publishDocument(w, identity, rule, document)
}

func (h *Handler) publishDocument(w http.ResponseWriter, identity Identity, rule data.NodeEdge, document decision.Document) {
	if err := document.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.documentInScope(rule, document) {
		writeError(w, http.StatusForbidden, "target_out_of_scope")
		return
	}
	if enabled, _ := rule.Points.Find("enabled", ""); enabled.Val() != 0 {
		h.ownershipMu.Lock()
		defer h.ownershipMu.Unlock()
		if err := decision.ValidateOutputOwnership(h.nc, rule.ID, document); err != nil {
			writeError(w, http.StatusConflict, "output_owned")
			return
		}
	}
	encoded, _ := json.Marshal(document)
	previous, _ := rule.Points.Text("applied", "")
	if previous == string(encoded) {
		writeJSON(w, map[string]any{"saved": true, "unchanged": true})
		return
	}

	var history data.Point
	if previous != "" {
		var previousDocument decision.Document
		if json.Unmarshal([]byte(previous), &previousDocument) != nil {
			writeError(w, http.StatusConflict, "invalid_published_version")
			return
		}
		version := RuleVersion{
			ID: data.NewID(), PublishedAt: time.Now(), PublishedBy: identity.UserID,
			Document: previousDocument,
		}
		versionText, _ := json.Marshal(version)
		history = data.NewPointString("history", version.ID, string(versionText))
		history.Origin = identity.UserID
		if err := client.SendNodePoint(h.nc, rule.ID, history, true); err != nil {
			writeError(w, http.StatusInternalServerError, "version_save_failed")
			return
		}
	}
	point := data.NewPointString("applied", "", string(encoded))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, rule.ID, point, true); err != nil {
		if previous != "" {
			history.Tombstone = 1
			_ = client.SendNodePoint(h.nc, rule.ID, history, true)
		}
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	draft := data.NewPointString("draft", "", string(encoded))
	draft.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, rule.ID, draft, true); err != nil {
		writeError(w, http.StatusInternalServerError, "draft_sync_failed")
		return
	}
	writeJSON(w, map[string]any{"saved": true})
}

// runScenarios evaluates every saved scenario of the submitted document. It is
// pure simulation and never sends points.
func (h *Handler) runScenarios(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if _, ok := h.rule(w, identity, id); !ok {
		return
	}
	var input struct {
		Document decision.Document `json:"document"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := input.Document.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"results": input.Document.RunScenarios(time.Now())})
}

func (h *Handler) restoreRule(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	rule, ok := h.rule(w, identity, id)
	if !ok {
		return
	}
	var input struct {
		VersionID string `json:"version_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	point, found := rule.Points.Find("history", input.VersionID)
	if !found || point.Tombstone%2 != 0 {
		writeError(w, http.StatusNotFound, "version_not_found")
		return
	}
	var version RuleVersion
	if json.Unmarshal([]byte(point.Txt()), &version) != nil {
		writeError(w, http.StatusConflict, "invalid_history_version")
		return
	}
	h.publishDocument(w, identity, rule, version.Document)
}
