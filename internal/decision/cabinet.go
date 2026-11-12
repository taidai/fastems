package decision

import "fastiot/internal/ems/alloc"

// CabinetController 是能力级调度的下游执行层：把分给一个 EnergyUnit 的
// 功率请求，按柜内各 Device 的能力拆成每台 PCS 的功率设定。设备差异由
// Device 能力值吸收，调度逻辑不认具体 PCS 型号。

// DeviceCapability 是运行时读到的 Device 调度能力。
type DeviceCapability struct {
	DeviceID         string
	RatedPowerKW     float64
	AvailablePowerKW float64
}

// DeviceDispatch 是给一个 Device 的功率设定。
type DeviceDispatch struct {
	DeviceID   string  `json:"device_id"`
	SetpointKW  float64 `json:"setpoint_kw"`
	Excluded    bool    `json:"excluded"`
	Reason      string  `json:"reason,omitempty"`
}

// DispatchCabinet 把 EnergyUnit 的功率请求拆到其下的 Device，按能力比例。
// 复用 alloc.Allocate，只做 Device 能力 → 分配成员的适配。
func DispatchCabinet(requestKW float64, devices []DeviceCapability) ([]DeviceDispatch, error) {
	members := make([]alloc.Member, 0, len(devices))
	for _, device := range devices {
		members = append(members, alloc.Member{
			ID:                  device.DeviceID,
			Online:              true,
			MaxCharge:           device.RatedPowerKW,
			MaxDischarge:        device.RatedPowerKW,
			AvailableCharge:     device.AvailablePowerKW,
			AvailableDischarge:  device.AvailablePowerKW,
		})
	}
	result, err := alloc.Allocate(requestKW, members)
	if err != nil {
		return nil, err
	}
	dispatches := make([]DeviceDispatch, 0, len(result.Shares))
	for _, share := range result.Shares {
		dispatches = append(dispatches, DeviceDispatch{
			DeviceID:  share.MemberID,
			SetpointKW: share.Setpoint,
			Excluded:  share.Excluded,
			Reason:    share.Reason,
		})
	}
	return dispatches, nil
}
