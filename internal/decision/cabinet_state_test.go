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

// 停机请求先进入停机中，只有回读确认后才落到停机；停机中不允许自动调度。
func TestStopWaitsForConfirmation(t *testing.T) {
	next, ok := Transition(CabinetRunning, EvStop)
	if !ok || next != CabinetStopping {
		t.Fatalf("stop request = %v ok=%v, want stopping", next, ok)
	}
	if AllowsAutoDispatch(CabinetStopping) {
		t.Fatal("stopping must not be auto-dispatchable")
	}
	next, ok = Transition(CabinetStopping, EvStopConfirmed)
	if !ok || next != CabinetStopped {
		t.Fatalf("stop confirmed = %v ok=%v, want stopped", next, ok)
	}
	if _, ok := Transition(CabinetStopping, EvStart); ok {
		t.Fatal("start during stopping must be rejected")
	}
}
