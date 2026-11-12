package appapi

import (
	"testing"
	"time"

	"fastems/internal/ems/command"
	"fastems/internal/ems/device"
	"fastems/internal/ems/signal"
)

type nullDispatcher struct{}

func (nullDispatcher) Write(string, string, []device.Write) error { return nil }

func testManager() *command.Manager {
	return command.New(
		nullDispatcher{},
		func(string) (device.Profile, bool) { return device.Profile{}, false },
		func(string, time.Time) map[string]signal.Sample { return nil },
		15*time.Second, 5*time.Second,
	)
}

func TestCommandHistorySurvivesRestart(t *testing.T) {
	dataDir := t.TempDir()
	first := testManager()
	WireCommandHistory(dataDir, first)
	started := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	first.Persist(command.Outcome{
		RequestID: "req-1", DeviceID: "ch1/1", CommandID: "on", Actor: "operator",
		State: command.Confirmed, StartedAt: started, UpdatedAt: started.Add(2 * time.Second),
	})
	first.Persist(command.Outcome{
		RequestID: "req-2", DeviceID: "ch1/1", CommandID: "off", Actor: "operator",
		State: command.Executing, StartedAt: started.Add(time.Minute), UpdatedAt: started.Add(time.Minute),
	})

	second := testManager()
	WireCommandHistory(dataDir, second)
	outcomes := second.List()
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 restored outcomes, got %d", len(outcomes))
	}
	if outcomes[0].RequestID != "req-1" || outcomes[0].State != command.Confirmed || !outcomes[0].Restored {
		t.Fatalf("confirmed outcome not restored: %+v", outcomes[0])
	}
	if outcomes[1].State != command.Unknown || outcomes[1].Reason == "" {
		t.Fatalf("in-flight outcome must restore as unknown with a reason: %+v", outcomes[1])
	}
}

func TestCommandHistoryKeepsLastStatePerRequest(t *testing.T) {
	dataDir := t.TempDir()
	first := testManager()
	WireCommandHistory(dataDir, first)
	started := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	first.Persist(command.Outcome{RequestID: "req-1", DeviceID: "d", CommandID: "c", State: command.Executing, StartedAt: started, UpdatedAt: started})
	first.Persist(command.Outcome{RequestID: "req-1", DeviceID: "d", CommandID: "c", State: command.Failed, Reason: "write failed", StartedAt: started, UpdatedAt: started.Add(time.Second)})

	second := testManager()
	WireCommandHistory(dataDir, second)
	outcomes := second.List()
	if len(outcomes) != 1 || outcomes[0].State != command.Failed || outcomes[0].Reason != "write failed" {
		t.Fatalf("last state must win: %+v", outcomes)
	}
}
