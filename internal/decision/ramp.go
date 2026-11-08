package decision

import "math"

// 爬坡限幅与死区（Q11）：默认爬坡 10 kW/s、死区 5 kW，均可配置。每 1s
// 协调周期执行一次，把目标设定值的变化速率限制在爬坡上限内，并在死区内
// 保持当前值不变，避免微小波动造成设备频繁动作。

// RampParams 是爬坡与死区参数，可配置。
type RampParams struct {
	RampRateKW   float64 // 爬坡速率 kW/s
	DeadbandKW   float64 // 死区 kW
}

// DefaultRampParams 返回定案默认值（Q11=A）。
func DefaultRampParams() RampParams {
	return RampParams{RampRateKW: 10, DeadbandKW: 5}
}

// RampLimit 把目标值 target 从当前值 current 按爬坡速率限幅推进一个周期
// （1s），死区内的偏差忽略（保持 current）。返回下一个应执行的设定值。
func RampLimit(current, target float64, params RampParams) float64 {
	delta := target - current
	if math.Abs(delta) <= params.DeadbandKW {
		return current
	}
	step := params.RampRateKW
	if step <= 0 {
		return target
	}
	if delta > step {
		delta = step
	} else if delta < -step {
		delta = -step
	}
	return current + delta
}
