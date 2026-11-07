package decision

import (
	"testing"
	"time"
)

// 2026-01-05 is a Monday.
func clockAt(day, hour, minute int) time.Time {
	return time.Date(2026, 1, day, hour, minute, 0, 0, time.Local)
}

const maskMonday = 1 << time.Monday
const maskMonToFri = 1<<time.Monday | 1<<time.Tuesday | 1<<time.Wednesday | 1<<time.Thursday | 1<<time.Friday

func TestTimeMatchesWindow(t *testing.T) {
	condition := Condition{Kind: "time", Start: "08:00", End: "20:00"}
	for _, step := range []struct {
		hour, minute int
		want         bool
	}{
		{7, 59, false}, {8, 0, true}, {19, 59, true}, {20, 0, false},
	} {
		if got := timeMatches(condition, clockAt(5, step.hour, step.minute)); got != step.want {
			t.Fatalf("%02d:%02d = %v, want %v", step.hour, step.minute, got, step.want)
		}
	}
}

func TestTimeMatchesCrossMidnight(t *testing.T) {
	condition := Condition{Kind: "time", Start: "22:00", End: "06:00"}
	for _, step := range []struct {
		hour, minute int
		want         bool
	}{
		{21, 59, false}, {22, 0, true}, {23, 59, true},
		{0, 0, true}, {5, 59, true}, {6, 0, false}, {12, 0, false},
	} {
		if got := timeMatches(condition, clockAt(5, step.hour, step.minute)); got != step.want {
			t.Fatalf("%02d:%02d = %v, want %v", step.hour, step.minute, got, step.want)
		}
	}
}

func TestCrossMidnightWeekdayBelongsToStartDay(t *testing.T) {
	condition := Condition{Kind: "time", Start: "22:00", End: "06:00", Weekdays: maskMonday}
	for _, step := range []struct {
		day, hour int
		want      bool
	}{
		{5, 23, true},  // Monday night: window started Monday
		{6, 3, true},   // Tuesday early morning: still Monday's window
		{6, 23, false}, // Tuesday night: Tuesday's window
		{5, 3, false},  // Monday early morning: Sunday's window
		{4, 23, false}, // Sunday night
	} {
		if got := timeMatches(condition, clockAt(step.day, step.hour, 0)); got != step.want {
			t.Fatalf("day %d %02d:00 = %v, want %v", step.day, step.hour, got, step.want)
		}
	}
}

func TestTimeMatchesWeekdayOnly(t *testing.T) {
	condition := Condition{Kind: "time", Weekdays: maskMonToFri}
	if !timeMatches(condition, clockAt(5, 12, 0)) {
		t.Fatal("Monday noon should match a Mon-Fri mask")
	}
	if timeMatches(condition, clockAt(3, 12, 0)) {
		t.Fatal("Saturday should not match a Mon-Fri mask")
	}
	if timeMatches(condition, clockAt(4, 0, 0)) {
		t.Fatal("Sunday midnight should not match a Mon-Fri mask")
	}
}

func TestTimeConditionValidation(t *testing.T) {
	valid := func(c Condition) error {
		table := Table{Rows: []Row{{ID: "row-1", Conditions: []Condition{c}}}}
		return table.Validate()
	}
	for _, condition := range []Condition{
		{Kind: "time"},                                          // empty
		{Kind: "time", Start: "08:00"},                          // missing end
		{Kind: "time", Start: "8:00", End: "20:00"},             // bad format
		{Kind: "time", Start: "08:00", End: "24:00"},            // out of range
		{Kind: "time", Start: "08:00", End: "08:00"},            // zero-length
		{Kind: "time", Start: "08:00", End: "20:00", Weekdays: 128}, // bad mask
	} {
		if err := valid(condition); err == nil {
			t.Fatalf("expected invalid time condition to fail: %+v", condition)
		}
	}
	for _, condition := range []Condition{
		{Kind: "time", Start: "22:00", End: "06:00"},
		{Kind: "time", Weekdays: maskMonday},
		{Kind: "time", Start: "08:00", End: "20:00", Weekdays: maskMonToFri},
	} {
		if err := valid(condition); err != nil {
			t.Fatalf("valid time condition rejected: %+v: %v", condition, err)
		}
	}
}

func timeDocument(timeCondition Condition) Document {
	return Document{
		Table: Table{Rows: []Row{{
			ID: "night",
			Conditions: []Condition{
				{PointID: "temp", Operator: "gt", Value: 30},
				timeCondition,
			},
			Actions: []Action{{ActionID: "set", Value: 1}},
		}}},
		Bindings: BindingSet{
			Points:  map[string]Target{"temp": {NodeID: "device-1", PointType: "value"}},
			Actions: map[string]Target{"set": {NodeID: "device-1", PointType: "valueSet"}},
		},
	}
}

func TestSimulateTimeCondition(t *testing.T) {
	document := timeDocument(Condition{Kind: "time", Start: "22:00", End: "06:00", Weekdays: maskMonToFri})

	hit, err := document.Simulate(map[string]float64{"temp": 35}, nil, clockAt(5, 23, 30))
	if err != nil {
		t.Fatal(err)
	}
	if hit.RowID != "night" {
		t.Fatalf("expected night row at Monday 23:30, got %+v", hit)
	}
	if len(hit.Times) != 1 || !hit.Times[0].Satisfied {
		t.Fatalf("time check should report satisfied: %+v", hit.Times)
	}

	miss, err := document.Simulate(map[string]float64{"temp": 35}, nil, clockAt(5, 12, 0))
	if err != nil {
		t.Fatal(err)
	}
	if miss.RowID != "" {
		t.Fatalf("expected no match at Monday noon, got %+v", miss)
	}
	if len(miss.Times) != 1 || miss.Times[0].Satisfied {
		t.Fatalf("time check should report not satisfied: %+v", miss.Times)
	}

	// Saturday 23:30 is inside the window but outside the weekday mask.
	weekend, err := document.Simulate(map[string]float64{"temp": 35}, nil, clockAt(3, 23, 30))
	if err != nil {
		t.Fatal(err)
	}
	if weekend.RowID != "" {
		t.Fatalf("expected no match on Saturday night, got %+v", weekend)
	}
}

func TestTimeSamplesDeduplicates(t *testing.T) {
	condition := Condition{Kind: "time", Start: "22:00", End: "06:00"}
	other := Condition{Kind: "time", Weekdays: maskMonday}
	table := Table{Rows: []Row{
		{ID: "a", Conditions: []Condition{condition}},
		{ID: "b", Conditions: []Condition{condition, other}},
	}}
	samples := TimeSamples(table, clockAt(5, 23, 0))
	if len(samples) != 2 {
		t.Fatalf("expected 2 unique time samples, got %d", len(samples))
	}
	for id, sample := range samples {
		if sample.Input != id || sample.Quality.String() != "good" {
			t.Fatalf("unexpected sample %+v", sample)
		}
	}
}
