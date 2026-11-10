package appapi

import (
	"strconv"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// pointFloat 读一个 point 的数值。float/int 点（NewPointFloat 写入，含
// modbus 实测值）走 ValueFloat；string 点走文本解析。此前只对 string 点
// 有效，float 点一律返回 0——这会读丢真实遥测，必须兼容。
func pointFloat(point data.Point) float64 {
	if point.Numeric() {
		if value, err := point.ValueFloat(); err == nil {
			return value
		}
	}
	value, err := strconv.ParseFloat(point.Txt(), 64)
	if err != nil {
		return 0
	}
	return value
}

// LoadCapabilityValues 从节点树读取 EnergyUnit 的能力/测点值，汇总成
// decision.CapabilityValues 供能力级调度使用。第一版只读 EnergyUnit 节点
// 的静态能力（ratedPowerKW + capability 点）；动态能力（从 Device 点位
// 汇总）待 Profile 能力声明落地后补。
func LoadCapabilityValues(nc *nats.Conn, energyUnitIDs []string) (decision.CapabilityValues, error) {
	values := make(decision.CapabilityValues)
	for _, id := range energyUnitIDs {
		nodes, err := siotclient.GetNodes(nc, "all", id, "", false)
		if err != nil || len(nodes) == 0 {
			continue
		}
		node := nodes[0]
		fields := make(map[decision.CapabilityField]float64)
		// 能源类型 → 方向约束：光伏只放电、充电只充电、储能双向。
		fields[decision.FieldEnergyType] = decision.EnergyStorageBidir
		if point, ok := node.Points.Find(PointTypeEnergyType, ""); ok {
			switch point.Txt() {
			case EnergyTypePV:
				fields[decision.FieldEnergyType] = decision.EnergyPVDischarge
			case EnergyTypeEV:
				fields[decision.FieldEnergyType] = decision.EnergyEVCharge
			}
		}
		if point, ok := node.Points.Find(PointTypeRatedPowerKW, ""); ok {
			fields[decision.FieldRatedPower] = pointFloat(point)
		}
		if point, ok := node.Points.Find(PointTypeCapability, ""); ok {
			capability, err := DecodeCapability(point.Txt())
			if err == nil {
				fields[decision.FieldAvailablePower] = capability.PowerMaxKW
				if capability.ChargeMaxKW > 0 || capability.DischargeMaxKW > 0 {
					fields[decision.FieldAvailableCharge] = capability.ChargeMaxKW
					fields[decision.FieldAvailableDischarge] = capability.DischargeMaxKW
				}
				if capability.Allowed {
					fields[decision.FieldStatus] = 1
				}
			}
		}
		if !AllowsEnergyUnitAutoDispatch(node) {
			fields[decision.FieldStatus] = 0
		}
		// 通用与光伏/充电专属能力字段（通用型能力模型的丰富度）。
		capFields := map[string]decision.CapabilityField{
			PointTypeActualPower:     decision.FieldActualPower,
			PointTypePVAvailableKW:   decision.FieldPVAvailableKW,
			PointTypePVLimitKW:       decision.FieldPVLimitKW,
			PointTypePVForecastKW:    decision.FieldPVForecastKW,
			PointTypeEVMaxLoadKW:     decision.FieldEVMaxLoadKW,
			PointTypeEVCurtailableKW: decision.FieldEVCurtailableKW,
			PointTypeEVOnlineRatio:   decision.FieldEVOnlineRatio,
		}
		for pointType, field := range capFields {
			if point, ok := node.Points.Find(pointType, ""); ok {
				fields[field] = pointFloat(point)
			}
		}
		values[id] = fields
	}
	return values, nil
}
