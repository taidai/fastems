package appapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/decision"
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

// PlantUnitGoals 只计算站级策略给各 EnergyUnit 的目标，不写设备。
// 硬优先级：防逆流 > 需量 > 削峰填谷。返回按 unitID 索引的目标列表，
// 供统一仲裁入口合并后一次性写设备。
func (h *Handler) PlantUnitGoals(plantID string) map[string][]decision.Goal {
	goals := map[string][]decision.Goal{}
	now := time.Now()

	if allocations, err := h.RunPlantAntiReverse(plantID); err == nil {
		for _, allocation := range allocations {
			if allocation.Excluded {
				continue
			}
			goals[allocation.EnergyUnitID] = append(goals[allocation.EnergyUnitID],
				decision.Goal{Priority: decision.GoalAntiReverse, RequestKW: allocation.SetpointKW, Source: "anti_reverse"})
		}
	}
	if allocations, err := h.RunPlantPeakShaving(plantID); err == nil {
		for _, allocation := range allocations {
			if allocation.Excluded {
				continue
			}
			goals[allocation.EnergyUnitID] = append(goals[allocation.EnergyUnitID],
				decision.Goal{Priority: decision.GoalPeakShaving, RequestKW: allocation.SetpointKW, Source: "demand"})
		}
	}

	nodes, err := siotclient.GetNodes(h.nc, "all", plantID, "", false)
	if err != nil || len(nodes) == 0 {
		return goals
	}
	strategy, ok := loadPlantStrategy(nodes[0])
	if !ok || strategy.PeakValleyPowerKW <= 0 || len(strategy.Periods) == 0 {
		return goals
	}
	minute := now.Hour()*60 + now.Minute()
	request := decision.PeakValleyAction(strategy.Periods, minute, strategy.PeakValleyPowerKW)
	if request == 0 {
		return goals
	}
	units, err := siotclient.GetNodes(h.nc, plantID, "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return goals
	}
	members := make([]decision.EnergyMember, 0, len(units))
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		members = append(members, decision.EnergyMember{EnergyUnitID: unit.ID})
		ids = append(ids, unit.ID)
	}
	caps, err := LoadCapabilityValues(h.nc, ids)
	if err != nil {
		return goals
	}
	allocations, err := decision.AllocateEnergy(request, members, caps)
	if err != nil {
		return goals
	}
	for _, allocation := range allocations {
		if allocation.Excluded {
			continue
		}
		goals[allocation.EnergyUnitID] = append(goals[allocation.EnergyUnitID],
			decision.Goal{Priority: decision.GoalPrice, RequestKW: allocation.SetpointKW, Source: "peak_valley"})
	}
	return goals
}

// RunPlantStrategy 让站内每个受控单元走一次统一仲裁入口（人工触发/测试用）。
// 它不再直接写设备；实际写入由 RunUnitArbitratedDispatch 统一完成。
func (h *Handler) RunPlantStrategy(plantID string) (bool, error) {
	plantGoals := h.PlantUnitGoals(plantID)
	if len(plantGoals) == 0 {
		return false, nil
	}
	var firstErr error
	for unitID := range plantGoals {
		if err := h.RunUnitArbitratedDispatch(unitID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return true, firstErr
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
