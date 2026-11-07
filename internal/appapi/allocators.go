package appapi

import (
	"encoding/json"
	"net/http"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// getAllocator returns the configuration stored on one business node.
func (h *Handler) getAllocator(w http.ResponseWriter, identity Identity, id string) {
	if _, ok := h.node(w, identity, id); !ok {
		return
	}
	config, err := decision.LoadAllocator(h.nc, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "allocator_not_found")
		return
	}
	writeJSON(w, config)
}

// putAllocator saves a validated allocator as a point on its business node.
func (h *Handler) putAllocator(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	if node.Type != data.NodeTypeGroup && node.Type != data.NodeTypeDevice {
		writeError(w, http.StatusBadRequest, "allocator_requires_group")
		return
	}
	var config decision.AllocatorConfig
	if !decodeJSON(w, r, &config) {
		return
	}
	if err := decision.ValidateAllocatorScope(h.nc, id, config); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_allocator")
		return
	}
	point := data.NewPointString("easyemsAllocator", "", string(encoded))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}
