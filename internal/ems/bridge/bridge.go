// Package bridge connects the product's persisted Modbus nodes to the EMS
// runtime: it builds channels from nodes, publishes read values back as points,
// applies valueSet changes as device writes and reports health to the nodes.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"fastiot/internal/ems/channel"
	"fastiot/internal/ems/runtime"
	"fastiot/internal/ems/signal"
)

// BusConfig is one channel derived from persisted nodes.
type BusConfig struct {
	ChannelID string
	Protocol  string // "tcp" or "rtu"
	Address   string // tcp "host:port"; rtu "port@baud"
	Period    time.Duration
	Devices   []channel.Device
}

// ValueSet is the desired value of one point and when it was requested.
type ValueSet struct {
	Value signal.Value
	At    time.Time
}

// Source reads configuration and writes observations back to product nodes.
type Source interface {
	Buses() ([]BusConfig, error)
	ValueSets() (map[string]ValueSet, error)
	PublishValue(ioNodeID string, value signal.Value) error
	PublishHealth(channelID string, lastGood time.Time, failures int) error
	PublishDeviceHealth(ioNodeID, health string) error
}

// BusOpener opens a channel transport.
type BusOpener func(protocol, address string, timeout time.Duration) (channel.Bus, error)

// Bridge keeps the runtime channels in sync with persisted nodes.
type Bridge struct {
	Service  *runtime.Service
	Source   Source
	OpenBus  BusOpener
	Timeout  time.Duration
	Interval time.Duration
	Log      func(error)

	mu      sync.Mutex
	applied map[string]string
	lastSet map[string]time.Time
	kinds   map[string]signal.Kind
	owners  map[string]owner
	health  map[string]string
}

type owner struct {
	ChannelID string
	DeviceID  string
}

// New builds a bridge and wires the runtime read hook.
func New(service *runtime.Service, source Source, open BusOpener) *Bridge {
	b := &Bridge{
		Service: service, Source: source, OpenBus: open,
		Timeout: 2 * time.Second, Interval: 2 * time.Second,
		applied: map[string]string{}, lastSet: map[string]time.Time{},
		kinds: map[string]signal.Kind{}, owners: map[string]owner{}, health: map[string]string{},
	}
	service.OnResults = func(_ string, results []channel.Result) {
		for _, result := range results {
			if err := source.PublishValue(result.PointID, result.Value); err != nil {
				b.report(err)
			}
		}
	}
	return b
}

// Run syncs immediately and then on the configured interval. Transient node
// errors are logged and retried; they do not stop the application.
func (b *Bridge) Run(ctx context.Context) error {
	interval := b.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if err := b.Sync(time.Now()); err != nil {
		b.report(err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := b.Sync(time.Now()); err != nil {
				b.report(err)
			}
		}
	}
}

// Restart drops the applied config marker for one channel and reconciles it
// again, reopening its transport without restarting the whole process.
func (b *Bridge) Restart(channelID string) error {
	b.mu.Lock()
	delete(b.applied, channelID)
	b.mu.Unlock()
	for _, ch := range b.Service.Channels() {
		if ch.ID == channelID {
			if closer, ok := ch.Bus.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
			break
		}
	}
	b.Service.RemoveChannel(channelID)
	return b.Sync(time.Now())
}

// Sync reconciles channels, applies pending valueSet changes and publishes
// health.
func (b *Bridge) Sync(now time.Time) error {
	buses, err := b.Source.Buses()
	if err != nil {
		return err
	}
	desired := make(map[string]BusConfig, len(buses))
	kinds := make(map[string]signal.Kind)
	owners := make(map[string]owner)
	for _, bus := range buses {
		desired[bus.ChannelID] = bus
		for _, device := range bus.Devices {
			for _, point := range device.Points {
				kinds[point.ID] = point.Kind
				owners[point.ID] = owner{ChannelID: bus.ChannelID, DeviceID: device.ID}
			}
		}
	}

	b.mu.Lock()
	for id, bus := range desired {
		hash := configHash(bus)
		if b.applied[id] == hash {
			continue
		}
		if _, exists := b.applied[id]; exists {
			b.Service.RemoveChannel(id)
		}
		transport, err := b.OpenBus(bus.Protocol, bus.Address, b.Timeout)
		if err != nil {
			b.report(fmt.Errorf("channel %s: open %s: %w", id, bus.Address, err))
			delete(b.applied, id)
			continue
		}
		if err := b.Service.AddChannel(id, transport, bus.Devices, bus.Period); err != nil {
			b.report(fmt.Errorf("channel %s: %w", id, err))
			delete(b.applied, id)
			continue
		}
		b.applied[id] = hash
	}
	for id := range b.applied {
		if _, ok := desired[id]; !ok {
			b.Service.RemoveChannel(id)
			delete(b.applied, id)
		}
	}
	b.kinds = kinds
	b.owners = owners
	b.mu.Unlock()

	sets, err := b.Source.ValueSets()
	if err != nil {
		return err
	}
	for ioID, set := range sets {
		b.mu.Lock()
		last, seen := b.lastSet[ioID]
		own, known := b.owners[ioID]
		kind := b.kinds[ioID]
		b.mu.Unlock()
		if !known || set.At.IsZero() {
			continue
		}
		if !seen {
			// The first observation is the baseline; never write a device just
			// because its stored valueSet happens to be non-zero.
			b.mu.Lock()
			b.lastSet[ioID] = set.At
			b.mu.Unlock()
			continue
		}
		if !last.Before(set.At) {
			continue
		}
		if err := b.queue(own, ioID, normalize(set.Value, kind)); err != nil {
			b.report(err)
			continue
		}
		b.mu.Lock()
		b.lastSet[ioID] = set.At
		b.mu.Unlock()
	}

	b.publishHealth()
	return nil
}

func (b *Bridge) queue(own owner, ioID string, value signal.Value) error {
	for _, ch := range b.Service.Channels() {
		if ch.ID != own.ChannelID {
			continue
		}
		if _, ok := ch.Device(own.DeviceID); !ok {
			return fmt.Errorf("channel %s has no device %s", own.ChannelID, own.DeviceID)
		}
		return ch.QueueWrite(own.DeviceID, ioID, value, "bridge:"+ioID+":"+time.Now().Format(time.RFC3339Nano))
	}
	return fmt.Errorf("channel %s is not running", own.ChannelID)
}

func (b *Bridge) publishHealth() {
	for _, ch := range b.Service.Channels() {
		b.mu.Lock()
		_, applied := b.applied[ch.ID]
		b.mu.Unlock()
		if !applied {
			continue
		}
		var last time.Time
		failures := 0
		for _, entry := range ch.Devices {
			stats := ch.Stats(entry.ID)
			if stats.LastSuccess.After(last) {
				last = stats.LastSuccess
			}
			failures += stats.ConsecutiveFailures
			health := string(ch.Health(entry.ID, time.Now(), 15*time.Second))
			b.mu.Lock()
			changed := b.health[entry.ID] != health
			if changed {
				b.health[entry.ID] = health
			}
			b.mu.Unlock()
			if changed {
				for _, point := range entry.Points {
					if err := b.Source.PublishDeviceHealth(point.ID, health); err != nil {
						b.report(err)
					}
				}
			}
		}
		if err := b.Source.PublishHealth(ch.ID, last, failures); err != nil {
			b.report(err)
		}
	}
}

func (b *Bridge) report(err error) {
	if b.Log != nil {
		b.Log(err)
	}
}

func normalize(value signal.Value, kind signal.Kind) signal.Value {
	if kind == signal.Bool && value.Kind == signal.Number {
		return signal.Flag(value.Num != 0)
	}
	if kind == signal.Number && value.Kind == signal.Bool {
		if value.Bool {
			return signal.Num(1)
		}
		return signal.Num(0)
	}
	return value
}

func configHash(bus BusConfig) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s|%s|%s|%d\n", bus.ChannelID, bus.Protocol, bus.Address, bus.Period)
	devices := append([]channel.Device(nil), bus.Devices...)
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
	for _, device := range devices {
		fmt.Fprintf(&builder, "D:%s:%d\n", device.ID, device.ServerID)
		points := append([]channel.Point(nil), device.Points...)
		sort.Slice(points, func(i, j int) bool { return points[i].ID < points[j].ID })
		for _, point := range points {
			readOnly := 0
			if point.ReadOnly {
				readOnly = 1
			}
			fmt.Fprintf(&builder, "P:%s:%s:%d:%s:%g:%g:%d:%d\n",
				point.ID, point.Function, point.Address, point.Format,
				point.Scale, point.Offset, point.Kind, readOnly)
		}
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:8])
}
