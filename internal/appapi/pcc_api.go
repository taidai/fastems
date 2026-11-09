package appapi

import (
	"net/http"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// 电站策略（防逆流/需量）的配置入口：把 PCC 反馈绑定到 Plant。
// 这是站级策略，与单柜规则分开——防逆流是整个并网点的事，不是单个柜的。

// savePlantPCC 处理 PUT /plants/{id}/pcc，绑定 PCC 反馈（主表+方向+上下限）。
func (h *Handler) savePlantPCC(w http.ResponseWriter, r *http.Request, identity Identity, plantID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	nodes, err := siotclient.GetNodes(h.nc, "all", plantID, "", false)
	if err != nil || len(nodes) == 0 {
		writeError(w, http.StatusNotFound, "plant_not_found")
		return
	}
	if nodes[0].Type != NodeTypePlant {
		writeError(w, http.StatusBadRequest, "requires_plant")
		return
	}
	var binding decision.PCCFeedback
	if !decodeJSON(w, r, &binding) {
		return
	}
	if binding.MeterPointID == "" {
		writeError(w, http.StatusBadRequest, "meter_point_required")
		return
	}
	if err := savePCCFeedback(h.nc, plantID, binding); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

// getPlantPCC 处理 GET /plants/{id}/pcc。
func (h *Handler) getPlantPCC(w http.ResponseWriter, identity Identity, plantID string) {
	nodes, err := siotclient.GetNodes(h.nc, "all", plantID, "", false)
	if err != nil || len(nodes) == 0 {
		writeError(w, http.StatusNotFound, "plant_not_found")
		return
	}
	binding, ok := loadPCCFeedback(nodes[0])
	writeJSON(w, map[string]any{"binding": binding, "configured": ok})
}

// listPlants 列出电站（供站级策略配置选择）。
func (h *Handler) listPlants(w http.ResponseWriter, identity Identity) {
	nodes, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypePlant, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list_failed")
		return
	}
	plants := make([]map[string]string, 0, len(nodes))
	for _, node := range nodes {
		assetCode, _ := node.Points.Find(PointTypeAssetCode, "")
		description, _ := node.Points.Find(data.PointTypeDescription, "")
		plants = append(plants, map[string]string{"id": node.ID, "asset_code": assetCode.Txt(), "description": description.Txt()})
	}
	writeJSON(w, map[string]any{"plants": plants})
}
