// Package runtime wires the semantic core to channels: periodic polling,
// latest-value state, command writes and readback confirmation.
package runtime

import (
	"context"
	"fmt"
	"time"

	"easyems/internal/ems/alarm"
	"easyems/internal/ems/channel"
	"easyems/internal/ems/command"
	"easyems/internal/ems/device"
	"easyems/internal/ems/signal"
	"easyems/internal/ems/state"
)

// Dispatcher queues a command's writes onto one channel.
type Dispatcher struct {
	Channel *channel.Channel
}

// Write implements command.Dispatcher.
func (d Dispatcher) Write(deviceID, requestID string, writes []device.Write) error {
	for _, write := range writes {
		if err := d.Channel.QueueWrite(deviceID, write.Point, write.Value, requestID); err != nil {
			return err
		}
	}
	return nil
}

// MultiDispatcher finds the channel that owns a device.
type MultiDispatcher struct {
	Channels []*channel.Channel
}

// Write implements command.Dispatcher across several channels.
func (d MultiDispatcher) Write(deviceID, requestID string, writes []device.Write) error {
	for _, ch := range d.Channels {
		if _, ok := ch.Device(deviceID); !ok {
			continue
		}
		for _, write := range writes {
			if err := ch.QueueWrite(deviceID, write.Point, write.Value, requestID); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown device %s", deviceID)
}

// Engine owns the channels, the latest-value store and command tracking. Call
// Cycle from one goroutine.
type Engine struct {
	Channels []*channel.Channel
	State    *state.Store
	Commands *command.Manager
	Alarms   *alarm.Engine
	MaxAge   time.Duration
	Period   time.Duration
	OnCycle  func(now time.Time)
}

// NewEngine builds the runtime for the given channels. samples for command
// readback come from the shared state store.
func NewEngine(channels []*channel.Channel, profiles func(string) (device.Profile, bool), maxAge, period, commandTimeout time.Duration) *Engine {
	store := state.New()
	manager := command.New(
		MultiDispatcher{Channels: channels},
		profiles,
		func(deviceID string, now time.Time) map[string]signal.Sample {
			return store.Samples(deviceID, now, maxAge)
		},
		maxAge, commandTimeout,
	)
	return &Engine{
		Channels: channels, State: store, Commands: manager, Alarms: alarm.New(),
		MaxAge: maxAge, Period: period,
	}
}

// Cycle polls every channel, updates state and advances commands.
func (e *Engine) Cycle(ctx context.Context, now time.Time) {
	for _, ch := range e.Channels {
		results, _ := ch.RunOnce(ctx, now)
		e.State.Apply(now, results)
		for _, entry := range ch.Devices {
			e.State.SetHealth(entry.ID, ch.Health(entry.ID, now, e.MaxAge), now)
		}
	}
	if e.Commands != nil {
		e.Commands.Tick(now)
	}
	e.updateAlarms(now)
	if e.OnCycle != nil {
		e.OnCycle(now)
	}
}

// updateAlarms turns communication health and command outcomes into alarm
// lifecycles. Repeating the same condition does not create new alarms.
func (e *Engine) updateAlarms(now time.Time) {
	if e.Alarms == nil {
		return
	}
	for _, ch := range e.Channels {
		for _, entry := range ch.Devices {
			offlineKey := "device-offline:" + entry.ID
			degradedKey := "device-degraded:" + entry.ID
			switch e.State.Health(entry.ID) {
			case channel.HealthOffline:
				stats := ch.Stats(entry.ID)
				_, _ = e.Alarms.Raise(now, offlineKey, alarm.Critical, "设备通信中断", stats.LastError, entry.ID, "")
				_, _ = e.Alarms.Clear(now, degradedKey)
			case channel.HealthDegraded:
				_, _ = e.Alarms.Raise(now, degradedKey, alarm.Warning, "设备通信不稳定", "", entry.ID, "")
				_, _ = e.Alarms.Clear(now, offlineKey)
			default:
				_, _ = e.Alarms.Clear(now, offlineKey)
				_, _ = e.Alarms.Clear(now, degradedKey)
			}
		}
	}
	if e.Commands == nil {
		return
	}
	for _, outcome := range e.Commands.List() {
		switch outcome.State {
		case command.Failed, command.Unknown:
			key := "command-" + string(outcome.State) + ":" + outcome.RequestID
			_, _ = e.Alarms.Raise(now, key, alarm.Warning, "控制结果异常", outcome.Reason, outcome.DeviceID, "")
		case command.Confirmed:
			_, _ = e.Alarms.Clear(now, "command-failed:"+outcome.RequestID)
			_, _ = e.Alarms.Clear(now, "command-unknown:"+outcome.RequestID)
		}
	}
}

// Run cycles immediately and then on the configured period until cancelled.
func (e *Engine) Run(ctx context.Context) error {
	period := e.Period
	if period <= 0 {
		period = time.Second
	}
	e.Cycle(ctx, time.Now())
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			e.Cycle(ctx, time.Now())
		}
	}
}
