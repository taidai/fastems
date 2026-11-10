package appapi

import (
	"encoding/json"
	"net/http"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// ResourceGroup 的用户入口：建组（成员 + 共享上限）与手动组调度。

// listResourceGroups 列出所有资源集群及其成员与共享上限。
func (h *Handler) listResourceGroups(w http.ResponseWriter, identity Identity) {
	nodes, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypeResourceGroup, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list_failed")
		return
	}
	groups := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		description, _ := node.Points.Find(data.PointTypeDescription, "")
		limit, _ := node.Points.Find(PointTypeSharedLimitKW, "")
		members, ok := loadResourceGroup(node)
		ids := []string{}
		if ok {
			for _, member := range members.Members {
				ids = append(ids, member.EnergyUnitID)
			}
		}
		groups = append(groups, map[string]any{
			"id": node.ID, "description": description.Txt(), "shared_limit_kw": pointFloat(limit), "members": ids,
		})
	}
	writeJSON(w, map[string]any{"groups": groups})
}

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
