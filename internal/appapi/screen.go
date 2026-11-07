package appapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

const screenPointType = "easyemsScreen"

// getScreen returns the saved running-screen layout of one business node.
func (h *Handler) getScreen(w http.ResponseWriter, identity Identity, id string) {
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	point, found := node.Points.Find(screenPointType, "")
	if !found || point.Tombstone%2 != 0 {
		writeError(w, http.StatusNotFound, "screen_not_found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(point.Txt()))
}

// putScreen stores the layout as a point on the node, so it lives in the same
// JetStream data as everything else and survives restarts.
func (h *Handler) putScreen(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	if node.Type != data.NodeTypeGroup && node.Type != data.NodeTypeDevice {
		writeError(w, http.StatusBadRequest, "screen_requires_group")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<10))
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_screen")
		return
	}
	var probe map[string]any
	if json.Unmarshal(body, &probe) != nil {
		writeError(w, http.StatusBadRequest, "invalid_screen")
		return
	}
	point := data.NewPointString(screenPointType, "", string(body))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}
