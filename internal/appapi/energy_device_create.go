package appapi

import (
	"net/http"
	"strconv"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// 能源节点关联设备：给 EnergyUnit 挂一台 Device（如 PCS），并映射其功率
// 设定点位（powerSetpoint → modbusIo 节点）。挂上后，能力级调度才能把
// 功率请求拆解到真实设备、delivered>0——这是"能建"到"真正可控"的一环。

// createEnergyDevice 处理 POST /energy-units/{id}/devices。
func (h *Handler) createEnergyDevice(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	var input struct {
		AssetCode       string  `json:"asset_code"`
		Description     string  `json:"description"`
		RatedPowerKW    float64 `json:"rated_power_kw"`
		PowerSetpointID string  `json:"power_setpoint_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Description == "" {
		writeError(w, http.StatusBadRequest, "description_required")
		return
	}
	if input.RatedPowerKW <= 0 {
		writeError(w, http.StatusBadRequest, "rated_power_required")
		return
	}
	if input.PowerSetpointID != "" {
		if _, ok := h.node(w, identity, input.PowerSetpointID); !ok {
			return
		}
	}
	assetCode, err := resolveAssetCode("", input.AssetCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_asset_code")
		return
	}
	id, err := CreateDomainNode(h.nc, energyUnitID, data.NodeTypeDevice, map[string]string{
		PointTypeAssetCode:     assetCode,
		PointTypeRatedPowerKW:  strconv.FormatFloat(input.RatedPowerKW, 'f', -1, 64),
		PointTypePowerSetpoint: input.PowerSetpointID,
		"description":          input.Description,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "create_failed")
		return
	}
	writeJSON(w, map[string]string{"id": id, "asset_code": assetCode})
}

// listEnergyDevices 列出一个 EnergyUnit 下的设备（含功率点位映射）。
func (h *Handler) listEnergyDevices(w http.ResponseWriter, identity Identity, energyUnitID string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	devices, err := siotclient.GetNodes(h.nc, energyUnitID, "all", data.NodeTypeDevice, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list_failed")
		return
	}
	out := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		assetCode, _ := device.Points.Find(PointTypeAssetCode, "")
		description, _ := device.Points.Find(data.PointTypeDescription, "")
		rated, _ := device.Points.Find(PointTypeRatedPowerKW, "")
		setpoint, _ := device.Points.Find(PointTypePowerSetpoint, "")
		out = append(out, map[string]any{
			"id": device.ID, "asset_code": assetCode.Txt(), "description": description.Txt(),
			"rated_power_kw": pointFloat(rated), "power_setpoint_id": setpoint.Txt(),
		})
	}
	writeJSON(w, map[string]any{"devices": out})
}
