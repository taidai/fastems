package decision

import "testing"

func TestArbitrate(t *testing.T) {
	t.Run("safety wins over everything", func(t *testing.T) {
		goals := []Goal{
			{Priority: GoalPrice, RequestKW: 300},
			{Priority: GoalSafety, RequestKW: 0},
			{Priority: GoalSchedule, RequestKW: 100},
		}
		got, ok := Arbitrate(goals)
		if !ok || got.Priority != GoalSafety || got.RequestKW != 0 {
			t.Fatalf("safety must win, got %+v ok=%v", got, ok)
		}
	})

	t.Run("empty is no-op", func(t *testing.T) {
		if _, ok := Arbitrate(nil); ok {
			t.Fatal("empty goals must not arbitrate")
		}
	})

	t.Run("first wins on tie", func(t *testing.T) {
		goals := []Goal{
			{Priority: GoalPeakShaving, RequestKW: 50},
			{Priority: GoalPeakShaving, RequestKW: 80},
		}
		got, _ := Arbitrate(goals)
		if got.RequestKW != 50 {
			t.Fatalf("tie must keep first, got %.1f", got.RequestKW)
		}
	})
}

func TestOrderByPriority(t *testing.T) {
	goals := []Goal{
		{Priority: GoalPrice, RequestKW: 1},
		{Priority: GoalSafety, RequestKW: 2},
		{Priority: GoalPeakShaving, RequestKW: 3},
	}
	ordered := OrderByPriority(goals)
	if ordered[0].Priority != GoalSafety || ordered[1].Priority != GoalPeakShaving || ordered[2].Priority != GoalPrice {
		t.Fatalf("wrong order: %+v", ordered)
	}
	if goals[0].Priority != GoalPrice {
		t.Fatal("OrderByPriority must not mutate input")
	}
}
