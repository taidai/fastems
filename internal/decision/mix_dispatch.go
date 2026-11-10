package decision

// 光储充协同（MIX-01/T23）：同一并网点（PCC）下，光伏限发、储能充放、充电
// 限流的联合调节。为维持 PCC 净购电目标，光伏出力波动时按优先级协同——
// 先储能多放/少放，再充电减载，最后光伏限发。不是简单方向剔除，而是真正
// 的联合优化，每步可解释、可追溯。

// MixInput 是一次光储充联合调节的输入快照。功率单位 kW，放电为正、充电为负。
type MixInput struct {
	TargetPCCKW         float64 // PCC 净购电目标
	BaseLoadKW          float64 // 基础负荷
	PVActualKW          float64 // 光伏实测出力
	ESSNetKW            float64 // 储能净功率（放电正、充电负）
	EVLoadKW            float64 // 充电当前负载
	ESSExtraDischargeKW float64 // 储能可额外放电能力
	ESSExtraChargeKW    float64 // 储能可额外充电能力（net<target 时少用）
	EVReducibleKW       float64 // 充电可减少负荷
	PVReducibleKW       float64 // 光伏可限发
}

// MixResult 是联合调节的结果：各资源新设定与剩余缺口。
type MixResult struct {
	ESSNetKW    float64 // 储能新净功率目标
	EVLoadKW    float64 // 充电新负载目标
	PVLimitKW   float64 // 光伏限发设定（≤ PVActual）
	NetPCCKW    float64 // 调节后净购电
	ShortfallKW float64 // 仍无法满足的缺口
	Reason      string  // 调节说明（可解释）
}

// MixDispatch 计算光储充联合调节。净购电 = 基础负荷 + 充电负载 − 储能净功率
// − 光伏出力（储能放电为正，放电降低净购电）。缺口按优先级分配。
func MixDispatch(in MixInput) MixResult {
	net := in.BaseLoadKW + in.EVLoadKW - in.ESSNetKW - in.PVActualKW
	gap := net - in.TargetPCCKW // >0 净购电超目标，需多放/减载；<0 需少放/多充

	essNet := in.ESSNetKW
	evLoad := in.EVLoadKW
	pvLimit := in.PVActualKW
	reason := ""

	if gap > 0 {
		// 净购电超目标：储能多放，再充电减载。光伏限发会减少发电、抬高净购电，
		// 方向相反，不能用于这一侧。
		essAdd := minf(gap, in.ESSExtraDischargeKW)
		essNet += essAdd
		remaining := gap - essAdd
		if remaining > 0 {
			evReduce := minf(remaining, in.EVReducibleKW)
			evLoad -= evReduce
			remaining -= evReduce
		}
		reason = "净购电超目标，按储能→充电顺序削减"
		net = in.BaseLoadKW + evLoad - essNet - pvLimit
		return MixResult{ESSNetKW: essNet, EVLoadKW: evLoad, PVLimitKW: pvLimit, NetPCCKW: net, ShortfallKW: remaining, Reason: reason}
	}

	// 净购电低于目标（外送偏多）：储能多充；仍不足时再限制光伏出力。
	essReduce := minf(-gap, in.ESSExtraChargeKW)
	essNet -= essReduce
	remaining := -gap - essReduce
	if remaining > 0 {
		pvReduce := minf(remaining, in.PVReducibleKW)
		pvLimit -= pvReduce
		remaining -= pvReduce
	}
	net = in.BaseLoadKW + in.EVLoadKW - essNet - pvLimit
	reason = "净购电低于目标，储能多充；外送仍偏多时限制光伏"
	return MixResult{ESSNetKW: essNet, EVLoadKW: evLoad, PVLimitKW: pvLimit, NetPCCKW: net, ShortfallKW: remaining, Reason: reason}
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
