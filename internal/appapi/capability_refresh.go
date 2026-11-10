package appapi

import (
	"encoding/json"
	"net/http"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// 动态能力刷新（CAP-03/CAP-06）：从 EnergyUnit 配置的 SOC/温度点位读取实测，
// 用 DynamicCharge/DynamicDischarge 合成当前可用能力，写回 capability 点。
// 调度只读 capability，因此 SOC 保护与温度降额会在下一秒自动生效。

const capabilityInputsPoint = "easyemsCapabilityInputs"

// CapabilityInputs 描述动态能力的来源与边界。
type CapabilityInputs struct {
	SOCPointID  string  `json:"soc_point_id"`
	TempPointID string  `json:"temp_point_id"`
	SOCMin      float64 `json:"soc_min"`
	SOCMax      float64 `json:"soc_max"`
	TempDerate  float64 `json:"temp_derate"`
	TempLimit   float64 `json:"temp_limit"`
}

func loadCapabilityInputs(node data.NodeEdge) (CapabilityInputs, bool) {
	point, found := node.Points.Find(capabilityInputsPoint, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return CapabilityInputs{}, false
	}
	var inputs CapabilityInputs
	if json.Unmarshal([]byte(point.Txt()), &inputs) != nil {
		return CapabilityInputs{}, false
	}
	return inputs, true
}

// readPointValue 读取一个节点的 value 点数值。
func readPointValue(nc *nats.Conn, nodeID string) (float64, bool) {
	nodes, err := siotclient.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return 0, false
	}
	point, found := nodes[0].Points.Find(data.PointTypeValue, "")
	if !found {
		return 0, false
	}
	return pointFloat(point), true
}

// RefreshCapabilities 扫描所有配置了动态能力来源的 EnergyUnit 并刷新能力。
func RefreshCapabilities(nc *nats.Conn) {
	units, err := siotclient.GetNodes(nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return
	}
	for _, unit := range units {
		inputs, ok := loadCapabilityInputs(unit)
		if !ok || inputs.SOCPointID == "" {
			continue
		}
		soc, socOK := readPointValue(nc, inputs.SOCPointID)
		temp, _ := readPointValue(nc, inputs.TempPointID)
		if !socOK {
			continue
		}
		rated := 0.0
		if point, found := unit.Points.Find(PointTypeRatedPowerKW, ""); found {
			rated = pointFloat(point)
		}
		limit := decision.CapabilityLimit{
			Rated: rated, Available: rated, SOC: soc,
			SOCMin: inputs.SOCMin, SOCMax: inputs.SOCMax,
			Temp: temp, TempDerate: inputs.TempDerate, TempLimit: inputs.TempLimit,
		}
		discharge, dischargeOK := decision.DynamicDischarge(limit)
		charge, chargeOK := decision.DynamicCharge(limit)
		capability := Capability{Kind: "discharge", Direction: "bidir", Allowed: dischargeOK || chargeOK, PowerMaxKW: 0}
		switch {
		case dischargeOK && chargeOK:
			if discharge > charge {
				capability.PowerMaxKW = discharge
			} else {
				capability.PowerMaxKW = charge
			}
		case dischargeOK:
			capability.PowerMaxKW = discharge
			capability.Direction = "discharge"
		case chargeOK:
			capability.PowerMaxKW = charge
			capability.Direction = "charge"
		}
		if !capability.Allowed {
			capability.LimitReason = "SOC 越界或温度超限"
		}
		text, err := EncodeCapability(capability)
		if err != nil {
			continue
		}
		point := data.NewPointString(PointTypeCapability, "", text)
		point.Origin = "capability-refresh"
		_ = siotclient.SendNodePoint(nc, unit.ID, point, true)
	}
}

// capabilityInputs 读写动态能力来源配置。
func (h *Handler) capabilityInputs(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
	nodes, err := siotclient.GetNodes(h.nc, "all", energyUnitID, "", false)
	if err != nil || len(nodes) == 0 {
		writeError(w, http.StatusNotFound, "energy_unit_not_found")
		return
	}
	if nodes[0].Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	switch r.Method {
	case http.MethodGet:
		inputs, ok := loadCapabilityInputs(nodes[0])
		writeJSON(w, map[string]any{"inputs": inputs, "configured": ok})
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var inputs CapabilityInputs
		if !decodeJSON(w, r, &inputs) {
			return
		}
		if inputs.SOCPointID == "" {
			writeError(w, http.StatusBadRequest, "soc_point_required")
			return
		}
		encoded, err := json.Marshal(inputs)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_inputs")
			return
		}
		point := data.NewPointString(capabilityInputsPoint, "", string(encoded))
		point.Origin = identity.UserID
		if err := siotclient.SendNodePoint(h.nc, energyUnitID, point, true); err != nil {
			writeError(w, http.StatusBadGateway, "save_failed")
			return
		}
		writeJSON(w, map[string]bool{"saved": true})
	}
}
