package appapi

import (
	"encoding/json"
	"net/http"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// ResourceGroup 的用户入口：建组（成员 + 共享上限）与手动组调度。

// createResourceGroup 处理 POST /resource-groups。
func (h *Handler) createResourceGroup(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Description   string   `json:"description"`
		SharedLimitKW float64  `json:"shared_limit_kw"`
		Members       []string `json:"members"` // EnergyUnit IDs
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Description == "" {
		writeError(w, http.StatusBadRequest, "description_required")
		return
	}
	if len(input.Members) == 0 {
		writeError(w, http.StatusBadRequest, "members_required")
		return
	}
	root, err := siotclient.GetRootNode(h.nc)
	if err != nil {
		writeError(w, http.StatusBadGateway, "root_unavailable")
		return
	}
	id, err := CreateDomainNode(h.nc, root.ID, NodeTypeResourceGroup, map[string]string{
		"description": input.Description,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "create_failed")
		return
	}
	if input.SharedLimitKW > 0 {
		limitPoint := data.NewPointFloat(PointTypeSharedLimitKW, "", input.SharedLimitKW)
		limitPoint.Origin = "easyems"
		_ = siotclient.SendNodePoint(h.nc, id, limitPoint, true)
	}
	memberText, _ := json.Marshal(input.Members)
	memberPoint := data.NewPointString(PointTypeGroupMembers, "", string(memberText))
	memberPoint.Origin = "easyems"
	_ = siotclient.SendNodePoint(h.nc, id, memberPoint, true)
	writeJSON(w, map[string]string{"id": id})
}

// dispatchResourceGroup 处理 POST /resource-groups/{id}/dispatch {request_kw}。
func (h *Handler) dispatchResourceGroup(w http.ResponseWriter, r *http.Request, identity Identity, groupID string) {
	if !canAction(identity, ActionWriteValue) {
		writeError(w, http.StatusForbidden, "write_value_required")
		return
	}
	var input struct {
		RequestKW float64 `json:"request_kw"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.RunGroupDispatch(groupID, input.RequestKW)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, result)
}
