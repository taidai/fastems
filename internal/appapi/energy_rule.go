package appapi

import (
	"encoding/json"
	"net/http"
	"time"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/decision"
)

// 能力级规则与分配器存到 EnergyUnit 节点上的 point。这是破坏性切换里
// 能力级模型接线的第一步：把 decision 的纯函数接到可持久化的配置。

const (
	capabilityRulePoint      = "easyemsCapabilityRule"
	capabilityAllocatorPoint = "easyemsEnergyAllocator"
)

// loadCapabilityRule 读取一个 EnergyUnit 上的能力级规则。
func loadCapabilityRule(node data.NodeEdge) (decision.CapabilityRule, bool) {
	point, found := node.Points.Find(capabilityRulePoint, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return decision.CapabilityRule{}, false
	}
	var rule decision.CapabilityRule
	if json.Unmarshal([]byte(point.Txt()), &rule) != nil {
		return decision.CapabilityRule{}, false
	}
	return rule, true
}

// loadEnergyAllocator 读取一个 EnergyUnit 上的能力级分配器成员。
func loadEnergyAllocator(node data.NodeEdge) ([]decision.EnergyMember, bool) {
	point, found := node.Points.Find(capabilityAllocatorPoint, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return nil, false
	}
	var members []decision.EnergyMember
	if json.Unmarshal([]byte(point.Txt()), &members) != nil {
		return nil, false
	}
	return members, true
}

// listEnergyUnits 列出所有 EnergyUnit 节点（供前端选择进入能力级规则编辑器）。
func (h *Handler) listEnergyUnits(w http.ResponseWriter, identity Identity) {
	nodes, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list_failed")
		return
	}
	units := make([]map[string]string, 0, len(nodes))
	for _, node := range nodes {
		if !h.allows(identity, node.ID) {
			continue
		}
		assetCode := ""
		if point, ok := node.Points.Find(PointTypeAssetCode, ""); ok {
			assetCode = point.Txt()
		}
		description := ""
		if point, ok := node.Points.Find(data.PointTypeDescription, ""); ok {
			description = point.Txt()
		}
		units = append(units, map[string]string{
			"id":          node.ID,
			"asset_code":  assetCode,
			"description": description,
		})
	}
	writeJSON(w, map[string]any{"units": units})
}

// energyRule 读写一个 EnergyUnit 上的能力级规则。
func (h *Handler) energyRule(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rule, _ := loadCapabilityRule(node)
		writeJSON(w, rule)
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var rule decision.CapabilityRule
		if !decodeJSON(w, r, &rule) {
			return
		}
		h.saveCapabilityRule(w, identity, energyUnitID, rule)
	}
}

// energyAllocator 读写一个 EnergyUnit 上的能力级分配器成员。
func (h *Handler) energyAllocator(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	switch r.Method {
	case http.MethodGet:
		members, _ := loadEnergyAllocator(node)
		writeJSON(w, map[string]any{"members": members})
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input struct {
			Members []decision.EnergyMember `json:"members"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		h.saveEnergyAllocator(w, identity, energyUnitID, input.Members)
	}
}

// publishCapabilityRule 发布一个 EnergyUnit 的草稿规则：校验后置为
// published 并记录发布时刻。发布是显式动作，草稿保存不改变运行行为。
func (h *Handler) publishCapabilityRule(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	rule, ok := loadCapabilityRule(node)
	if !ok {
		writeError(w, http.StatusBadRequest, "no_rule")
		return
	}
	rule.State = decision.RulePublished
	rule.PublishedAt = time.Now().UTC().Format(time.RFC3339)
	h.saveCapabilityRule(w, identity, energyUnitID, rule)
}

// simulateCapabilityRule 求值一个能力级规则（草稿或已发布），返回命中的
// 动作。纯函数求值，绝不写设备——这是"模拟测试语义独立"的边界。
func (h *Handler) simulateCapabilityRule(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	var input struct {
		Rule   decision.CapabilityRule   `json:"rule"`
		Values decision.CapabilityValues `json:"values"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	// 用节点真实能力补足模拟输入（如额定功率），前端只覆盖要模拟的字段。
	actualValues, err := LoadCapabilityValues(h.nc, []string{energyUnitID})
	if err != nil {
		writeError(w, http.StatusBadGateway, "capability_load_failed")
		return
	}
	if input.Values == nil {
		input.Values = make(decision.CapabilityValues)
	}
	for id, fields := range actualValues {
		if input.Values[id] == nil {
			input.Values[id] = make(map[decision.CapabilityField]float64)
		}
		for field, value := range fields {
			if _, supplied := input.Values[id][field]; !supplied {
				input.Values[id][field] = value
			}
		}
	}
	actions := decision.EvaluateCapabilityRule(input.Rule, input.Values)
	writeJSON(w, map[string]any{"actions": actions})
}

// saveCapabilityRule 把能力级规则写到 EnergyUnit 节点。
func (h *Handler) saveCapabilityRule(w http.ResponseWriter, identity Identity, energyUnitID string, rule decision.CapabilityRule) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	if rule.ID == "" {
		writeError(w, http.StatusBadRequest, "rule_id_required")
		return
	}
	if rule.State == "" {
		rule.State = decision.RuleDraft
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_rule")
		return
	}
	point := data.NewPointString(capabilityRulePoint, "", string(encoded))
	point.Origin = identity.UserID
	if err := siotclient.SendNodePoint(h.nc, energyUnitID, point, true); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

// saveEnergyAllocator 把能力级分配器成员写到 EnergyUnit 节点。
func (h *Handler) saveEnergyAllocator(w http.ResponseWriter, identity Identity, energyUnitID string, members []decision.EnergyMember) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	encoded, err := json.Marshal(members)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_members")
		return
	}
	point := data.NewPointString(capabilityAllocatorPoint, "", string(encoded))
	point.Origin = identity.UserID
	if err := siotclient.SendNodePoint(h.nc, energyUnitID, point, true); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}
