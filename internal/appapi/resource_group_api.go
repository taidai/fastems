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
		if !h.allows(identity, node.ID) {
			continue
		}
		description, _ := node.Points.Find(data.PointTypeDescription, "")
		limit, _ := node.Points.Find(PointTypeSharedLimitKW, "")
		members, ok := loadResourceGroup(node)
		ids := []string{}
		visible := true
		if ok {
			for _, member := range members.Members {
				if !h.allows(identity, member.EnergyUnitID) {
					visible = false
					break
				}
				ids = append(ids, member.EnergyUnitID)
			}
		}
		if !visible {
			continue
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
	members, ok := h.checkedEnergyUnits(w, identity, input.Members)
	if !ok {
		return
	}
	parent, err := h.resourceGroupParent(identity, members)
	if err == errNoVisibleParent {
		writeError(w, http.StatusForbidden, "no_access")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "root_unavailable")
		return
	}
	id, err := CreateDomainNode(h.nc, parent, NodeTypeResourceGroup, map[string]string{
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
	if !canAction(identity, ActionCommand) {
		writeError(w, http.StatusForbidden, "command_required")
		return
	}
	node, ok := h.node(w, identity, groupID)
	if !ok {
		return
	}
	if node.Type != NodeTypeResourceGroup {
		writeError(w, http.StatusBadRequest, "requires_resource_group")
		return
	}
	group, configured := loadResourceGroup(node)
	if !configured {
		writeError(w, http.StatusBadRequest, "resource_group_has_no_members")
		return
	}
	memberIDs := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		memberIDs = append(memberIDs, member.EnergyUnitID)
	}
	if _, ok := h.checkedEnergyUnits(w, identity, memberIDs); !ok {
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
