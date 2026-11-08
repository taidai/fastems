package appapi

import (
	"encoding/json"
	"net/http"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
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
