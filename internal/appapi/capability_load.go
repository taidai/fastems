package appapi

import (
	"strconv"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// pointFloat 读一个 point 的数值，兼容 string 与 float point。
func pointFloat(point data.Point) float64 {
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
		if point, ok := node.Points.Find(PointTypeRatedPowerKW, ""); ok {
			fields[decision.FieldRatedPower] = pointFloat(point)
		}
		if point, ok := node.Points.Find(PointTypeCapability, ""); ok {
			capability, err := DecodeCapability(point.Txt())
			if err == nil {
				fields[decision.FieldAvailablePower] = capability.PowerMaxKW
				if capability.Allowed {
					fields[decision.FieldStatus] = 1
				}
			}
		}
		values[id] = fields
	}
	return values, nil
}
