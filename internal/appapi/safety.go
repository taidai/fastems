package appapi

import (
	"encoding/json"
	"net/http"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/decision"
	"fastems/internal/ems/safety"
)

func (h *Handler) putSafety(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Node   string        `json:"node"`
		Safety safety.Config `json:"safety"`
	}
	if !decodeJSON(w, r, &input) || input.Node == "" || !h.allows(identity, input.Node) {
		writeError(w, http.StatusBadRequest, "invalid_safety")
		return
	}
	nodes, err := client.GetNodes(h.nc, "all", input.Node, "", false)
	if err != nil || len(nodes) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_safety")
		return
	}
	if err := decision.ValidateTarget(h.nc, nodes[0].Parent, decision.Target{NodeID: input.Node, PointType: data.PointTypeValueSet}, true, nil); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_safety")
		return
	}
	if _, err := input.Safety.Output(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_safety")
		return
	}
	for _, condition := range input.Safety.Conditions {
		if err := decision.ValidateTarget(h.nc, nodes[0].Parent, decision.Target{NodeID: condition.PointID, PointType: data.PointTypeValue}, false, nil); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_safety")
			return
		}
	}
	encoded, _ := json.Marshal(input.Safety)
	point := data.NewPointString("easyemsSafety", "", string(encoded))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, input.Node, point, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]any{"saved": true})
}
