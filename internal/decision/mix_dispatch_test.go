package decision

import "testing"

// T23 光储充联合调节：负荷100、充电30、光伏降至35、储能放电20，目标净购电60。
// 第一步储能多放15（到35）达标；第二步储能限增10、充电减5（到25）达标。
func TestAcceptanceT23MixDispatch(t *testing.T) {
	t.Run("step1 storage covers PV drop", func(t *testing.T) {
		result := MixDispatch(MixInput{
			TargetPCCKW: 60, BaseLoadKW: 100, PVActualKW: 35, ESSNetKW: 20, EVLoadKW: 30,
			ESSExtraDischargeKW: 15, EVReducibleKW: 0, PVReducibleKW: 0,
		})
		if result.ESSNetKW != 35 {
			t.Fatalf("储能应增放至 35，实测 %.1f", result.ESSNetKW)
		}
		if result.NetPCCKW != 60 {
			t.Fatalf("净购电应回到 60，实测 %.1f", result.NetPCCKW)
		}
		if result.ShortfallKW != 0 {
			t.Fatalf("不应有缺口，实测 %.1f", result.ShortfallKW)
		}
	})

	t.Run("step2 storage limited, EV reduces", func(t *testing.T) {
		result := MixDispatch(MixInput{
			TargetPCCKW: 60, BaseLoadKW: 100, PVActualKW: 35, ESSNetKW: 20, EVLoadKW: 30,
			ESSExtraDischargeKW: 10, EVReducibleKW: 5, PVReducibleKW: 0,
		})
		if result.ESSNetKW != 30 {
			t.Fatalf("储能限增至 30，实测 %.1f", result.ESSNetKW)
		}
		if result.EVLoadKW != 25 {
			t.Fatalf("充电应减至 25，实测 %.1f", result.EVLoadKW)
		}
		if result.NetPCCKW != 60 {
			t.Fatalf("净购电应回到 60，实测 %.1f", result.NetPCCKW)
		}
		if result.ShortfallKW != 0 {
			t.Fatalf("不应有缺口，实测 %.1f", result.ShortfallKW)
		}
	})

	t.Run("shortfall reported when insufficient", func(t *testing.T) {
		result := MixDispatch(MixInput{
			TargetPCCKW: 60, BaseLoadKW: 100, PVActualKW: 35, ESSNetKW: 20, EVLoadKW: 30,
			ESSExtraDischargeKW: 5, EVReducibleKW: 0, PVReducibleKW: 0,
		})
		if result.ShortfallKW != 10 {
			t.Fatalf("能力不足应报缺口 10，实测 %.1f", result.ShortfallKW)
		}
	})
}
