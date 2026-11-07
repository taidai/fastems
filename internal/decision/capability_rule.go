package decision

// CapabilityRule 是一条能力级规则：条件引用 EnergyUnit 的能力/测点，
// 动作是对 EnergyUnit 的功率请求。所有条件 AND；命中返回动作，未命中
// 返回 nil。这是决策表从点位级切换到能力级的语义核心。
type CapabilityRule struct {
	ID         string                `json:"id"`
	Conditions []CapabilityCondition `json:"conditions"`
	Actions    []CapabilityAction    `json:"actions"`
}

// EvaluateCapabilityRule 用能力值求值能力级规则，返回命中的动作。
// 任一条件不满足即不命中。
func EvaluateCapabilityRule(rule CapabilityRule, values CapabilityValues) []CapabilityAction {
	for _, condition := range rule.Conditions {
		if !EvaluateCapability(condition, values) {
			return nil
		}
	}
	return rule.Actions
}
