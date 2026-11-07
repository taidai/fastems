package alarm

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func raise(t *testing.T, engine *Engine, at time.Time, key, detail string) Alarm {
	t.Helper()
	alarm, err := engine.Raise(at, key, Critical, "设备通信中断", detail, "d1", "temp")
	if err != nil {
		t.Fatalf("raise: %v", err)
	}
	return alarm
}

func TestRaiseDeduplicatesLiveCondition(t *testing.T) {
	engine := New()
	first := raise(t, engine, t0, "d1:offline", "timeout")
	second := raise(t, engine, t0.Add(time.Second), "d1:offline", "timeout")
	if engine.Active() != 1 || len(engine.List()) != 1 {
		t.Fatalf("expected one alarm, got %d", len(engine.List()))
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("repeated condition must not reset creation time")
	}
	changed := raise(t, engine, t0.Add(2*time.Second), "d1:offline", "no response for 30s")
	if changed.Detail != "no response for 30s" {
		t.Fatalf("detail not updated: %+v", changed)
	}
}

func TestClearResolvesAndRecurrenceReactivates(t *testing.T) {
	engine := New()
	raise(t, engine, t0, "d1:offline", "timeout")
	cleared, err := engine.Clear(t0.Add(time.Minute), "d1:offline")
	if err != nil || cleared.Active || cleared.State != Resolved {
		t.Fatalf("unexpected clear result %+v err=%v", cleared, err)
	}
	if engine.Active() != 0 {
		t.Fatal("resolved alarm must not count as active")
	}
	again := raise(t, engine, t0.Add(2*time.Minute), "d1:offline", "timeout")
	if !again.Active || again.State != Active || !again.CreatedAt.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("recurrence not reactivated: %+v", again)
	}
}

func TestAcknowledgeAndCloseRules(t *testing.T) {
	engine := New()
	raise(t, engine, t0, "d1:offline", "timeout")
	if _, err := engine.Close(t0, "d1:offline"); err == nil {
		t.Fatal("active alarms must not be closed directly")
	}
	acked, err := engine.Acknowledge(t0.Add(time.Second), "d1:offline", "operator")
	if err != nil || acked.State != Acknowledged || acked.AckedBy != "operator" || !acked.Active {
		t.Fatalf("unexpected ack %+v err=%v", acked, err)
	}
	if _, err := engine.Acknowledge(t0.Add(2*time.Second), "d1:offline", "operator"); err == nil {
		t.Fatal("acknowledging twice must fail")
	}
	if _, err := engine.Clear(t0.Add(3*time.Second), "d1:offline"); err != nil {
		t.Fatal(err)
	}
	closed, err := engine.Close(t0.Add(4*time.Second), "d1:offline")
	if err != nil || closed.State != Closed || closed.Active {
		t.Fatalf("unexpected close %+v err=%v", closed, err)
	}
	if _, err := engine.Acknowledge(t0.Add(5*time.Second), "d1:offline", "operator"); err == nil {
		t.Fatal("closed alarm must not be acknowledged")
	}
}

func TestListPutsActiveFirst(t *testing.T) {
	engine := New()
	raise(t, engine, t0, "a", "first")
	if _, err := engine.Clear(t0.Add(time.Second), "a"); err != nil {
		t.Fatal(err)
	}
	raise(t, engine, t0.Add(2*time.Second), "b", "second")
	list := engine.List()
	if len(list) != 2 || list[0].Key != "b" || list[1].Key != "a" {
		t.Fatalf("order = %+v", list)
	}
}

func TestInvalidInputsAreRejected(t *testing.T) {
	engine := New()
	if _, err := engine.Raise(t0, "", Critical, "t", "d", "", ""); err == nil {
		t.Fatal("empty key must be rejected")
	}
	if _, err := engine.Raise(t0, "k", "urgent", "t", "d", "", ""); err == nil {
		t.Fatal("invalid severity must be rejected")
	}
	if _, err := engine.Acknowledge(t0, "missing", "user"); err == nil {
		t.Fatal("unknown alarm must be rejected")
	}
}

func TestMaintenanceSuppressesRaiseUntilReleased(t *testing.T) {
	engine := New()
	now := time.Now()
	if _, err := engine.Raise(now, "k", Warning, "温度高", "45", "d", "p"); err != nil {
		t.Fatal(err)
	}
	until := now.Add(time.Hour)
	if _, err := engine.Shelve(now, "k", "检修", "admin", until); err != nil {
		t.Fatal(err)
	}
	alarm, _ := engine.Get("k")
	if alarm.MaintenanceUntil.IsZero() || alarm.MaintenanceReason != "检修" || alarm.MaintainedBy != "admin" {
		t.Fatalf("alarm = %+v", alarm)
	}
	if _, err := engine.Clear(now.Add(time.Minute), "k"); err != nil {
		t.Fatal(err)
	}
	// During the window a recurring condition must not become active again.
	if _, err := engine.Raise(now.Add(2*time.Minute), "k", Warning, "温度高", "46", "d", "p"); err != nil {
		t.Fatal(err)
	}
	alarm, _ = engine.Get("k")
	if alarm.Active {
		t.Fatalf("maintained alarm became active: %+v", alarm)
	}
	// After the window it can raise again.
	if _, err := engine.Raise(until.Add(time.Minute), "k", Warning, "温度高", "47", "d", "p"); err != nil {
		t.Fatal(err)
	}
	alarm, _ = engine.Get("k")
	if !alarm.Active {
		t.Fatalf("alarm did not reactivate: %+v", alarm)
	}
	if _, err := engine.Release(now.Add(3*time.Minute), "k"); err != nil {
		t.Fatal(err)
	}
	alarm, _ = engine.Get("k")
	if !alarm.MaintenanceUntil.IsZero() || alarm.MaintenanceReason != "" {
		t.Fatalf("maintenance not released: %+v", alarm)
	}
}

func TestMaintenanceExportImport(t *testing.T) {
	engine := New()
	now := time.Now()
	if _, err := engine.Shelve(now, "future", "检修", "admin", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	exported := engine.ExportMaintenance()
	restored := New()
	restored.ImportMaintenance(exported)
	if _, err := restored.Raise(now.Add(time.Minute), "future", Warning, "测试", "detail", "d", "p"); err != nil {
		t.Fatal(err)
	}
	alarm, _ := restored.Get("future")
	if alarm.Active {
		t.Fatalf("imported maintenance did not suppress raise: %+v", alarm)
	}
}

func TestAcknowledgeNote(t *testing.T) {
	engine := New()
	at := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
	if _, err := engine.Raise(at, "k", Warning, "温度高", "46", "d", "p"); err != nil {
		t.Fatal(err)
	}
	acked, err := engine.AcknowledgeWithNote(at.Add(time.Second), "k", "operator", "已联系现场")
	if err != nil {
		t.Fatal(err)
	}
	if acked.AckNote != "已联系现场" {
		t.Fatalf("ack note = %q", acked.AckNote)
	}
}
