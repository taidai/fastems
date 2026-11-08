package decision

import "testing"

func TestInterlocked(t *testing.T) {
	if !Interlocked(InterlockState{Fault: true}) {
		t.Fatal("fault must interlock")
	}
	if !Interlocked(InterlockState{OverLimit: true}) {
		t.Fatal("over-limit must interlock")
	}
	if Interlocked(InterlockState{}) {
		t.Fatal("clean state must not interlock")
	}
	if Interlocked(InterlockState{Fault: true, ManualRelease: true}) {
		t.Fatal("manual release must clear interlock")
	}
}

func TestRollback(t *testing.T) {
	if got := Rollback(CommandState{TimedOut: true, HasConfirmed: true, LastConfirmed: 120}); got != 120 {
		t.Fatalf("must roll back to last confirmed, got %.1f", got)
	}
	if got := Rollback(CommandState{TimedOut: true}); got != 0 {
		t.Fatalf("no confirmed value must roll back to 0, got %.1f", got)
	}
}

func TestManualOverrides(t *testing.T) {
	if _, ok := ManualOverrides(ControlAuto, 0, false, 0); ok {
		t.Fatal("auto mode must not override")
	}
	if v, ok := ManualOverrides(ControlManual, 75, true, 0); !ok || v != 75 {
		t.Fatalf("manual setpoint must win, got %.1f ok=%v", v, ok)
	}
	if v, ok := ManualOverrides(ControlManual, 0, false, 30); !ok || v != 30 {
		t.Fatalf("manual without value must fall back, got %.1f ok=%v", v, ok)
	}
}
