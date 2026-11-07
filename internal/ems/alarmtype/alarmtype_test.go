package alarmtype

import (
	"testing"
	"time"
)

func TestActiveThresholdsAndStaleness(t *testing.T) {
	now := time.Now()
	base := Config{Code: "T_HIGH", Name: "温度高", Severity: "warning", Trigger: "gt", PointID: "t", Value: 40, Confirm: "manual"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	if !base.Active(now, Sample{Present: true, Value: 45, ValueTime: now}) {
		t.Fatal("45 should exceed 40")
	}
	if base.Active(now, Sample{Present: true, Value: 35, ValueTime: now}) {
		t.Fatal("35 should not exceed 40")
	}
	if base.Active(now, Sample{Present: true, Value: 45, ValueTime: now.Add(-20 * time.Second)}) {
		t.Fatal("stale value must not trigger")
	}
	offline := Config{Code: "DEV_OFF", Name: "设备离线", Severity: "critical", Trigger: "offline", PointID: "c", Confirm: "auto"}
	if !offline.Active(now, Sample{Offline: true}) {
		t.Fatal("offline sample should trigger")
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	for _, config := range []Config{
		{Code: "x", Name: "n", Severity: "warning", Trigger: "gt", PointID: "p", Confirm: "manual"},
		{Code: "OK", Name: "", Severity: "warning", Trigger: "gt", PointID: "p", Confirm: "manual"},
		{Code: "OK", Name: "n", Severity: "fatal", Trigger: "gt", PointID: "p", Confirm: "manual"},
		{Code: "OK", Name: "n", Severity: "warning", Trigger: "between", PointID: "p", Confirm: "manual"},
		{Code: "OK", Name: "n", Severity: "warning", Trigger: "gt", PointID: "", Confirm: "manual"},
		{Code: "OK", Name: "n", Severity: "warning", Trigger: "gt", PointID: "p", Confirm: "maybe"},
	} {
		if err := config.Validate(); err == nil {
			t.Fatalf("expected invalid config: %+v", config)
		}
	}
}
