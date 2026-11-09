package decision

import "testing"

func baseBinding() PCCFeedback {
	return PCCFeedback{MeterPointID: "meter-1", Sign: 1, MaxAgeMS: 15000, UpperLimitKW: 100, LowerLimitKW: 0}
}

func TestReadPCC(t *testing.T) {
	t.Run("normalizes direction", func(t *testing.T) {
		r := ReadPCC(baseBinding(), -30, 1000, true)
		if !r.Usable || r.KW != -30 {
			t.Fatalf("expected usable -30, got %+v", r)
		}
	})
	t.Run("sign flips", func(t *testing.T) {
		b := baseBinding()
		b.Sign = -1
		r := ReadPCC(b, 30, 1000, true)
		if r.KW != -30 {
			t.Fatalf("sign must flip, got %.1f", r.KW)
		}
	})
	t.Run("invalid point unusable", func(t *testing.T) {
		if r := ReadPCC(baseBinding(), 0, 0, false); r.Usable {
			t.Fatal("invalid point must be unusable")
		}
	})
	t.Run("stale data unusable", func(t *testing.T) {
		if r := ReadPCC(baseBinding(), 10, 20000, true); r.Usable {
			t.Fatal("stale data must be unusable")
		}
	})
}

func TestAntiReverseFromFeedback(t *testing.T) {
	t.Run("reverse flow reduces", func(t *testing.T) {
		reading := ReadPCC(baseBinding(), -30, 100, true)
		reduction, ok := AntiReverseFromFeedback(baseBinding(), reading)
		if !ok || reduction != 30 {
			t.Fatalf("expected 30 reduction, got %.1f ok=%v", reduction, ok)
		}
	})
	t.Run("unusable blocks control", func(t *testing.T) {
		if _, ok := AntiReverseFromFeedback(baseBinding(), PCCReading{Usable: false}); ok {
			t.Fatal("unusable feedback must not authorize control")
		}
	})
	t.Run("no reverse no reduction", func(t *testing.T) {
		reading := ReadPCC(baseBinding(), 50, 100, true)
		if reduction, _ := AntiReverseFromFeedback(baseBinding(), reading); reduction != 0 {
			t.Fatalf("expected 0, got %.1f", reduction)
		}
	})
}

func TestPeakShavingFromFeedback(t *testing.T) {
	t.Run("over limit shaves", func(t *testing.T) {
		reading := ReadPCC(baseBinding(), 130, 100, true)
		shave, ok := PeakShavingFromFeedback(baseBinding(), reading)
		if !ok || shave != 30 {
			t.Fatalf("expected 30 shave, got %.1f ok=%v", shave, ok)
		}
	})
	t.Run("unusable blocks", func(t *testing.T) {
		if _, ok := PeakShavingFromFeedback(baseBinding(), PCCReading{Usable: false}); ok {
			t.Fatal("unusable must block")
		}
	})
}
