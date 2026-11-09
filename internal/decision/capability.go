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
	FieldEnergyType     CapabilityField = "energy_type"     // 能源类型（方向约束）

	// 通用能力：当前实测功率（发电/负载，按类型语义）。
	FieldActualPower CapabilityField = "actual_power"
	// 光伏专属：估算可用出力、限发设定、预测出力。
	FieldPVAvailableKW CapabilityField = "pv_available_kw"
	FieldPVLimitKW     CapabilityField = "pv_limit_kw"
	FieldPVForecastKW  CapabilityField = "pv_forecast_kw"
	// 充电桩专属：最大负载、可削减功率、在线率。
	FieldEVMaxLoadKW      CapabilityField = "ev_max_load_kw"
	FieldEVCurtailableKW  CapabilityField = "ev_curtailable_kw"
	FieldEVOnlineRatio    CapabilityField = "ev_online_ratio"
)

// 能源类型编码（FieldEnergyType 的取值）：决定节点的可调方向。
// 同一套 EnergyUnit+Capability+分配，靠方向约束表达不同资源，而非写死三套逻辑。
const (
	EnergyStorageBidir = 0 // 储能：可充可放
	EnergyPVDischarge  = 1 // 光伏：只能放电（发电）
	EnergyEVCharge     = 2 // 充电桩：只能充电（耗电）
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
