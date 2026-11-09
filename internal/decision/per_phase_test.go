package decision

import "testing"

// T24 三相合计与逐相反送：三相 −6、+3、+3，反送限值 0。
// 合相净值为 0，A 相有 6 kW 反送偏差。
func TestAcceptanceT24PerPhase(t *testing.T) {
	reading := PhaseReading{A: -6, B: 3, C: 3}

	t.Run("net zero but phase A reverse", func(t *testing.T) {
		verdict := EvaluatePhase(reading, 0, false)
		if verdict.NetKW != 0 {
			t.Fatalf("合相净值应为 0，实测 %.1f", verdict.NetKW)
		}
		if verdict.PhaseReverseKW[0] != 6 {
			t.Fatalf("A 相反送应为 6，实测 %.1f", verdict.PhaseReverseKW[0])
		}
		if verdict.PhaseReverseKW[1] != 0 || verdict.PhaseReverseKW[2] != 0 {
			t.Fatalf("B/C 相无反送，实测 %v", verdict.PhaseReverseKW)
		}
		if !verdict.CanPerPhase && verdict.Note == "" {
			t.Fatal("不支持分相时应说明无法独立调相")
		}
	})

	t.Run("per-phase capable reports correctly", func(t *testing.T) {
		verdict := EvaluatePhase(reading, 0, true)
		if !verdict.CanPerPhase {
			t.Fatal("支持分相时 CanPerPhase 应为 true")
		}
	})
}
