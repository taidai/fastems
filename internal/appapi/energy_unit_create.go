package appapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// 用户自建能源节点（EnergyUnit）。此前只有 seed 能预置演示站点，接入向导
// 是点位级的、建不出 EnergyUnit——这是"能用"与"演示"的分水岭。这里提供
// 创建入口：asset_code 自动生成或人工指定一次，自动归属默认 Site→Plant。

// ensureDefaultPlant 返回用于挂载新能源节点的 Plant：优先复用第一个 Site
// 的第一个 Plant，没有则创建默认 Site→Plant（首版单站单柜）。
func ensureDefaultPlant(nc *nats.Conn) (string, error) {
	sites, err := siotclient.GetNodes(nc, "all", "all", NodeTypeSite, false)
	if err != nil {
		return "", err
	}
	var siteID string
	if len(sites) > 0 {
		siteID = sites[0].ID
	} else {
		root, err := siotclient.GetRootNode(nc)
		if err != nil {
			return "", err
		}
		siteID, err = CreateDomainNode(nc, root.ID, NodeTypeSite, map[string]string{
			PointTypeAssetCode: "SITE01", "description": "默认站点",
		})
		if err != nil {
			return "", err
		}
	}
	plants, err := siotclient.GetNodes(nc, siteID, "all", NodeTypePlant, false)
	if err != nil {
		return "", err
	}
	if len(plants) > 0 {
		return plants[0].ID, nil
	}
	return CreateDomainNode(nc, siteID, NodeTypePlant, map[string]string{
		PointTypeAssetCode: "PLANT01", "description": "默认电站",
	})
}

// createEnergyUnit 处理 POST /energy-units。
func (h *Handler) createEnergyUnit(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		AssetCode    string  `json:"asset_code"`
		Description  string  `json:"description"`
		RatedPowerKW float64 `json:"rated_power_kw"`
		EnergyType   string  `json:"energy_type"`
		PlantID      string  `json:"plant_id"`
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
	energyType := input.EnergyType
	if energyType == "" {
		energyType = EnergyTypeStorage
	}
	// asset_code：人工指定一次，否则自动生成（Q4=C）。
	assetCode, err := resolveAssetCode("", input.AssetCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_asset_code")
		return
	}
	parent := input.PlantID
	if parent == "" {
		parent, err = h.visibleDefaultPlant(identity)
		if errors.Is(err, errNoVisibleParent) {
			writeError(w, http.StatusForbidden, "no_access")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadGateway, "plant_resolve_failed")
			return
		}
	} else {
		node, ok := h.node(w, identity, parent)
		if !ok {
			return
		}
		if node.Type != NodeTypePlant {
			writeError(w, http.StatusBadRequest, "requires_plant")
			return
		}
	}
	id, err := CreateDomainNode(h.nc, parent, NodeTypeEnergyUnit, map[string]string{
		PointTypeAssetCode:    assetCode,
		PointTypeEnergyType:   energyType,
		PointTypeRatedPowerKW: strconv.FormatFloat(input.RatedPowerKW, 'f', -1, 64),
		"description":         input.Description,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "create_failed")
		return
	}
	// 初始能力：额定即当前可用上限（动态能力由运行时 SOC/温度修正）。
	capabilityText, err := EncodeCapability(Capability{Kind: "discharge", Direction: "bidir", PowerMaxKW: input.RatedPowerKW, Allowed: true})
	if err == nil {
		point := data.NewPointString(PointTypeCapability, "", capabilityText)
		point.Origin = "easyems"
		_ = siotclient.SendNodePoint(h.nc, id, point, true)
	}
	writeJSON(w, map[string]string{"id": id, "asset_code": assetCode, "plant_id": parent})
}
