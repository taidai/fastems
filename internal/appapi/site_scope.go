package appapi

import (
	"net/http"
	"strconv"

	"github.com/simpleiot/simpleiot/data"
)

// 站点作用域：一个项目（root）下可以有多个 Site。界面上选中某个 Site 后，
// 预检、台账与域内计数只应看这个站点的子树；通道、规则等接入层对象挂在项目下，
// 属于全站共用，不因站点切换而消失。

func siteAncestor(id string, byID map[string]data.NodeEdge, siteIDs map[string]bool) (string, bool) {
	seen := map[string]bool{}
	cur := id
	for cur != "" && !seen[cur] {
		seen[cur] = true
		if siteIDs[cur] {
			return cur, true
		}
		node, ok := byID[cur]
		if !ok {
			return "", false
		}
		cur = node.Parent
	}
	return "", false
}

// scopeNodesToSite 返回选中站点的子树 + 未归属任何站点的接入层对象。
// siteID 为空时返回原集合。
func scopeNodesToSite(nodes []data.NodeEdge, siteID string) []data.NodeEdge {
	if siteID == "" {
		return nodes
	}
	byID := make(map[string]data.NodeEdge, len(nodes))
	siteIDs := make(map[string]bool)
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Type == NodeTypeSite {
			siteIDs[node.ID] = true
		}
	}
	out := make([]data.NodeEdge, 0, len(nodes))
	for _, node := range nodes {
		site, underSite := siteAncestor(node.ID, byID, siteIDs)
		if !underSite || site == siteID {
			out = append(out, node)
		}
	}
	return out
}

// siteSubtreeIDs 返回严格属于该站点的节点 ID（不含项目级接入对象）。
func siteSubtreeIDs(nodes []data.NodeEdge, siteID string) map[string]bool {
	ids := make(map[string]bool)
	if siteID == "" {
		return ids
	}
	byID := make(map[string]data.NodeEdge, len(nodes))
	siteIDs := make(map[string]bool)
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Type == NodeTypeSite {
			siteIDs[node.ID] = true
		}
	}
	for _, node := range nodes {
		if site, ok := siteAncestor(node.ID, byID, siteIDs); ok && site == siteID {
			ids[node.ID] = true
		}
	}
	return ids
}

// checkSite 校验一个可选的 site 参数；为空表示项目级（不按站点过滤）。
func (h *Handler) checkSite(w http.ResponseWriter, identity Identity, siteID string) bool {
	if siteID == "" {
		return true
	}
	node, ok := h.node(w, identity, siteID)
	if !ok {
		return false
	}
	if node.Type != NodeTypeSite {
		writeError(w, http.StatusBadRequest, "requires_site")
		return false
	}
	return true
}

// siteLedgerRefs 找出本站点单元实际使用的通道/站号，以及本站子树里的业务节点。
// 设备台账按站点过滤时，用这两类引用判断“这台设备属于哪个站”。
func siteLedgerRefs(nodes []data.NodeEdge, siteID string) (map[string]bool, map[string]bool) {
	refs := map[string]bool{}
	subtree := siteSubtreeIDs(nodes, siteID)
	if len(subtree) == 0 {
		return refs, subtree
	}
	byID := make(map[string]data.NodeEdge, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, unit := range nodes {
		if unit.Type != NodeTypeEnergyUnit || !subtree[unit.ID] {
			continue
		}
		for _, device := range nodes {
			if device.Parent != unit.ID || device.Type != data.NodeTypeDevice {
				continue
			}
			point, ok := device.Points.Find(PointTypePowerSetpoint, "")
			if !ok || point.Txt() == "" {
				continue
			}
			pointNode, ok := byID[point.Txt()]
			if !ok {
				continue
			}
			server, _ := pointNode.Points.ValueInt(data.PointTypeID, "")
			refs[pointNode.Parent+"/"+strconv.Itoa(server)] = true
		}
	}
	return refs, subtree
}
