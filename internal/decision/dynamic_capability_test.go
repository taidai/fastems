package decision

import "testing"

func baseLimit() CapabilityLimit {
	return CapabilityLimit{
		Rated: 250, Available: 200, SOC: 60, SOCMin: 10, SOCMax: 90,
		Temp: 30, TempDerate: 45, TempLimit: 55,
	}
}

func TestDynamicDischarge(t *testing.T) {
	t.Run("min of rated and available", func(t *testing.T) {
		capacity, ok := DynamicDischarge(baseLimit())
		if !ok || capacity != 200 {
			t.Fatalf("expected 200, got %.1f ok=%v", capacity, ok)
		}
	})

	t.Run("low SOC blocks discharge", func(t *testing.T) {
		limit := baseLimit()
		limit.SOC = 5
		if _, ok := DynamicDischarge(limit); ok {
			t.Fatal("low SOC must block discharge")
		}
	})

	t.Run("high temperature blocks", func(t *testing.T) {
		limit := baseLimit()
		limit.Temp = 56
		if _, ok := DynamicDischarge(limit); ok {
			t.Fatal("over-limit temperature must block")
		}
	})

	t.Run("temperature derates", func(t *testing.T) {
		limit := baseLimit()
		limit.Temp = 50 // 降额区间中点，能力应约为一半
		capacity, ok := DynamicDischarge(limit)
		if !ok || capacity >= 200 {
			t.Fatalf("expected derated below 200, got %.1f", capacity)
		}
	})
}

func TestDynamicCharge(t *testing.T) {
	t.Run("high SOC blocks charge", func(t *testing.T) {
		limit := baseLimit()
		limit.SOC = 95
		if _, ok := DynamicCharge(limit); ok {
			t.Fatal("high SOC must block charge")
		}
	})

	t.Run("normal SOC allows charge", func(t *testing.T) {
		capacity, ok := DynamicCharge(baseLimit())
		if !ok || capacity != 200 {
			t.Fatalf("expected 200, got %.1f ok=%v", capacity, ok)
		}
	})
}
