package runtime

import (
	"testing"
	"time"

	"fastiot/internal/ems/channel"
	"fastiot/internal/ems/signal"
)

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func tempDevice() channel.Device {
	return channel.Device{
		ID: "d1", ServerID: 1,
		Points: []channel.Point{{
			ID: "temp", Function: channel.HoldingRegister, Address: 10,
			Format: "uint16", Scale: 0.1, Kind: signal.Number,
		}},
	}
}

func TestServicePollsAndRecordsHistory(t *testing.T) {
	bus := &fakeBus{coils: map[uint16]bool{}, holding: map[uint16]uint16{10: 351}, fail: map[byte]bool{}}
	service := NewService(nil, 5*time.Second, 3*time.Second, 100)
	defer service.Close()
	if err := service.AddChannel("c1", bus, []channel.Device{tempDevice()}, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		value, ok := service.State.Get(time.Now(), "d1", "temp", time.Minute)
		return ok && value.Value.Num == 35.1
	})
	waitFor(t, func() bool { return service.History.Len("d1", "temp") >= 2 })
	history := service.History.Query("d1", "temp", time.Time{}, time.Time{}, 10)
	if len(history) < 2 || history[len(history)-1].Value.Num != 35.1 {
		t.Fatalf("history = %+v", history)
	}
}

func TestRemoveChannelStopsPolling(t *testing.T) {
	bus := &fakeBus{coils: map[uint16]bool{}, holding: map[uint16]uint16{10: 100}, fail: map[byte]bool{}}
	service := NewService(nil, 5*time.Second, 3*time.Second, 100)
	defer service.Close()
	if err := service.AddChannel("c1", bus, []channel.Device{tempDevice()}, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(service.Channels()) == 1 })
	ch := service.Channels()[0]
	waitFor(t, func() bool { return ch.Stats("d1").Requests >= 2 })
	if !service.RemoveChannel("c1") {
		t.Fatal("channel was not removed")
	}
	requests := ch.Stats("d1").Requests
	time.Sleep(30 * time.Millisecond)
	if later := ch.Stats("d1").Requests; later > requests+1 {
		t.Fatalf("polling continued after removal: %d -> %d", requests, later)
	}
	if len(service.Channels()) != 0 {
		t.Fatal("channel list should be empty")
	}
}

func TestServiceRaisesOfflineAlarm(t *testing.T) {
	bus := &fakeBus{coils: map[uint16]bool{}, holding: map[uint16]uint16{}, fail: map[byte]bool{1: true}}
	service := NewService(nil, 5*time.Second, 3*time.Second, 100)
	defer service.Close()
	if err := service.AddChannel("c1", bus, []channel.Device{tempDevice()}, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		alarm, ok := service.Alarms.Get("device-offline:d1")
		return ok && alarm.Active
	})
}
