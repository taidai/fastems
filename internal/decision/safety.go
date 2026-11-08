package decision

// 安全兜底三道闸（Q4/Q5/Q9/Q10）：人工优先、超时回退、安全互锁。它们是
// Command Engine / Cabinet Controller 决定"如何安全执行"的纯函数语义，
// 保证任何故障、越限或未确认状态下都不会带着错误设定继续跑。

// InterlockState 是安全互锁（Q9）的输入状态。
type InterlockState struct {
	Fault         bool // 设备故障
	OverLimit     bool // 功率/SOC/温度越限
	ManualRelease bool // 人工确认解除（仅在人工确认后为真）
}

// Interlocked 判断是否应硬停机。故障或越限即互锁；只有人工确认解除后才
// 允许恢复，避免互锁状态被自动清除造成二次冲击。
func Interlocked(s InterlockState) bool {
	if s.ManualRelease {
		return false
	}
	return s.Fault || s.OverLimit
}

// CommandState 是回读确认（Q4）与超时回退（Q10）的输入状态。
type CommandState struct {
	Pending        bool    // 有待确认的指令
	TimedOut       bool    // 回读超时（结果未知）
	HasConfirmed   bool    // 存在上一个已确认值
	LastConfirmed  float64 // 上一个已确认值 kW
}

// Rollback 超时回退：回读超时且结果未知时，回退到上一个已确认值；无确认
// 值则回 0。这是 Q10=A 的安全值语义。
func Rollback(s CommandState) float64 {
	if s.HasConfirmed {
		return s.LastConfirmed
	}
	return 0
}

// ControlMode 是人工指令接管状态（Q5）。
type ControlMode int

const (
	ControlAuto   ControlMode = 0 // 自动调度
	ControlManual ControlMode = 1 // 人工接管，停止自动
)

// ManualOverrides 判断人工是否应覆盖自动调度：人工模式下自动停止，直到
// 显式恢复为自动。返回人工模式下的设定值（若人工未给值则回退安全值）。
func ManualOverrides(mode ControlMode, manualSetpoint float64, hasManual bool, fallback float64) (float64, bool) {
	if mode != ControlManual {
		return 0, false
	}
	if hasManual {
		return manualSetpoint, true
	}
	return fallback, true
}
