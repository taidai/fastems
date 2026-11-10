package decision

// PCC 反馈绑定（TOP-02 / OBJ-01 首版子集）：防逆流/需量要闭环，必须知道
// "PCC 功率从哪个点位读、购售方向、数据有效期"。此前 AntiReverse 吃的是
// 空参数，没有反馈来源。这里定义反馈绑定的语义与质量判定。

// PCCFeedback 是一个并网点（PCC）的反馈绑定：主表点位、购售方向、有效期。
// 首版单并网点；多 PCC 时每个 PCC 各持有一份绑定（范围外暂缓）。
type PCCFeedback struct {
	MeterPointID string  // 主表点位 ID（grid.active_power 所在节点）
	Sign         float64 // 购售方向：+1 购电为正（默认），-1 送电为正
	MaxAgeMS     int64   // 数据有效期 ms；0 = 不检查
	UpperLimitKW float64 // 购电上限（需量），0 = 不限
	LowerLimitKW float64 // 送电下限（反送阈值），通常 0 = 禁止反送
	// PhasePointIDs 是可选的 A/B/C 三相功率点位；三相同为新鲜数据时才给出
	// 逐相判定，否则只按合相处理。
	PhasePointIDs []string
}

// PCCReading 是读到的一次 PCC 实测。
type PCCReading struct {
	KW     float64 // 归一化后的 PCC 功率（已乘 Sign，购电为正）
	Usable bool    // 数据是否可用于闭环控制
	Reason string  // 不可用原因（缺点位/非数值/过期）
}

// ReadPCC 用原始主表读数构造一次 PCC 实测。rawKW 是主表原始值，ageMS 是
// 数据年龄（采样或接收至今），valid 表示点位存在且为数值。
func ReadPCC(binding PCCFeedback, rawKW float64, ageMS int64, valid bool) PCCReading {
	if !valid {
		return PCCReading{Usable: false, Reason: "主表点位缺失或非数值"}
	}
	if binding.MaxAgeMS > 0 && ageMS > binding.MaxAgeMS {
		return PCCReading{Usable: false, Reason: "主表数据过期"}
	}
	sign := binding.Sign
	if sign == 0 {
		sign = 1
	}
	return PCCReading{KW: rawKW * sign, Usable: true}
}

// AntiReverseFromFeedback 用 PCC 实测计算防逆流削减量。数据不可用时返回
// ok=false，调用方应跳过本轮调节（过期数据不授权动作，Q6/G06）。
func AntiReverseFromFeedback(binding PCCFeedback, reading PCCReading) (float64, bool) {
	if !reading.Usable {
		return 0, false
	}
	return AntiReverse(reading.KW - binding.LowerLimitKW), true
}

// PeakShavingFromFeedback 用 PCC 实测计算需量削减量。
func PeakShavingFromFeedback(binding PCCFeedback, reading PCCReading) (float64, bool) {
	if !reading.Usable {
		return 0, false
	}
	return PeakShaving(reading.KW, binding.UpperLimitKW), true
}
