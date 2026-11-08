package decision

import "testing"

func TestRampLimit(t *testing.T) {
	params := DefaultRampParams()

	t.Run("within deadband holds current", func(t *testing.T) {
		if got := RampLimit(100, 103, params); got != 100 {
			t.Fatalf("deadband must hold, got %.1f", got)
		}
	})

	t.Run("caps upward step", func(t *testing.T) {
		if got := RampLimit(100, 200, params); got != 110 {
			t.Fatalf("must cap at 10 kW/s, got %.1f", got)
		}
	})

	t.Run("caps downward step", func(t *testing.T) {
		if got := RampLimit(100, 0, params); got != 90 {
			t.Fatalf("must cap downward at 10 kW/s, got %.1f", got)
		}
	})

	t.Run("within one step reaches target", func(t *testing.T) {
		if got := RampLimit(100, 108, params); got != 108 {
			t.Fatalf("small step must reach target, got %.1f", got)
		}
	})
}
