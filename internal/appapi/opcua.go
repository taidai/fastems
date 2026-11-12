package appapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/ems/channel"
	"fastiot/internal/ems/opcua"
	"fastiot/internal/ems/runtime"
	"fastiot/internal/ems/signal"
)

// OPC-UA 接入：每条 opcua 通道对应一个端点，子节点 opcuaPoint 映射完整
// NodeID。Runner 轮询读取并把值写回产品点位；写入通过共享命令管理器
// （注册为该通道的 dispatcher），broker/设备确认仍以真实回读为准。

const (
	NodeTypeOpcua        = "opcua"
	NodeTypeOpcuaPoint   = "opcuaPoint"
	pointTypeOpcuaURI    = "uri"
	pointTypeOpcuaNode   = "nodeId"
	pointTypeOpcuaPoll   = "pollPeriod"
	pointTypeOpcuaHealth = "opcuaHealth"
	pointTypeLastGood    = "lastGoodPoll"
	pointTypeLastError   = "lastError"
)

// OpcuaManager 管理运行中的 OPC-UA runner。
type OpcuaManager struct {
	nc      *nats.Conn
	service *runtime.Service
	mu      sync.Mutex
	runners map[string]context.CancelFunc
	active  map[string]string // channel ID → 配置指纹
	// Dial overrides the OPC-UA connection factory in tests.
	Dial opcua.Dialer
}

// NewOpcuaManager 创建管理器。
func NewOpcuaManager(nc *nats.Conn, service *runtime.Service) *OpcuaManager {
	return &OpcuaManager{nc: nc, service: service, runners: map[string]context.CancelFunc{}, active: map[string]string{}}
}

type opcuaChannelConfig struct {
	ID          string
	Endpoint    string
	PollPeriod  time.Duration
	Description string
	Points      []opcua.PointMapping
	fingerprint string
}

func loadOpcuaChannels(nc *nats.Conn) []opcuaChannelConfig {
	channels, err := siotclient.GetNodes(nc, "all", "all", NodeTypeOpcua, false)
	if err != nil {
		return nil
	}
	configs := make([]opcuaChannelConfig, 0, len(channels))
	for _, node := range channels {
		if disabled, _ := node.Points.Value(data.PointTypeDisabled, ""); disabled != 0 {
			continue
		}
		endpoint, _ := node.Points.Text(pointTypeOpcuaURI, "")
		if endpoint == "" {
			continue
		}
		poll, _ := node.Points.Value(pointTypeOpcuaPoll, "")
		period := time.Duration(poll) * time.Millisecond
		if period <= 0 {
			period = 2 * time.Second
		}
		points, err := siotclient.GetNodes(nc, node.ID, "all", NodeTypeOpcuaPoint, false)
		if err != nil {
			continue
		}
		config := opcuaChannelConfig{ID: node.ID, Endpoint: endpoint, PollPeriod: period}
		fingerprint := endpoint + "|" + period.String()
		for _, point := range points {
			if disabled, _ := point.Points.Value(data.PointTypeDisabled, ""); disabled != 0 {
				continue
			}
			nodeID, _ := point.Points.Text(pointTypeOpcuaNode, "")
			if nodeID == "" {
				continue
			}
			config.Points = append(config.Points, opcua.PointMapping{PointID: point.ID, NodeID: nodeID})
			fingerprint += "|" + point.ID + ":" + nodeID
		}
		config.fingerprint = fingerprint
		configs = append(configs, config)
	}
	return configs
}

// Sync 按当前配置启动/停止/重建 runner。配置变化时重建，移除的通道停止。
func (m *OpcuaManager) Sync() {
	configs := loadOpcuaChannels(m.nc)
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	for _, config := range configs {
		seen[config.ID] = true
		if current, exists := m.active[config.ID]; exists && current == config.fingerprint {
			continue
		}
		if cancel, exists := m.runners[config.ID]; exists {
			cancel()
			delete(m.runners, config.ID)
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.runners[config.ID] = cancel
		m.active[config.ID] = config.fingerprint
		runner := opcua.NewRunner(config.Endpoint, config.ID, config.Points)
		runner.Interval = config.PollPeriod
		if m.Dial != nil {
			runner.Dial = m.Dial
		}
		runner.Observe = m.publish
		runner.OnHealth = func(online bool, reason string) { m.publishHealth(config.ID, online, reason) }
		if m.service != nil {
			m.service.RegisterDispatcher(config.ID, runner)
		}
		go func(id string, runner *opcua.Runner) { _ = runner.Run(ctx) }(config.ID, runner)
	}
	for id, cancel := range m.runners {
		if seen[id] {
			continue
		}
		cancel()
		delete(m.runners, id)
		delete(m.active, id)
	}
}

func (m *OpcuaManager) publish(deviceID string, values []opcua.ResultValue) {
	results := make([]channel.Result, 0, len(values))
	for _, value := range values {
		at := value.At
		if at.IsZero() {
			at = time.Now()
		}
		point := data.NewPointFloat(data.PointTypeValue, "", value.Value.Num)
		point.Time = at
		point.Origin = "opcua"
		_ = siotclient.SendNodePoint(m.nc, value.PointID, point, false)
		results = append(results, channel.Result{
			DeviceID: deviceID, PointID: value.PointID, Value: value.Value,
			Quality: signal.Good, SourceTime: at,
		})
	}
	if m.service != nil && len(results) > 0 {
		m.service.Observe(time.Now(), results)
	}
	if len(results) > 0 {
		_ = siotclient.SendNodePoint(m.nc, deviceID, data.NewPointFloat(pointTypeLastGood, "", float64(time.Now().Unix())), false)
		_ = siotclient.SendNodePoint(m.nc, deviceID, data.NewPointFloat(pointTypeOpcuaHealth, "", 1), false)
	}
}

func (m *OpcuaManager) publishHealth(channelID string, online bool, reason string) {
	health := 0.0
	if online {
		health = 1
	}
	_ = siotclient.SendNodePoint(m.nc, channelID, data.NewPointFloat(pointTypeOpcuaHealth, "", health), false)
	if reason != "" {
		_ = siotclient.SendNodePoint(m.nc, channelID, data.NewPointString(pointTypeLastError, "", reason), false)
	}
}

// opcuaChannels 处理通道列表与创建：GET/POST /opcua/channels。
func (h *Handler) opcuaChannels(w http.ResponseWriter, r *http.Request, identity Identity) {
	switch r.Method {
	case http.MethodGet:
		nodes, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypeOpcua, false)
		if err != nil {
			writeError(w, http.StatusBadGateway, "list_failed")
			return
		}
		items := make([]map[string]any, 0, len(nodes))
		for _, node := range nodes {
			if !h.allows(identity, node.ID) {
				continue
			}
			endpoint, _ := node.Points.Text(pointTypeOpcuaURI, "")
			description, _ := node.Points.Text(data.PointTypeDescription, "")
			poll, _ := node.Points.Value(pointTypeOpcuaPoll, "")
			health, _ := node.Points.Value(pointTypeOpcuaHealth, "")
			lastError, _ := node.Points.Text(pointTypeLastError, "")
			lastGood, _ := node.Points.Find(pointTypeLastGood, "")
			points, _ := siotclient.GetNodes(h.nc, node.ID, "all", NodeTypeOpcuaPoint, false)
			items = append(items, map[string]any{
				"id": node.ID, "description": description, "endpoint": endpoint,
				"poll_ms": poll, "online": health != 0, "last_error": lastError,
				"last_good": lastGood.Time, "points": len(points),
			})
		}
		writeJSON(w, map[string]any{"channels": items})
	case http.MethodPost:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input struct {
			Parent      string `json:"parent"`
			Description string `json:"description"`
			Endpoint    string `json:"endpoint"`
			PollMS      int    `json:"poll_ms"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if input.Endpoint == "" {
			writeError(w, http.StatusBadRequest, "endpoint_required")
			return
		}
		if input.Parent == "" {
			root, err := siotclient.GetRootNode(h.nc)
			if err != nil {
				writeError(w, http.StatusBadGateway, "root_unavailable")
				return
			}
			input.Parent = root.ID
		}
		if _, ok := h.node(w, identity, input.Parent); !ok {
			return
		}
		id, err := CreateDomainNode(h.nc, input.Parent, NodeTypeOpcua, map[string]string{
			data.PointTypeDescription: input.Description, pointTypeOpcuaURI: input.Endpoint,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "create_failed")
			return
		}
		if input.PollMS > 0 {
			_ = siotclient.SendNodePoint(h.nc, id, data.NewPointFloat(pointTypeOpcuaPoll, "", float64(input.PollMS)), true)
		}
		if h.opcua != nil {
			h.opcua.Sync()
		}
		writeJSON(w, map[string]string{"id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

// opcuaPoints 处理某通道下的测点：GET/POST /opcua/channels/{id}/points。
func (h *Handler) opcuaPoints(w http.ResponseWriter, r *http.Request, identity Identity, channelID string) {
	node, ok := h.node(w, identity, channelID)
	if !ok {
		return
	}
	if node.Type != NodeTypeOpcua {
		writeError(w, http.StatusBadRequest, "requires_opcua_channel")
		return
	}
	switch r.Method {
	case http.MethodGet:
		nodes, err := siotclient.GetNodes(h.nc, channelID, "all", NodeTypeOpcuaPoint, false)
		if err != nil {
			writeError(w, http.StatusBadGateway, "list_failed")
			return
		}
		items := make([]map[string]any, 0, len(nodes))
		for _, point := range nodes {
			nodeID, _ := point.Points.Text(pointTypeOpcuaNode, "")
			description, _ := point.Points.Text(data.PointTypeDescription, "")
			readOnly, _ := point.Points.Value(data.PointTypeReadOnly, "")
			value, _ := point.Points.Value(data.PointTypeValue, "")
			items = append(items, map[string]any{
				"id": point.ID, "description": description, "node_id": nodeID,
				"read_only": readOnly != 0, "value": value,
			})
		}
		writeJSON(w, map[string]any{"points": items})
	case http.MethodPost:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input struct {
			NodeID      string `json:"node_id"`
			Description string `json:"description"`
			ReadOnly    bool   `json:"read_only"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if _, err := opcua.ParseNodeID(input.NodeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_node_id")
			return
		}
		id, err := CreateDomainNode(h.nc, channelID, NodeTypeOpcuaPoint, map[string]string{
			data.PointTypeDescription: input.Description, pointTypeOpcuaNode: input.NodeID,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "create_failed")
			return
		}
		if input.ReadOnly {
			_ = siotclient.SendNodePoint(h.nc, id, data.NewPointFloat(data.PointTypeReadOnly, "", 1), true)
		}
		if h.opcua != nil {
			h.opcua.Sync()
		}
		writeJSON(w, map[string]string{"id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}
