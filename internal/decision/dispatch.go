package decision

// DispatchCapability 执行一次能力级调度端到端：求值能力级规则得到每个
// EnergyUnit 的功率请求，再按柜内 Device 能力拆成每台 PCS 的设定值。
// 纯函数，无副作用——这是能力级调度替换点位级调度后的完整执行语义。
//
// 数据流：
//   CapabilityRule 动作(EnergyUnit 功率请求)
//     → DispatchCabinet(每个 EnergyUnit 的请求，柜内 Device)
//     → 汇总 Device 功率设定
func DispatchCapability(rule CapabilityRule, values CapabilityValues, cabinets map[string][]DeviceCapability) []DeviceDispatch {
	actions := EvaluateCapabilityRule(rule, values)
	var dispatches []DeviceDispatch
	for _, action := range actions {
		devices, err := DispatchCabinet(action.RequestKW, cabinets[action.EnergyUnitID])
		if err != nil {
			continue
		}
		dispatches = append(dispatches, devices...)
	}
	return dispatches
}
