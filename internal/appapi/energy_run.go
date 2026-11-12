package appapi

import (
	"fmt"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/decision"
)

// RunCapabilityDispatch 执行一次能力级调度。柜内拆解、动态能力约束、人工/
// 互锁闸门和爬坡统一由 writeEnergyUnitPower 执行，避免单柜路径绕过约束。
func (h *Handler) RunCapabilityDispatch(energyUnitID string) ([]decision.DeviceDispatch, error) {
	nodes, err := siotclient.GetNodes(h.nc, "all", energyUnitID, "", false)
	if err != nil || len(nodes) == 0 {
		return nil, fmt.Errorf("energy unit %s not found", energyUnitID)
	}
	rule, ok := loadCapabilityRule(nodes[0])
	if !ok {
		return nil, fmt.Errorf("no capability rule on %s", energyUnitID)
	}
	if rule.State != decision.RulePublished {
		return nil, fmt.Errorf("capability rule on %s is not published", energyUnitID)
	}
	if !AllowsEnergyUnitAutoDispatch(nodes[0]) {
		return nil, fmt.Errorf("energy unit %s is interlocked or manually controlled", energyUnitID)
	}
	values, err := LoadCapabilityValues(h.nc, []string{energyUnitID})
	if err != nil {
		return nil, err
	}
	requestKW := 0.0
	for _, action := range decision.EvaluateCapabilityRule(rule, values) {
		if action.EnergyUnitID == energyUnitID {
			requestKW += action.RequestKW
		}
	}
	dispatches, stage, err := h.writeEnergyUnitPower(energyUnitID, requestKW, dispatchTrace{ID: decision.NewDispatchID(), Source: "schedule"})
	if err != nil {
		return nil, err
	}
	record := decision.BuildRecord(energyUnitID, rule.ID, requestKW, values, LoadCabinetState(nodes[0]), dispatches)
	record.Stage = stage
	if err := appendDispatchRecord(h.nc, energyUnitID, record); err != nil {
		fmt.Printf("easyems: append dispatch record %s: %v\n", energyUnitID, err)
	}
	return dispatches, nil
}

// readCurrentSetpoint 读一个节点的当前设定值（valueSet 点）。无该点或非
// 数值时返回 ok=false，调用方按"无当前值"处理（直接写目标）。
func readCurrentSetpoint(nc *nats.Conn, nodeID string) (float64, bool) {
	nodes, err := siotclient.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return 0, false
	}
	point, found := nodes[0].Points.Find(data.PointTypeValueSet, "")
	if !found {
		return 0, false
	}
	return pointFloat(point), true
}
