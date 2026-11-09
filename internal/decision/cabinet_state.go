package decision

// 柜体运行状态机（CTL-01）：EnergyUnit 的启动/待机/运行/停机/故障/检修
// 状态转移、守卫与"谁确认状态已改变"。互锁（Q9）挂在状态机上——故障/
// 检修态禁止自动调度。命令状态机（command.State）管单条命令，本状态机
// 管柜体整体运行，两者不同。

// CabinetState 是柜体运行状态。
type CabinetState string

const (
	CabinetStandby     CabinetState = "standby"     // 待机（已上电，未运行）
	CabinetStarting    CabinetState = "starting"    // 启动中（待回读确认）
	CabinetRunning     CabinetState = "running"     // 运行（可调度）
	CabinetStopped     CabinetState = "stopped"     // 停机
	CabinetFault       CabinetState = "fault"       // 故障（硬停，禁止自动）
	CabinetMaintenance CabinetState = "maintenance" // 检修（人工接管，禁止自动）
)

// CabinetEvent 是驱动状态转移的事件。
type CabinetEvent string

const (
	EvStart           CabinetEvent = "start"            // 请求启动
	EvStartConfirmed  CabinetEvent = "start_confirmed"  // 启动回读确认（谁确认：命令回读）
	EvStop            CabinetEvent = "stop"             // 请求停机
	EvStopConfirmed   CabinetEvent = "stop_confirmed"   // 停机回读确认
	EvFault           CabinetEvent = "fault"            // 故障/越限（硬停）
	EvFaultCleared    CabinetEvent = "fault_cleared"    // 人工确认解除故障
	EvEnterMaintenance CabinetEvent = "enter_maintenance" // 进入检修
	EvExitMaintenance CabinetEvent = "exit_maintenance"   // 退出检修
)

// Transition 按转移表推进状态。返回新状态与是否允许该转移；非法转移
// 返回原状态 + false（不静默改变状态）。
func Transition(state CabinetState, event CabinetEvent) (CabinetState, bool) {
	switch event {
	case EvFault:
		// 任何非检修态都可故障硬停（故障永远赢，Q9）。
		if state != CabinetMaintenance {
			return CabinetFault, true
		}
		return state, false
	case EvStart:
		if state == CabinetStandby || state == CabinetStopped {
			return CabinetStarting, true
		}
	case EvStartConfirmed:
		if state == CabinetStarting {
			return CabinetRunning, true
		}
	case EvStop:
		if state == CabinetRunning || state == CabinetStarting || state == CabinetStandby {
			return CabinetStopped, true
		}
	case EvStopConfirmed:
		if state == CabinetStopped || state == CabinetRunning {
			return CabinetStopped, true
		}
	case EvFaultCleared:
		// 故障只能人工确认解除，解除后回待机（不自动恢复运行）。
		if state == CabinetFault {
			return CabinetStandby, true
		}
	case EvEnterMaintenance:
		if state != CabinetFault {
			return CabinetMaintenance, true
		}
	case EvExitMaintenance:
		if state == CabinetMaintenance {
			return CabinetStandby, true
		}
	}
	return state, false
}

// AllowsAutoDispatch 判断该状态是否允许自动调度。只有运行态允许；故障/
// 检修/停机/启动中一律禁止——这是互锁落到状态机的闸门。
func AllowsAutoDispatch(state CabinetState) bool {
	return state == CabinetRunning
}

// NeedsManualRelease 判断该状态是否需要人工确认才能恢复（故障/检修）。
func NeedsManualRelease(state CabinetState) bool {
	return state == CabinetFault || state == CabinetMaintenance
}
