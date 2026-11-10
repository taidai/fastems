package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"easyems/internal/ems/alarm"
	"easyems/internal/ems/channel"
	"easyems/internal/ems/command"
	"easyems/internal/ems/device"
	"easyems/internal/ems/history"
	"easyems/internal/ems/signal"
	"easyems/internal/ems/state"
)

// Service is the long-running runtime: channels come and go, state and history
// are shared, commands and alarms advance on a fixed tick.
type Service struct {
	State   *state.Store
	Alarms  *alarm.Engine
	History *history.Store
	MaxAge  time.Duration
	// OnResults is called after each channel cycle with the values read. The
	// bridge uses it to publish values back to product nodes.
	OnResults func(deviceID string, results []channel.Result)

	mu       sync.Mutex
	channels map[string]*channel.Channel
	cancels  map[string]context.CancelFunc
	commands *command.Manager
	// extra maps runtime device IDs of non-Modbus adapters (OPC-UA, MQTT) to
	// their dispatcher. It is consulted before channel routing.
	extra  map[string]command.Dispatcher
	closed bool
}

// NewService builds an empty service. profiles resolves device capabilities for
// commands; pass a function returning false when commands are not configured.
func NewService(profiles func(string) (device.Profile, bool), maxAge, commandTimeout time.Duration, historyLimit int) *Service {
	service := &Service{
		State:    state.New(),
		Alarms:   alarm.New(),
		History:  history.New(historyLimit),
		MaxAge:   maxAge,
		channels: make(map[string]*channel.Channel),
		cancels:  make(map[string]context.CancelFunc),
		extra:    make(map[string]command.Dispatcher),
	}
	if profiles == nil {
		profiles = func(string) (device.Profile, bool) { return device.Profile{}, false }
	}
	service.commands = command.New(
		serviceDispatcher{service: service},
		profiles,
		func(deviceID string, now time.Time) map[string]signal.Sample {
			return service.State.Samples(deviceID, now, maxAge)
		},
		maxAge, commandTimeout,
	)
	return service
}

// SetHistoryLimit changes the per-point history cap at runtime.
func (s *Service) SetHistoryLimit(limit int) {
	if s.History != nil {
		s.History.SetLimit(limit)
	}
}

// Commands exposes command submission and tracking.
func (s *Service) Commands() *command.Manager { return s.commands }

// RegisterDispatcher routes writes for one runtime device ID to a protocol
// adapter (OPC-UA, MQTT). It takes precedence over Modbus channel routing and
// uses the same command.Manager.
func (s *Service) RegisterDispatcher(deviceID string, dispatcher command.Dispatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.extra == nil {
		s.extra = make(map[string]command.Dispatcher)
	}
	s.extra[deviceID] = dispatcher
}

// UnregisterDispatcher removes a protocol adapter.
func (s *Service) UnregisterDispatcher(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.extra, deviceID)
}

// Observe feeds real protocol observations (OPC-UA/MQTT) into the shared
// state, history and OnResults path, so command readback can confirm them.
func (s *Service) Observe(now time.Time, results []channel.Result) {
	if len(results) == 0 {
		return
	}
	s.State.Apply(now, results)
	s.History.Record(toHistory(results))
	if s.OnResults != nil {
		s.OnResults("", results)
	}
}

// AddChannel validates and starts one channel runner.
func (s *Service) AddChannel(id string, bus channel.Bus, devices []channel.Device, period time.Duration) error {
	ch, err := channel.New(id, bus, devices)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("service is closed")
	}
	if _, exists := s.channels[id]; exists {
		return fmt.Errorf("channel %s already exists", id)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.channels[id] = ch
	s.cancels[id] = cancel
	for _, entry := range devices {
		s.State.SetHealth(entry.ID, channel.HealthUnknown, time.Now())
	}
	go s.runChannel(ctx, ch, period)
	return nil
}

// RemoveChannel stops one channel runner.
func (s *Service) RemoveChannel(id string) bool {
	s.mu.Lock()
	cancel, ok := s.cancels[id]
	if ok {
		delete(s.cancels, id)
		delete(s.channels, id)
	}
	s.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// Channels returns the configured channels in id order.
func (s *Service) Channels() []*channel.Channel {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*channel.Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		list = append(list, ch)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// Snapshot returns the latest values with effective quality.
func (s *Service) Snapshot() []state.Value {
	return s.State.Snapshot(time.Now(), s.MaxAge)
}

// Run advances command tracking and command alarms until the context ends.
func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Close()
			return ctx.Err()
		case <-ticker.C:
			now := time.Now()
			s.commands.Tick(now)
			s.updateCommandAlarms(now)
		}
	}
}

// Close stops every channel runner.
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for id, cancel := range s.cancels {
		cancels = append(cancels, cancel)
		delete(s.cancels, id)
		delete(s.channels, id)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Service) runChannel(ctx context.Context, ch *channel.Channel, period time.Duration) {
	runner := &channel.Runner{
		Channel: ch,
		Period:  period,
		OnCycle: func(now time.Time, results []channel.Result, _ []channel.WriteResult) {
			s.State.Apply(now, results)
			s.History.Record(toHistory(results))
			for _, entry := range ch.Devices {
				s.State.SetHealth(entry.ID, ch.Health(entry.ID, now, s.MaxAge), now)
			}
			s.updateDeviceAlarms(now, ch)
			if s.OnResults != nil && len(results) > 0 {
				s.OnResults(ch.ID, results)
			}
		},
	}
	_ = runner.Run(ctx)
}

func (s *Service) updateDeviceAlarms(now time.Time, ch *channel.Channel) {
	for _, entry := range ch.Devices {
		offlineKey := "device-offline:" + entry.ID
		degradedKey := "device-degraded:" + entry.ID
		switch s.State.Health(entry.ID) {
		case channel.HealthOffline:
			_, _ = s.Alarms.Raise(now, offlineKey, alarm.Critical, "设备通信中断", ch.Stats(entry.ID).LastError, entry.ID, "")
			_, _ = s.Alarms.Clear(now, degradedKey)
		case channel.HealthDegraded:
			_, _ = s.Alarms.Raise(now, degradedKey, alarm.Warning, "设备通信不稳定", "", entry.ID, "")
			_, _ = s.Alarms.Clear(now, offlineKey)
		default:
			_, _ = s.Alarms.Clear(now, offlineKey)
			_, _ = s.Alarms.Clear(now, degradedKey)
		}
	}
}

func (s *Service) updateCommandAlarms(now time.Time) {
	for _, outcome := range s.commands.List() {
		if outcome.Restored {
			continue
		}
		switch outcome.State {
		case command.Failed, command.Unknown:
			key := "command-" + string(outcome.State) + ":" + outcome.RequestID
			_, _ = s.Alarms.Raise(now, key, alarm.Warning, "控制结果异常", outcome.Reason, outcome.DeviceID, "")
		case command.Confirmed:
			_, _ = s.Alarms.Clear(now, "command-failed:"+outcome.RequestID)
			_, _ = s.Alarms.Clear(now, "command-unknown:"+outcome.RequestID)
		}
	}
}

func toHistory(results []channel.Result) []history.Sample {
	samples := make([]history.Sample, 0, len(results))
	for _, result := range results {
		samples = append(samples, history.Sample{
			DeviceID: result.DeviceID, PointID: result.PointID,
			Value: result.Value, Quality: result.Quality,
			SourceTime: result.SourceTime, ReceivedTime: time.Now(),
		})
	}
	return samples
}

type serviceDispatcher struct{ service *Service }

func (d serviceDispatcher) Write(deviceID, requestID string, writes []device.Write) error {
	d.service.mu.Lock()
	extra := d.service.extra[deviceID]
	channels := make([]*channel.Channel, 0, len(d.service.channels))
	for _, ch := range d.service.channels {
		channels = append(channels, ch)
	}
	d.service.mu.Unlock()
	if extra != nil {
		return extra.Write(deviceID, requestID, writes)
	}
	for _, ch := range channels {
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
