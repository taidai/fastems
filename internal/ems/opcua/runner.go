package opcua

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"easyems/internal/ems/device"
	"easyems/internal/ems/signal"
)

// Connection is the narrow OPC-UA surface the runner needs; Client implements
// it and tests can use a fake.
type Connection interface {
	Read(ctx context.Context, nodeID string) (NodeValue, error)
	Write(ctx context.Context, nodeID string, value signal.Value) error
	Close(ctx context.Context)
}

// Dialer opens a connection to an endpoint.
type Dialer func(endpoint string) Connection

// PointMapping maps one product point node to an OPC-UA node id.
type PointMapping struct {
	PointID string
	NodeID  string
}

// ResultValue is one successful read bound to its product point.
type ResultValue struct {
	PointID string
	Value   signal.Value
	At      time.Time
}

// Runner polls one OPC-UA endpoint and serves writes for the same points.
// It reconnects with a bounded backoff and reports health changes.
type Runner struct {
	Endpoint string
	DeviceID string // runtime device id used by the command manager
	Points   []PointMapping
	Interval time.Duration
	Dial     Dialer
	Observe  func(deviceID string, values []ResultValue)
	OnHealth func(online bool, reason string)

	mu   sync.Mutex
	conn Connection
}

// NewRunner builds a runner with a default dialer.
func NewRunner(endpoint, deviceID string, points []PointMapping) *Runner {
	return &Runner{
		Endpoint: endpoint, DeviceID: deviceID, Points: points,
		Interval: 2 * time.Second,
		Dial:     func(endpoint string) Connection { return NewClient(endpoint) },
	}
}

// Run polls until the context is canceled, reconnecting with backoff.
func (r *Runner) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		conn := r.Dial(r.Endpoint)
		if err := connect(ctx, conn); err != nil {
			r.reportHealth(false, err.Error())
			if !sleep(ctx, backoff) {
				return nil
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		r.setConn(conn)
		r.reportHealth(true, "")
		backoff = time.Second
		for ctx.Err() == nil {
			if err := r.poll(ctx, conn); err != nil {
				break
			}
			if !sleep(ctx, r.Interval) {
				return nil
			}
		}
		r.setConn(nil)
		conn.Close(context.Background())
		r.reportHealth(false, "opcua connection lost")
	}
}

// connector is implemented by Client; fake connections in tests may not need
// an explicit connect step.
type connector interface {
	Connect(ctx context.Context) error
}

func connect(ctx context.Context, conn Connection) error {
	if client, ok := conn.(connector); ok {
		return client.Connect(ctx)
	}
	return nil
}

func (r *Runner) poll(ctx context.Context, conn Connection) error {
	values := make([]ResultValue, 0, len(r.Points))
	var lastErr error
	for _, point := range r.Points {
		if point.NodeID == "" || point.PointID == "" {
			continue
		}
		value, err := conn.Read(ctx, point.NodeID)
		if err != nil {
			lastErr = err
			continue
		}
		values = append(values, ResultValue{PointID: point.PointID, Value: value.Value, At: value.At})
	}
	if len(values) > 0 && r.Observe != nil {
		r.Observe(r.DeviceID, values)
	}
	if lastErr != nil && len(values) == 0 {
		return lastErr
	}
	return nil
}

// Write implements command.Dispatcher: it maps a product point to its OPC-UA
// node and writes it. It never retries internally.
func (r *Runner) Write(_ string, _ string, writes []device.Write) error {
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	if conn == nil {
		return errors.New("opcua not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, write := range writes {
		nodeID := ""
		for _, point := range r.Points {
			if point.PointID == write.Point {
				nodeID = point.NodeID
				break
			}
		}
		if nodeID == "" {
			return fmt.Errorf("opcua point %s is not mapped", write.Point)
		}
		if err := conn.Write(ctx, nodeID, write.Value); err != nil {
			return err
		}
	}
	return nil
}

// PointIDs returns the mapped product point ids in a stable order.
func (r *Runner) PointIDs() []string {
	ids := make([]string, 0, len(r.Points))
	for _, point := range r.Points {
		ids = append(ids, point.PointID)
	}
	sort.Strings(ids)
	return ids
}

func (r *Runner) setConn(conn Connection) {
	r.mu.Lock()
	r.conn = conn
	r.mu.Unlock()
}

func (r *Runner) reportHealth(online bool, reason string) {
	if r.OnHealth != nil {
		r.OnHealth(online, reason)
	}
}

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
