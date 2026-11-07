package decision

// 能力级调度：调度对象从「具体点位」切换到「EnergyUnit 的能力/测点」。
// 规则条件引用 EnergyUnit 的能力字段，动作是对 EnergyUnit 的功率请求，
// 由 Cabinet Controller 按 Capability 拆到具体设备。这是领域模型重构
// 阶段 2 的核心语义（Q11=A）。

// CapabilityField 是 EnergyUnit 参与调度的能力/测点字段。值由运行时从
// EnergyUnit 节点（capability 点 + Device 点位汇总）读取。
type CapabilityField string

const (
	FieldSOC            CapabilityField = "soc"             // SOC %
	FieldAvailablePower CapabilityField = "available_power" // 可用功率 kW（动态能力）
	FieldRatedPower     CapabilityField = "rated_power"     // 额定功率 kW
	FieldStatus         CapabilityField = "status"          // 状态
)

// CapabilityRef 引用一个 EnergyUnit 的测点/能力字段。
type CapabilityRef struct {
	EnergyUnitID string         `json:"energy_unit_id"`
	Field        CapabilityField `json:"field"`
}

// CapabilityCondition 能力级条件：引用 EnergyUnit 测点/能力而非具体点位。
type CapabilityCondition struct {
	Ref      CapabilityRef `json:"ref"`
	Operator string        `json:"operator"`
	Value    float64       `json:"value"`
}

// CapabilityAction 能力级动作：对 EnergyUnit 请求功率。正值充电、负值放电，
// 方向语义由上层按现场约定；实际执行由 Cabinet Controller 拆到设备。
type CapabilityAction struct {
	EnergyUnitID string  `json:"energy_unit_id"`
	RequestKW    float64 `json:"request_kw"`
}

// CapabilityValues 是运行时读到的 EnergyUnit 能力/测点值。
// 外层 key 是 EnergyUnit 节点 ID，内层 key 是 CapabilityField。
type CapabilityValues map[string]map[CapabilityField]float64

// EvaluateCapability 用能力值求值一个能力级条件。缺失的值永不匹配。
func EvaluateCapability(condition CapabilityCondition, values CapabilityValues) bool {
	fields, ok := values[condition.Ref.EnergyUnitID]
	if !ok {
		return false
	}
	value, ok := fields[condition.Ref.Field]
	if !ok {
		return false
	}
	return compare(value, condition.Operator, condition.Value)
}
