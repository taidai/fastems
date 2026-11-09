package appapi

import (
	"fmt"

	siotclient "github.com/simpleiot/simpleiot/client"

	"easyems/internal/decision"
)

// RunUnitArbitratedDispatch 统一仲裁入口（Q8/OBJ-02/G10）：收集一个
// EnergyUnit 的所有功率请求来源——站级防逆流、站级需量、单柜规则——按
// 硬优先级（安全>防逆流>需量>计划>电价）仲裁出唯一目标，再统一写设备。
// 解决此前"单柜规则放电与站级防逆流充电同时写同一设备"的冲突。
func (h *Handler) RunUnitArbitratedDispatch(energyUnitID string) error {
	nodes, err := siotclient.GetNodes(h.nc, "all", energyUnitID, "", false)
	if err != nil || len(nodes) == 0 {
		return fmt.Errorf("energy unit %s not found", energyUnitID)
	}
	node := nodes[0]
	rule, ok := loadCapabilityRule(node)
	if !ok || rule.State != decision.RulePublished {
		return fmt.Errorf("no published rule on %s", energyUnitID)
	}
	if !decision.AllowsAutoDispatch(LoadCabinetState(node)) {
		return fmt.Errorf("energy unit %s interlocked", energyUnitID)
	}
	values, err := LoadCapabilityValues(h.nc, []string{energyUnitID})
	if err != nil {
		return err
	}

	goals := []decision.Goal{}

	// 单柜规则请求（计划优先级）。
	ruleRequest := 0.0
	for _, action := range decision.EvaluateCapabilityRule(rule, values) {
		if action.EnergyUnitID == energyUnitID {
			ruleRequest += action.RequestKW
		}
	}
	goals = append(goals, decision.Goal{Priority: decision.GoalSchedule, RequestKW: ruleRequest})

	// 站级策略（若所属 Plant 有 PCC 绑定）：防逆流 > 需量 > 单柜规则。
	plantID := node.Parent
	if plantID != "" {
		if antiAllocs, err := h.RunPlantAntiReverse(plantID); err == nil {
			for _, a := range antiAllocs {
				if a.EnergyUnitID == energyUnitID && !a.Excluded {
					goals = append(goals, decision.Goal{Priority: decision.GoalAntiReverse, RequestKW: a.SetpointKW})
				}
			}
		}
		if peakAllocs, err := h.RunPlantPeakShaving(plantID); err == nil {
			for _, a := range peakAllocs {
				if a.EnergyUnitID == energyUnitID && !a.Excluded {
					goals = append(goals, decision.Goal{Priority: decision.GoalPeakShaving, RequestKW: a.SetpointKW})
				}
			}
		}
	}

	// 仲裁出唯一目标，统一写设备。
	best, ok := decision.Arbitrate(goals)
	if !ok {
		return nil
	}
	return h.writeEnergyUnitPower(energyUnitID, best.RequestKW)
}
