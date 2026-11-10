// Package opcua 提供 OPC-UA 设备接入：连接、读点、写点与源时间戳。
// 复用 gopcua 库，作为多协议驱动（Modbus/MQTT/OPC-UA）的 OPC-UA 基础。
package opcua

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"

	"easyems/internal/ems/signal"
)

// ParseNodeID parses a full OPC-UA node id such as ns=3;s=Tag or i=2259.
func ParseNodeID(text string) (*ua.NodeID, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("opcua node id is empty")
	}
	valid := false
	for _, segment := range strings.Split(trimmed, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(segment), "=")
		if !found || value == "" {
			return nil, fmt.Errorf("opcua node id %q has an invalid segment %q", text, segment)
		}
		switch key {
		case "ns":
			if _, err := strconv.Atoi(value); err != nil {
				return nil, fmt.Errorf("opcua node id %q has an invalid namespace", text)
			}
		case "i", "s", "g", "b":
			valid = true
		default:
			return nil, fmt.Errorf("opcua node id %q has an unknown identifier %q", text, key)
		}
	}
	if !valid {
		return nil, fmt.Errorf("opcua node id %q has no identifier", text)
	}
	id, err := ua.ParseNodeID(trimmed)
	if err != nil {
		return nil, fmt.Errorf("opcua node id %q: %w", text, err)
	}
	return id, nil
}

// NodeValue 是一次 OPC-UA 读取结果：值、UA 状态与源时间。
type NodeValue struct {
	Value  signal.Value
	Status string
	At     time.Time
}

// Client 是一个最小 OPC-UA 设备连接。
type Client struct {
	endpoint string
	conn     *opcua.Client
}

// NewClient 创建 OPC-UA 客户端（如 opc.tcp://host:4840）。
func NewClient(endpoint string) *Client {
	return &Client{endpoint: endpoint}
}

// Connect 建立到 OPC-UA server 的连接。
func (c *Client) Connect(ctx context.Context) error {
	conn, err := opcua.NewClient(c.endpoint)
	if err != nil {
		return fmt.Errorf("opcua new client %s: %w", c.endpoint, err)
	}
	if err := conn.Connect(ctx); err != nil {
		return fmt.Errorf("opcua connect %s: %w", c.endpoint, err)
	}
	c.conn = conn
	return nil
}

// Close 关闭连接。
func (c *Client) Close(ctx context.Context) {
	if c.conn != nil {
		c.conn.Close(ctx)
	}
}

// Read 读取一个节点的当前值。nodeID 支持完整写法（ns=3;s=Tag、i=2259）。
func (c *Client) Read(ctx context.Context, nodeID string) (NodeValue, error) {
	if c.conn == nil {
		return NodeValue{}, fmt.Errorf("opcua not connected")
	}
	id, err := ParseNodeID(nodeID)
	if err != nil {
		return NodeValue{}, err
	}
	response, err := c.conn.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{{NodeID: id, AttributeID: ua.AttributeIDValue}},
	})
	if err != nil {
		return NodeValue{}, err
	}
	if len(response.Results) == 0 {
		return NodeValue{}, fmt.Errorf("no result for %s", nodeID)
	}
	result := response.Results[0]
	if result.Status != ua.StatusOK {
		return NodeValue{}, fmt.Errorf("opcua read %s status %v", nodeID, result.Status)
	}
	at := time.Now()
	if !result.SourceTimestamp.IsZero() {
		at = result.SourceTimestamp
	}
	if result.Value == nil || result.Value.Value() == nil {
		return NodeValue{}, fmt.Errorf("opcua read %s returned no value", nodeID)
	}
	switch typed := result.Value.Value().(type) {
	case float64:
		return NodeValue{Value: signal.Num(typed), Status: "good", At: at}, nil
	case float32:
		return NodeValue{Value: signal.Num(float64(typed)), Status: "good", At: at}, nil
	case int64:
		return NodeValue{Value: signal.Num(float64(typed)), Status: "good", At: at}, nil
	case int32:
		return NodeValue{Value: signal.Num(float64(typed)), Status: "good", At: at}, nil
	case uint32:
		return NodeValue{Value: signal.Num(float64(typed)), Status: "good", At: at}, nil
	case uint16:
		return NodeValue{Value: signal.Num(float64(typed)), Status: "good", At: at}, nil
	case bool:
		return NodeValue{Value: signal.Flag(typed), Status: "good", At: at}, nil
	default:
		return NodeValue{}, fmt.Errorf("opcua read %s unsupported type %T", nodeID, result.Value.Value())
	}
}

// Write 写一个节点值（数值或开关）。
func (c *Client) Write(ctx context.Context, nodeID string, value signal.Value) error {
	if c.conn == nil {
		return fmt.Errorf("opcua not connected")
	}
	id, err := ParseNodeID(nodeID)
	if err != nil {
		return err
	}
	var raw any
	switch value.Kind {
	case signal.Number:
		raw = value.Num
	case signal.Bool:
		raw = value.Bool
	default:
		return fmt.Errorf("opcua write %s unsupported kind %s", nodeID, value.Kind)
	}
	variant, err := ua.NewVariant(raw)
	if err != nil {
		return err
	}
	response, err := c.conn.Write(ctx, &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{{
			NodeID: id, AttributeID: ua.AttributeIDValue, Value: &ua.DataValue{Value: variant},
		}},
	})
	if err != nil {
		return err
	}
	for _, result := range response.Results {
		if result != ua.StatusOK {
			return fmt.Errorf("opcua write %s status %v", nodeID, result)
		}
	}
	return nil
}

// Endpoint 返回配置的端点，供健康信息展示（不含凭据）。
func (c *Client) Endpoint() string { return c.endpoint }
