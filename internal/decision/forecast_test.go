package decision

import "testing"

func TestForecast(t *testing.T) {
	t.Run("stable series near mean", func(t *testing.T) {
		samples := []ForecastSample{{50}, {50}, {50}, {50}}
		out := Forecast(samples, 2)
		if len(out) != 2 || out[0] != 50 || out[1] != 50 {
			t.Fatalf("稳定序列应预测 50，实测 %v", out)
		}
	})
	t.Run("rising series increases", func(t *testing.T) {
		samples := []ForecastSample{{10}, {20}, {30}, {40}}
		out := Forecast(samples, 2)
		if out[1] <= out[0] {
			t.Fatalf("上升序列预测应递增，实测 %v", out)
		}
	})
	t.Run("empty returns nil", func(t *testing.T) {
		if Forecast(nil, 3) != nil {
			t.Fatal("空样本应返回 nil")
		}
	})
	t.Run("no negative power", func(t *testing.T) {
		samples := []ForecastSample{{5}, {4}, {3}, {2}}
		out := Forecast(samples, 10)
		for _, v := range out {
			if v < 0 {
				t.Fatalf("预测功率不应为负，实测 %v", out)
			}
		}
	})
}
