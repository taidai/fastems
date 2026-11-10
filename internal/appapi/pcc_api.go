package appapi

import (
	"net/http"
	"time"

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
	node, ok := h.node(w, identity, plantID)
	if !ok {
		return
	}
	if node.Type != NodeTypePlant {
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
	if len(binding.PhasePointIDs) > 3 {
		writeError(w, http.StatusBadRequest, "invalid_phase_bindings")
		return
	}
	for _, pointID := range binding.PhasePointIDs {
		if pointID == "" {
			continue
		}
		if _, ok := h.node(w, identity, pointID); !ok {
			return
		}
	}
	if err := savePCCFeedback(h.nc, plantID, binding); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

// getPlantPCC 处理 GET /plants/{id}/pcc。
func (h *Handler) getPlantPCC(w http.ResponseWriter, identity Identity, plantID string) {
	node, ok := h.node(w, identity, plantID)
	if !ok {
		return
	}
	if node.Type != NodeTypePlant {
		writeError(w, http.StatusBadRequest, "requires_plant")
		return
	}
	binding, configured := loadPCCFeedback(node)
	writeJSON(w, map[string]any{"binding": binding, "configured": configured})
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
		if !h.allows(identity, node.ID) {
			continue
		}
		assetCode, _ := node.Points.Find(PointTypeAssetCode, "")
		description, _ := node.Points.Find(data.PointTypeDescription, "")
		plants = append(plants, map[string]string{"id": node.ID, "asset_code": assetCode.Txt(), "description": description.Txt()})
	}
	writeJSON(w, map[string]any{"plants": plants})
}

// phaseVerdict 处理 GET /plants/{id}/phase-verdict：三相点位都新鲜时给出
// 逐相反送判定；执行器能力未知，因此始终如实说明不支持独立调相。
func (h *Handler) phaseVerdict(w http.ResponseWriter, identity Identity, plantID string) {
	node, ok := h.node(w, identity, plantID)
	if !ok {
		return
	}
	if node.Type != NodeTypePlant {
		writeError(w, http.StatusBadRequest, "requires_plant")
		return
	}
	binding, configured := loadPCCFeedback(node)
	if !configured || len(binding.PhasePointIDs) < 3 {
		writeError(w, http.StatusBadRequest, "phase_points_not_configured")
		return
	}
	now := time.Now()
	maxAge := time.Duration(binding.MaxAgeMS) * time.Millisecond
	if maxAge <= 0 {
		maxAge = 15 * time.Second
	}
	reading := decision.PhaseReading{}
	values := []*float64{&reading.A, &reading.B, &reading.C}
	for index, pointID := range binding.PhasePointIDs[:3] {
		value, ok, reason := readPointSample(h.nc, pointID, now, maxAge)
		if !ok {
			writeJSON(w, map[string]any{"available": false, "reason": reason})
			return
		}
		*values[index] = value
	}
	verdict := decision.EvaluatePhase(reading, binding.LowerLimitKW, false)
	writeJSON(w, map[string]any{
		"available": true, "net_kw": verdict.NetKW,
		"phase_reverse_kw": verdict.PhaseReverseKW,
		"can_per_phase":    verdict.CanPerPhase, "note": verdict.Note,
	})
}
