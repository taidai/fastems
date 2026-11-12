package opcua

import (
	"context"
	"errors"
	"testing"
	"time"

	"fastems/internal/ems/device"
	"fastems/internal/ems/signal"
)

func TestParseNodeIDFormats(t *testing.T) {
	for _, text := range []string{"ns=2;s=MyVar", "ns=3;i=1001", "i=2259", "ns=1;g=7c6dd6f8-2b0a-4a4e-a1f0-6b1f5f9c9f00"} {
		if _, err := ParseNodeID(text); err != nil {
			t.Fatalf("ParseNodeID(%q): %v", text, err)
		}
	}
	for _, text := range []string{"", "not-a-node"} {
		if _, err := ParseNodeID(text); err == nil {
			t.Fatalf("ParseNodeID(%q) must fail", text)
		}
	}
}

type fakeConn struct {
	values   map[string]NodeValue
	readErr  error
	writeErr error
	writes   map[string]signal.Value
}

func (f *fakeConn) Read(_ context.Context, nodeID string) (NodeValue, error) {
	if f.readErr != nil {
		return NodeValue{}, f.readErr
	}
	value, ok := f.values[nodeID]
	if !ok {
		return NodeValue{}, errors.New("not found")
	}
	return value, nil
}

func (f *fakeConn) Write(_ context.Context, nodeID string, value signal.Value) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.writes == nil {
		f.writes = map[string]signal.Value{}
	}
	f.writes[nodeID] = value
	return nil
}

func (f *fakeConn) Close(context.Context) {}

func TestRunnerPollsAndWritesMappedPoints(t *testing.T) {
	conn := &fakeConn{values: map[string]NodeValue{
		"ns=2;s=SOC":  {Value: signal.Num(57), At: time.Now()},
		"ns=2;s=Temp": {Value: signal.Num(31), At: time.Now()},
	}}
	runner := NewRunner("opc.tcp://test:4840", "opc-1", []PointMapping{
		{PointID: "soc-point", NodeID: "ns=2;s=SOC"},
		{PointID: "temp-point", NodeID: "ns=2;s=Temp"},
	})
	runner.Dial = func(string) Connection { return conn }
	observed := []ResultValue{}
	runner.Observe = func(_ string, values []ResultValue) { observed = append(observed, values...) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(observed) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(observed) < 2 {
		t.Fatalf("expected two observations, got %+v", observed)
	}

	// 等待运行器持有连接后写入。
	for time.Now().Before(deadline) {
		if err := runner.Write("opc-1", "req", []device.Write{{Point: "soc-point", Value: signal.Num(80)}}); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if conn.writes["ns=2;s=SOC"].Num != 80 {
		t.Fatalf("write not mapped: %+v", conn.writes)
	}
	if err := runner.Write("opc-1", "req2", []device.Write{{Point: "unmapped", Value: signal.Num(1)}}); err == nil {
		t.Fatal("unmapped point must be rejected")
	}
}

func TestRunnerReportsHealthAndReconnects(t *testing.T) {
	failing := &fakeConn{readErr: errors.New("timeout")}
	healthy := &fakeConn{values: map[string]NodeValue{"ns=2;s=SOC": {Value: signal.Num(40), At: time.Now()}}}
	dials := 0
	runner := NewRunner("opc.tcp://test:4840", "opc-1", []PointMapping{{PointID: "soc-point", NodeID: "ns=2;s=SOC"}})
	runner.Dial = func(string) Connection {
		dials++
		if dials == 1 {
			return failing
		}
		return healthy
	}
	runner.Interval = 10 * time.Millisecond
	health := []bool{}
	runner.OnHealth = func(online bool, _ string) { health = append(health, online) }
	observed := 0
	runner.Observe = func(string, []ResultValue) { observed++ }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()
	deadline := time.Now().Add(3 * time.Second)
	for (observed == 0 || len(health) < 2) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if observed == 0 {
		t.Fatal("runner should read after reconnecting")
	}
	if len(health) < 2 || !health[0] || health[1] {
		t.Fatalf("health sequence = %v", health)
	}
}
