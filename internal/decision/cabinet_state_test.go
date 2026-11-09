package decision

import "testing"

func TestCabinetTransition(t *testing.T) {
	t.Run("start then confirm reaches running", func(t *testing.T) {
		s, ok := Transition(CabinetStandby, EvStart)
		if !ok || s != CabinetStarting {
			t.Fatalf("standby+start=%v ok=%v", s, ok)
		}
		s, ok = Transition(s, EvStartConfirmed)
		if !ok || s != CabinetRunning {
			t.Fatalf("starting+confirmed=%v ok=%v", s, ok)
		}
	})
	t.Run("fault from running hard-stops", func(t *testing.T) {
		s, ok := Transition(CabinetRunning, EvFault)
		if !ok || s != CabinetFault {
			t.Fatalf("running+fault=%v ok=%v", s, ok)
		}
	})
	t.Run("fault clears only to standby not running", func(t *testing.T) {
		s, ok := Transition(CabinetFault, EvFaultCleared)
		if !ok || s != CabinetStandby {
			t.Fatalf("fault+cleared=%v ok=%v, must not auto-resume", s, ok)
		}
	})
	t.Run("illegal transition keeps state", func(t *testing.T) {
		s, ok := Transition(CabinetRunning, EvStart)
		if ok || s != CabinetRunning {
			t.Fatalf("running+start must be rejected, got %v ok=%v", s, ok)
		}
	})
	t.Run("maintenance blocks fault event", func(t *testing.T) {
		if _, ok := Transition(CabinetMaintenance, EvFault); ok {
			t.Fatal("maintenance must not transition on fault")
		}
	})
}

func TestAllowsAutoDispatch(t *testing.T) {
	allowed := map[CabinetState]bool{
		CabinetRunning: true, CabinetStandby: false, CabinetStarting: false,
		CabinetStopped: false, CabinetFault: false, CabinetMaintenance: false,
	}
	for state, want := range allowed {
		if got := AllowsAutoDispatch(state); got != want {
			t.Fatalf("AllowsAutoDispatch(%v)=%v want %v", state, got, want)
		}
	}
}

func TestNeedsManualRelease(t *testing.T) {
	if !NeedsManualRelease(CabinetFault) || !NeedsManualRelease(CabinetMaintenance) {
		t.Fatal("fault/maintenance need manual release")
	}
	if NeedsManualRelease(CabinetRunning) {
		t.Fatal("running must not need manual release")
	}
}
