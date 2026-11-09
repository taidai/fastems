package decision

// 逐相反送（TOP-06/PCC-02/T24）：防逆流可按合相净值或逐相分别判定。
// 合相净值为零时，单相仍可能反送——例如三相 −6、+3、+3，合相为 0，
// 但 A 相有 6 kW 反送。执行器若只支持三相对称调节，须如实报告无法独立
// 调相，而不是承诺能调。

// PhaseReading 是一个 PCC 的三相读数（购电为正、送电为负）。
type PhaseReading struct {
	A, B, C float64
}

// PerPhaseAntiReverse 分别计算各相的反送削减量（正值=该相需削减的反送）。
// 每相独立判定：该相 < 反送下限（通常 0）即需削减。
func PerPhaseAntiReverse(reading PhaseReading, lowerLimitKW float64) (a, b, c float64) {
	a = AntiReverse(reading.A - lowerLimitKW)
	b = AntiReverse(reading.B - lowerLimitKW)
	c = AntiReverse(reading.C - lowerLimitKW)
	return a, b, c
}

// PhaseVerdict 是逐相判定结果：合相净值与各相反送，及执行器能否分相调节。
type PhaseVerdict struct {
	NetKW          float64 // 合相净功率
	PhaseReverseKW [3]float64 // 各相反送削减量
	CanPerPhase    bool    // 执行器是否支持分相调节
	Note           string  // 可解释说明
}

// EvaluatePhase 判定一个三相读数：合相与各相反送。canPerPhase 表示执行器
// 是否支持分相调节；不支持时，即使识别了单相反送也只能按对称调节处理。
func EvaluatePhase(reading PhaseReading, lowerLimitKW float64, canPerPhase bool) PhaseVerdict {
	a, b, c := PerPhaseAntiReverse(reading, lowerLimitKW)
	net := reading.A + reading.B + reading.C
	note := "合相判定"
	if canPerPhase {
		note = "支持逐相判定与调节"
	} else if a+b+c > 0 {
		note = "识别单相反送，但执行器仅支持三相对称调节，不能独立调相"
	}
	return PhaseVerdict{
		NetKW:          net,
		PhaseReverseKW: [3]float64{a, b, c},
		CanPerPhase:    canPerPhase,
		Note:           note,
	}
}
