package appapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/decision"
	"fastems/internal/ems/history"
	"fastems/internal/ems/runtime"
	"fastems/internal/ems/signal"
	"fastems/internal/emsapi"
)

func TestPlan96TargetAt(t *testing.T) {
	value := 80.0
	plan := plan96{Version: 1, Slots: make([]*float64, 96)}
	plan.Slots[40] = &value
	encoded, _ := json.Marshal(plan)
	node := data.NodeEdge{Points: data.Points{data.NewPointString(plan96Point, "0", string(encoded))}}
	loaded, ok := loadPlan96(node)
	if !ok {
		t.Fatal("plan did not load")
	}
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 11, 11, hour, minute, 0, 0, time.UTC)
	}
	if got, has := loaded.targetAt(at(10, 5)); !has || got != 80 {
		t.Fatalf("10:05 target = %v, %v", got, has)
	}
	if _, has := loaded.targetAt(at(10, 20)); has {
		t.Fatal("empty slot must mean no plan target")
	}
	if _, has := loaded.targetAt(at(23, 59)); has {
		t.Fatal("unset slot must not produce a target")
	}
}

func TestPlantDemandWindow(t *testing.T) {
	service := runtime.NewService(nil, 15*time.Second, 5*time.Second, 100)
	now := time.Now()
	samples := make([]history.Sample, 0, 40)
	for i := 0; i < 40; i++ {
		samples = append(samples, history.Sample{
			DeviceID: "dev", PointID: "meter", Quality: signal.Good, Value: signal.Num(120),
			SourceTime: now.Add(-time.Duration(i) * 30 * time.Second),
		})
	}
	service.History.Record(samples)
	handler := &Handler{}
	handler.SetEMS(emsapi.New(service, 15*time.Second))

	window, err := handler.plantDemandWindow(decision.PCCFeedback{MeterPointID: "meter"}, now)
	if err != nil || !window.Available {
		t.Fatalf("window = %#v, %v", window, err)
	}
	if window.DemandKW < 119.99 || window.DemandKW > 120.01 {
		t.Fatalf("demand = %.2f", window.DemandKW)
	}

	// 缺一个 bin：窗口不可用，且给出缺样数量。
	service2 := runtime.NewService(nil, 15*time.Second, 5*time.Second, 100)
	missingFrom := now.Add(-7 * time.Minute)
	missingTo := now.Add(-6 * time.Minute)
	without := make([]history.Sample, 0, len(samples))
	for _, sample := range samples {
		if !sample.SourceTime.Before(missingFrom) && sample.SourceTime.Before(missingTo) {
			continue
		}
		without = append(without, sample)
	}
	service2.History.Record(without)
	handler2 := &Handler{}
	handler2.SetEMS(emsapi.New(service2, 15*time.Second))
	window, err = handler2.plantDemandWindow(decision.PCCFeedback{MeterPointID: "meter"}, now)
	if err != nil || window.Available || window.ValidBins != 14 || len(window.MissingBins) != 1 {
		t.Fatalf("missing window = %#v, %v", window, err)
	}
}
