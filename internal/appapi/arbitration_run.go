package appapi

import (
	"fmt"

	siotclient "github.com/simpleiot/simpleiot/client"

	"fastems/internal/decision"
)

// RunUnitArbitratedDispatch 统一仲裁入口（Q8/OBJ-02/G10）：收集一个
// EnergyUnit 的所有功率请求来源——单柜规则、站级防逆流、需量、削峰填谷——
// 按硬优先级仲裁出唯一目标，再统一写设备。站级来源只产生目标，不再直接
// 写设备后跳过仲裁。没有已发布单柜规则的单元仍可被站级策略控制。
func (h *Handler) RunUnitArbitratedDispatch(energyUnitID string) error {
	nodes, err := siotclient.GetNodes(h.nc, "all", energyUnitID, "", false)
	if err != nil || len(nodes) == 0 {
		return fmt.Errorf("energy unit %s not found", energyUnitID)
	}
	node := nodes[0]
	if node.Type != NodeTypeEnergyUnit {
		return fmt.Errorf("%s is not an energy unit", energyUnitID)
	}
	if !AllowsEnergyUnitAutoDispatch(node) {
		return fmt.Errorf("energy unit %s interlocked or manually controlled", energyUnitID)
	}
	values, err := LoadCapabilityValues(h.nc, []string{energyUnitID})
	if err != nil {
		return err
	}

	goals := []decision.Goal{}
	ruleID := ""

	// 单柜规则请求（计划优先级）。
	if rule, ok := loadCapabilityRule(node); ok && rule.State == decision.RulePublished {
		ruleID = rule.ID
		ruleRequest := 0.0
		for _, action := range decision.EvaluateCapabilityRule(rule, values) {
			if action.EnergyUnitID == energyUnitID {
				ruleRequest += action.RequestKW
			}
		}
		goals = append(goals, decision.Goal{Priority: decision.GoalSchedule, RequestKW: ruleRequest, Source: "schedule", RuleID: rule.ID})
	}

	// 站级目标：防逆流 > 需量 > 削峰填谷，由所属 Plant 计算。
	if plantID := node.Parent; plantID != "" {
		for _, goal := range h.PlantUnitGoals(plantID)[energyUnitID] {
			goals = append(goals, goal)
		}
		// 站级计划：Site 总目标按 Plant → EnergyUnit 分层分配，计划优先级。
		if plantNodes, err := siotclient.GetNodes(h.nc, "all", plantID, "", false); err == nil && len(plantNodes) > 0 {
			if siteID := plantNodes[0].Parent; siteID != "" {
				if siteNodes, err := siotclient.GetNodes(h.nc, "all", siteID, "", false); err == nil && len(siteNodes) > 0 && siteNodes[0].Type == NodeTypeSite {
					if goal, ok := h.SiteUnitGoals(siteID)[energyUnitID]; ok {
						goals = append(goals, goal)
					}
				}
			}
		}
	}

	// 仲裁出唯一目标，统一写设备，并留下可复盘的追溯记录。
	best, ok := decision.Arbitrate(goals)
	if !ok {
		return nil
	}
	trace := dispatchTrace{ID: decision.NewDispatchID(), Source: best.Source}
	dispatches, stage, err := h.writeEnergyUnitPower(energyUnitID, best.RequestKW, trace)
	if err != nil {
		return err
	}
	record := decision.BuildRecord(energyUnitID, ruleID, best.RequestKW, values, LoadCabinetState(node), dispatches)
	record.DispatchID = trace.ID
	record.Source = best.Source
	record.Priority = int(best.Priority)
	record.Stage = stage
	// 目标、下发、缺口、来源与阶段都未变化的 tick 不重复写追溯，避免
	// 稳定运行时把有意义的时间线淹没在每秒重复记录里。
	records := listDispatchRecords(h.nc, energyUnitID)
	if last := len(records) - 1; last >= 0 {
		previous := records[last]
		if previous.RequestKW == record.RequestKW && previous.DeliveredKW == record.DeliveredKW &&
			previous.ShortfallKW == record.ShortfallKW && previous.Source == record.Source &&
			previous.Stage == record.Stage && previous.CabinetState == record.CabinetState {
			return nil
		}
	}
	if err := appendDispatchRecord(h.nc, energyUnitID, record); err != nil {
		fmt.Printf("fastems: append dispatch record %s: %v\n", energyUnitID, err)
	}
	return nil
}
