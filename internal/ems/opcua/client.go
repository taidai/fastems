// Package opcua 提供最小可用的 OPC-UA 设备接入：连接、读点、写点。
// 复用 gopcua 库，作为多协议驱动（Modbus/MQTT/OPC-UA）的 OPC-UA 基础。
package opcua

import (
	"context"
	"fmt"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

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

// Read 读取一个节点的当前值（如 ns=2;s=MyVar）。
func (c *Client) Read(ctx context.Context, nodeID string) (any, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("opcua not connected")
	}
	req := &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{{NodeID: ua.NewStringNodeID(2, nodeID)}},
	}
	resp, err := c.conn.Read(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("no result for %s", nodeID)
	}
	return resp.Results[0].Value.Value(), nil
}

// Write 写一个节点值（float64）。
func (c *Client) Write(ctx context.Context, nodeID string, value float64) error {
	if c.conn == nil {
		return fmt.Errorf("opcua not connected")
	}
	v, err := ua.NewVariant(value)
	if err != nil {
		return err
	}
	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{{
			NodeID:      ua.NewStringNodeID(2, nodeID),
			AttributeID: ua.AttributeIDValue,
			Value:       &ua.DataValue{Value: v},
		}},
	}
	resp, err := c.conn.Write(ctx, req)
	if err != nil {
		return err
	}
	for _, result := range resp.Results {
		if result != ua.StatusOK {
			return fmt.Errorf("opcua write %s status %v", nodeID, result)
		}
	}
	return nil
}
