// Package channel runs one communication channel: it polls devices serially,
// groups contiguous registers into block reads, tracks per-device health and
// applies queued writes without letting one device break its neighbours.
package channel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"easyems/internal/ems/signal"
)

// Function is a Modbus data area.
type Function string

const (
	Coil            Function = "coil"
	DiscreteInput   Function = "discreteInput"
	HoldingRegister Function = "holdingRegister"
	InputRegister   Function = "inputRegister"
)

// Point is one readable or writable value on a device.
type Point struct {
	ID string
	// Name is the business name, used for display only.
	Name     string
	Function Function
	Address  uint16
	// Format applies to registers: uint16, int16, uint32, int32, float32.
	Format string
	// Scale and Offset convert raw to engineering units.
	Scale  float64
	Offset float64
	Kind   signal.Kind
	// ReadOnly points cannot be queued as writes.
	ReadOnly bool
}

// Size is the number of registers or bits a point occupies.
func (p Point) Size() uint16 {
	if p.Function == Coil || p.Function == DiscreteInput {
		return 1
	}
	switch p.Format {
	case "uint32", "int32", "float32":
		return 2
	default:
		return 1
	}
}

func (p Point) boolean() bool { return p.Function == Coil || p.Function == DiscreteInput }

// Device is one slave on the channel.
type Device struct {
	ID       string
	ServerID byte
	Points   []Point
}

func (d Device) point(id string) (Point, bool) {
	for _, point := range d.Points {
		if point.ID == id {
			return point, true
		}
	}
	return Point{}, false
}

// Bus is the transport used by one channel. Calls happen serially.
type Bus interface {
	ReadCoils(server byte, address, count uint16) ([]bool, error)
	ReadDiscreteInputs(server byte, address, count uint16) ([]bool, error)
	ReadHoldingRegisters(server byte, address, count uint16) ([]uint16, error)
	ReadInputRegisters(server byte, address, count uint16) ([]uint16, error)
	WriteCoil(server byte, address uint16, value bool) error
	WriteHoldingRegister(server byte, address uint16, value uint16) error
}

// Health is the state of one device on the channel.
type Health string

const (
	HealthUnknown  Health = "unknown"
	HealthHealthy  Health = "healthy"
	HealthDegraded Health = "degraded"
	HealthOffline  Health = "offline"
)

// Stats is the counters and the last error of one device.
type Stats struct {
	Requests            int
	Failures            int
	ConsecutiveFailures int
	LastSuccess         time.Time
	LastError           string
	LatencyCount        int
	LatencyTotalMS      float64
	LatencyMaxMS        float64
	LatencyLastMS       float64
}

// Result is one successfully read point.
type Result struct {
	DeviceID   string
	PointID    string
	Value      signal.Value
	Quality    signal.Quality
	SourceTime time.Time
}

// WriteResult is the outcome of one queued write. A failure means the device
// may or may not have applied it; the command manager decides the final state.
type WriteResult struct {
	DeviceID  string
	PointID   string
	RequestID string
	Value     signal.Value
	Error     string
}

type pendingWrite struct {
	deviceID  string
	pointID   string
	value     signal.Value
	requestID string
}

type deviceState struct {
	stats Stats
	// buckets hold one-minute counters for sliding 5m/1h quality windows.
	buckets [60]minuteBucket
}

type minuteBucket struct {
	minute     int64 // unix minute
	requests   int
	failures   int
	latencySum float64
	latencyMax float64
	errors     map[string]int
}

// WindowStats is the quality summary of one device over a recent window.
type WindowStats struct {
	Minutes      int            `json:"minutes"`
	Polls        int            `json:"polls"`
	Failures     int            `json:"failures"`
	SuccessRate  float64        `json:"success_rate"`
	AvgLatencyMS float64        `json:"avg_latency_ms"`
	MaxLatencyMS float64        `json:"max_latency_ms"`
	Errors       map[string]int `json:"errors,omitempty"`
}

// Channel is one serial communication path. Call RunOnce from a single
// goroutine; QueueWrite may be called from other goroutines.
type Channel struct {
	ID           string
	Bus          Bus
	Devices      []Device
	MaxGap       uint16
	MaxRegisters uint16
	OfflineAfter int

	mu      sync.Mutex
	pending []pendingWrite
	states  map[string]*deviceState
	index   map[string]*Device
}

// New validates devices and returns a channel.
func New(id string, bus Bus, devices []Device) (*Channel, error) {
	if id == "" {
		return nil, errors.New("channel id is empty")
	}
	if bus == nil {
		return nil, errors.New("channel bus is nil")
	}
	index := make(map[string]*Device, len(devices))
	for i := range devices {
		device := &devices[i]
		if device.ID == "" {
			return nil, errors.New("device id is empty")
		}
		if _, exists := index[device.ID]; exists {
			return nil, fmt.Errorf("duplicate device %s", device.ID)
		}
		for _, point := range device.Points {
			if point.ID == "" {
				return nil, fmt.Errorf("device %s: point without id", device.ID)
			}
			if point.boolean() && point.Kind != signal.Bool {
				return nil, fmt.Errorf("point %s: bit area requires a bool kind", point.ID)
			}
			if !point.boolean() && point.Kind != signal.Number {
				return nil, fmt.Errorf("point %s: register requires a number kind", point.ID)
			}
			if point.Function != Coil && point.Function != DiscreteInput &&
				point.Function != HoldingRegister && point.Function != InputRegister {
				return nil, fmt.Errorf("point %s: unknown function %q", point.ID, point.Function)
			}
		}
		index[device.ID] = device
	}
	return &Channel{
		ID: id, Bus: bus, Devices: devices,
		MaxGap: 0, MaxRegisters: 120, OfflineAfter: 3,
		states: make(map[string]*deviceState, len(devices)),
		index:  index,
	}, nil
}

// Device returns a copy of one device on the channel.
func (c *Channel) Device(id string) (Device, bool) {
	device, ok := c.index[id]
	if !ok {
		return Device{}, false
	}
	return *device, true
}

// QueueWrite schedules a write for the next RunOnce. It rejects unknown,
// read-only or kind-mismatched targets.
func (c *Channel) QueueWrite(deviceID, pointID string, value signal.Value, requestID string) error {
	device, ok := c.index[deviceID]
	if !ok {
		return fmt.Errorf("unknown device %s", deviceID)
	}
	point, ok := device.point(pointID)
	if !ok {
		return fmt.Errorf("device %s has no point %s", deviceID, pointID)
	}
	if point.ReadOnly {
		return fmt.Errorf("point %s is read-only", pointID)
	}
	if point.Kind != value.Kind {
		return fmt.Errorf("point %s expects %s but got %s", pointID, point.Kind, value.Kind)
	}
	if _, _, err := encode(point, value); err != nil {
		return err
	}
	c.mu.Lock()
	c.pending = append(c.pending, pendingWrite{deviceID, pointID, value, requestID})
	c.mu.Unlock()
	return nil
}

// RunOnce applies queued writes, then polls every device. A failing device is
// recorded and skipped for the rest of this run; other devices still run.
func (c *Channel) RunOnce(ctx context.Context, now time.Time) ([]Result, []WriteResult) {
	writes := c.drain()
	results := make([]Result, 0)
	writeResults := make([]WriteResult, 0, len(writes))
	for _, write := range writes {
		if err := ctx.Err(); err != nil {
			break
		}
		device := c.index[write.deviceID]
		point, _ := device.point(write.pointID)
		result := WriteResult{DeviceID: write.deviceID, PointID: write.pointID, RequestID: write.requestID, Value: write.value}
		if err := c.write(device.ServerID, point, write.value); err != nil {
			result.Error = err.Error()
			c.recordFailure(write.deviceID, err, now)
		} else {
			c.recordSuccess(write.deviceID, now)
		}
		writeResults = append(writeResults, result)
	}

	for i := range c.Devices {
		if err := ctx.Err(); err != nil {
			break
		}
		device := &c.Devices[i]
		deviceOK := true
		for _, block := range buildBlocks(device.Points, c.MaxGap, c.MaxRegisters) {
			if err := ctx.Err(); err != nil {
				break
			}
			started := time.Now()
			bits, words, err := c.read(device.ServerID, block)
			c.recordLatency(device.ID, time.Since(started))
			if err != nil {
				deviceOK = false
				c.recordFailure(device.ID, err, now)
				continue
			}
			for _, index := range block.points {
				point := device.Points[index]
				value, err := decode(point, block.address, bits, words)
				if err != nil {
					deviceOK = false
					c.recordFailure(device.ID, err, now)
					continue
				}
				results = append(results, Result{
					DeviceID: device.ID, PointID: point.ID,
					Value: value, Quality: signal.Good, SourceTime: now,
				})
			}
		}
		if deviceOK {
			c.recordSuccess(device.ID, now)
		}
	}
	return results, writeResults
}

func (c *Channel) drain() []pendingWrite {
	c.mu.Lock()
	defer c.mu.Unlock()
	writes := c.pending
	c.pending = nil
	return writes
}

func (c *Channel) read(server byte, block block) ([]bool, []uint16, error) {
	switch block.function {
	case Coil:
		bits, err := c.Bus.ReadCoils(server, block.address, block.count)
		return bits, nil, err
	case DiscreteInput:
		bits, err := c.Bus.ReadDiscreteInputs(server, block.address, block.count)
		return bits, nil, err
	case HoldingRegister:
		words, err := c.Bus.ReadHoldingRegisters(server, block.address, block.count)
		return nil, words, err
	case InputRegister:
		words, err := c.Bus.ReadInputRegisters(server, block.address, block.count)
		return nil, words, err
	default:
		return nil, nil, fmt.Errorf("unknown function %q", block.function)
	}
}

func (c *Channel) write(server byte, point Point, value signal.Value) error {
	coil, words, err := encode(point, value)
	if err != nil {
		return err
	}
	if point.boolean() {
		return c.Bus.WriteCoil(server, point.Address, coil)
	}
	for index, word := range words {
		if err := c.Bus.WriteHoldingRegister(server, point.Address+uint16(index), word); err != nil {
			return err
		}
	}
	return nil
}

func (c *Channel) recordLatency(deviceID string, elapsed time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state(deviceID)
	ms := float64(elapsed.Microseconds()) / 1000
	state.stats.LatencyCount++
	state.stats.LatencyTotalMS += ms
	state.stats.LatencyLastMS = ms
	if ms > state.stats.LatencyMaxMS {
		state.stats.LatencyMaxMS = ms
	}
	bucket := state.bucket(time.Now())
	bucket.requests++
	bucket.latencySum += ms
	if ms > bucket.latencyMax {
		bucket.latencyMax = ms
	}
}

func (c *Channel) recordSuccess(deviceID string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state(deviceID)
	state.stats.Requests++
	state.stats.ConsecutiveFailures = 0
	state.stats.LastSuccess = now
	state.stats.LastError = ""
}

func (c *Channel) recordFailure(deviceID string, err error, _ time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state(deviceID)
	state.stats.Requests++
	state.stats.Failures++
	state.stats.ConsecutiveFailures++
	state.stats.LastError = err.Error()
	bucket := state.bucket(time.Now())
	bucket.requests++
	bucket.failures++
	if bucket.errors == nil {
		bucket.errors = map[string]int{}
	}
	bucket.errors[classifyError(err)]++
}

// classifyError groups transport errors for the quality panel.
func classifyError(err error) string {
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "timeout") || strings.Contains(text, "deadline"):
		return "timeout"
	case strings.Contains(text, "connect") || strings.Contains(text, "refused") || strings.Contains(text, "closed") || strings.Contains(text, "broken pipe"):
		return "disconnect"
	case strings.Contains(text, "crc"):
		return "crc"
	case strings.Contains(text, "exception") || strings.Contains(text, "illegal"):
		return "exception"
	case strings.Contains(text, "write"):
		return "write"
	default:
		return "other"
	}
}

func (c *Channel) state(deviceID string) *deviceState {
	state, ok := c.states[deviceID]
	if !ok {
		state = &deviceState{}
		c.states[deviceID] = state
	}
	return state
}

// bucket returns the current minute bucket, resetting it when the minute
// changes so old counters never leak into the new window.
func (s *deviceState) bucket(now time.Time) *minuteBucket {
	minute := now.Unix() / 60
	slot := int(minute % 60)
	bucket := &s.buckets[slot]
	if bucket.minute != minute {
		*bucket = minuteBucket{minute: minute}
	}
	return bucket
}

// Window summarizes the last minutes of one device from the minute buckets.
func (c *Channel) Window(deviceID string, now time.Time, minutes int) WindowStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := WindowStats{Minutes: minutes, Errors: map[string]int{}}
	state, ok := c.states[deviceID]
	if !ok {
		return result
	}
	oldest := now.Add(-time.Duration(minutes)*time.Minute).Unix() / 60
	latencyCount := 0
	for index := range state.buckets {
		bucket := state.buckets[index]
		if bucket.minute == 0 || bucket.minute < oldest {
			continue
		}
		result.Polls += bucket.requests
		result.Failures += bucket.failures
		if bucket.latencyMax > result.MaxLatencyMS {
			result.MaxLatencyMS = bucket.latencyMax
		}
		result.AvgLatencyMS += bucket.latencySum
		latencyCount += bucket.requests - bucket.failures
		for name, count := range bucket.errors {
			result.Errors[name] += count
		}
	}
	if latencyCount > 0 {
		result.AvgLatencyMS = result.AvgLatencyMS / float64(latencyCount)
	}
	if result.Polls > 0 {
		result.SuccessRate = float64(result.Polls-result.Failures) / float64(result.Polls) * 100
	}
	return result
}

// Stats returns a copy of the counters for one device.
func (c *Channel) Stats(deviceID string) Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state, ok := c.states[deviceID]; ok {
		return state.stats
	}
	return Stats{}
}

// Health derives the device state from its last success and consecutive
// failures. A stale success also means offline, so a silent port cannot look
// healthy forever.
func (c *Channel) Health(deviceID string, now time.Time, maxAge time.Duration) Health {
	c.mu.Lock()
	state, ok := c.states[deviceID]
	var stats Stats
	if ok {
		stats = state.stats
	}
	offlineAfter := c.OfflineAfter
	if offlineAfter <= 0 {
		offlineAfter = 3
	}
	c.mu.Unlock()

	if stats.Requests == 0 {
		return HealthUnknown
	}
	if stats.ConsecutiveFailures >= offlineAfter {
		return HealthOffline
	}
	if stats.LastSuccess.IsZero() {
		return HealthDegraded
	}
	if maxAge > 0 && now.Sub(stats.LastSuccess) > maxAge {
		return HealthOffline
	}
	if stats.ConsecutiveFailures > 0 {
		return HealthDegraded
	}
	return HealthHealthy
}

type block struct {
	function Function
	address  uint16
	count    uint16
	points   []int
}

// buildBlocks groups points of the same function into contiguous reads, so a
// device is not polled one register at a time.
func buildBlocks(points []Point, maxGap, maxRegisters uint16) []block {
	if maxRegisters == 0 {
		maxRegisters = 120
	}
	byFunction := make(map[Function][]int)
	for index, point := range points {
		byFunction[point.Function] = append(byFunction[point.Function], index)
	}
	functions := make([]Function, 0, len(byFunction))
	for function := range byFunction {
		functions = append(functions, function)
	}
	sort.Slice(functions, func(i, j int) bool { return functions[i] < functions[j] })

	blocks := make([]block, 0)
	for _, function := range functions {
		indices := byFunction[function]
		sort.Slice(indices, func(i, j int) bool {
			return points[indices[i]].Address < points[indices[j]].Address
		})
		current := block{function: function}
		end := uint16(0)
		for _, index := range indices {
			point := points[index]
			size := point.Size()
			if current.count == 0 {
				current = block{function: function, address: point.Address, count: size, points: []int{index}}
				end = point.Address + size
				continue
			}
			gap := point.Address - end
			if point.Address < end || gap > maxGap || current.count+size > maxRegisters {
				blocks = append(blocks, current)
				current = block{function: function, address: point.Address, count: size, points: []int{index}}
				end = point.Address + size
				continue
			}
			current.count = point.Address + size - current.address
			current.points = append(current.points, index)
			end = point.Address + size
		}
		if current.count > 0 {
			blocks = append(blocks, current)
		}
	}
	return blocks
}
