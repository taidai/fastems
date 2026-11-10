package appapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// 电站策略（②级）：需量控制与分时电价削峰填谷。与防逆流一起构成站级
// 硬优先级：防逆流 > 需量 > 削峰填谷。站级策略生效时，单柜规则让位。

const plantStrategyPoint = "easyemsPlantStrategy"

// PlantStrategy 是电站级的分时策略配置。
type PlantStrategy struct {
	PeakValleyPowerKW float64                 `json:"peak_valley_power_kw"` // 谷充/峰放功率幅值
	Periods           []decision.TariffPeriod `json:"periods"`
}

func loadPlantStrategy(node data.NodeEdge) (PlantStrategy, bool) {
	point, found := node.Points.Find(plantStrategyPoint, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return PlantStrategy{}, false
	}
	var strategy PlantStrategy
	if json.Unmarshal([]byte(point.Txt()), &strategy) != nil {
		return PlantStrategy{}, false
	}
	return strategy, true
}

func savePlantStrategy(nc *nats.Conn, plantID string, strategy PlantStrategy) error {
	encoded, err := json.Marshal(strategy)
	if err != nil {
		return err
	}
	point := data.NewPointString(plantStrategyPoint, "", string(encoded))
	point.Origin = "easyems"
	return siotclient.SendNodePoint(nc, plantID, point, true)
}

// PlantStrategyActive 按硬优先级执行站级策略：防逆流 > 需量 > 削峰填谷。
// 返回是否已接管该电站的功率分配；接管时单柜规则本轮不执行。
func (h *Handler) RunPlantStrategy(plantID string) (bool, error) {
	if allocations, err := h.RunPlantAntiReverse(plantID); err != nil {
		// 无 PCC 绑定或数据过期：继续尝试分时策略
	} else if len(allocations) > 0 {
		if err := h.applyAllocations(allocations); err != nil {
			return true, err
		}
		return true, nil
	}

	if allocations, err := h.RunPlantPeakShaving(plantID); err != nil {
		// 同上
	} else if len(allocations) > 0 {
		if err := h.applyAllocations(allocations); err != nil {
			return true, err
		}
		return true, nil
	}

	nodes, err := siotclient.GetNodes(h.nc, "all", plantID, "", false)
	if err != nil || len(nodes) == 0 {
		return false, nil
	}
	strategy, ok := loadPlantStrategy(nodes[0])
	if !ok || strategy.PeakValleyPowerKW <= 0 || len(strategy.Periods) == 0 {
		return false, nil
	}
	now := time.Now()
	minute := now.Hour()*60 + now.Minute()
	request := decision.PeakValleyAction(strategy.Periods, minute, strategy.PeakValleyPowerKW)
	if request == 0 {
		return false, nil
	}
	units, err := siotclient.GetNodes(h.nc, plantID, "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return false, err
	}
	members := make([]decision.EnergyMember, 0, len(units))
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		members = append(members, decision.EnergyMember{EnergyUnitID: unit.ID})
		ids = append(ids, unit.ID)
	}
	caps, err := LoadCapabilityValues(h.nc, ids)
	if err != nil {
		return false, err
	}
	allocations, err := decision.AllocateEnergy(request, members, caps)
	if err != nil {
		return false, err
	}
	if err := h.applyAllocations(allocations); err != nil {
		return true, err
	}
	return true, nil
}

// applyAllocations 把站级分配写进各能源设备的柜内设备。
func (h *Handler) applyAllocations(allocations []decision.EnergyAllocation) error {
	for _, allocation := range allocations {
		if allocation.Excluded {
			continue
		}
		if _, err := h.writeEnergyUnitPower(allocation.EnergyUnitID, allocation.SetpointKW); err != nil {
			return err
		}
	}
	return nil
}

// plantStrategy 读写电站分时策略（削峰填谷）。
func (h *Handler) plantStrategy(w http.ResponseWriter, r *http.Request, identity Identity, plantID string) {
	node, ok := h.node(w, identity, plantID)
	if !ok {
		return
	}
	if node.Type != NodeTypePlant {
		writeError(w, http.StatusBadRequest, "requires_plant")
		return
	}
	switch r.Method {
	case http.MethodGet:
		strategy, configured := loadPlantStrategy(node)
		writeJSON(w, map[string]any{"strategy": strategy, "configured": configured})
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var strategy PlantStrategy
		if !decodeJSON(w, r, &strategy) {
			return
		}
		if err := savePlantStrategy(h.nc, plantID, strategy); err != nil {
			writeError(w, http.StatusBadGateway, "save_failed")
			return
		}
		writeJSON(w, map[string]bool{"saved": true})
	}
}
