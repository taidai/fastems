package appapi

import (
	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// EnergyUnit 运行状态的读写与调度闸门（CTL-01）。状态存 cabinetState 点，
// 缺省视为 running（兼容未建模的存量单元）；显式故障/检修才禁止调度。

// LoadCabinetState 读一个 EnergyUnit 的运行状态。无点位时返回 running
// （未显式建模状态的单元默认允许调度，与既有行为一致）。
func LoadCabinetState(node data.NodeEdge) decision.CabinetState {
	point, found := node.Points.Find(PointTypeCabinetState, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return decision.CabinetRunning
	}
	return decision.CabinetState(point.Txt())
}

const manualModePoint = "easyemsManual"

// AllowsEnergyUnitAutoDispatch 统一判断柜体状态与人工接管。所有自动来源都
// 必须经过这个闸门，不能只在单柜调度循环检查。
func AllowsEnergyUnitAutoDispatch(node data.NodeEdge) bool {
	if !decision.AllowsAutoDispatch(LoadCabinetState(node)) {
		return false
	}
	if point, found := node.Points.Find(manualModePoint, ""); found && point.Tombstone%2 == 0 && pointFloat(point) != 0 {
		return false
	}
	return true
}

// SetCabinetState 写 EnergyUnit 的运行状态。
func SetCabinetState(nc *nats.Conn, energyUnitID string, state decision.CabinetState) error {
	point := data.NewPointString(PointTypeCabinetState, "", string(state))
	point.Origin = "easyems"
	return siotclient.SendNodePoint(nc, energyUnitID, point, true)
}

// ApplyCabinetEvent 对一个 EnergyUnit 施加状态事件，按转移表推进并持久化。
// 非法转移返回 ok=false 且不写。
func ApplyCabinetEvent(nc *nats.Conn, node data.NodeEdge, event decision.CabinetEvent) (decision.CabinetState, bool, error) {
	current := LoadCabinetState(node)
	next, ok := decision.Transition(current, event)
	if !ok {
		return current, false, nil
	}
	if err := SetCabinetState(nc, node.ID, next); err != nil {
		return current, false, err
	}
	return next, true, nil
}
