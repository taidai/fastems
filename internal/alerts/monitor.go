package alerts

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

type Alert struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Severity  string    `json:"severity"`
	Active    bool      `json:"active"`
	Status    string    `json:"status"`
	NodeID    string    `json:"node_id"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AckedBy   string    `json:"acked_by,omitempty"`
	Assignee  string    `json:"assignee,omitempty"`
}

func Sync(nc *nats.Conn, nodeID, kind, severity, title, detail string, active bool) error {
	nodes, err := client.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return err
	}
	now := time.Now()
	alert := Alert{}
	point, found := nodes[0].Points.Find("alert", kind)
	if found {
		_ = json.Unmarshal([]byte(point.Txt()), &alert)
	}
	if active && alert.Active && alert.Detail == detail && alert.Status != "closed" {
		return nil
	}
	if !active && (!found || !alert.Active) {
		return nil
	}
	if active && !alert.Active {
		alert.CreatedAt = now
		alert.Status = "active"
		alert.AckedBy = ""
	}
	if !active {
		alert.Status = "resolved"
		alert.Active = false
	} else {
		alert.Active = true
		if alert.Status == "closed" || alert.Status == "resolved" {
			alert.Status = "active"
		}
	}
	alert.ID = nodeID + ":" + kind
	alert.Kind = kind
	alert.Severity = severity
	alert.NodeID = nodeID
	alert.Title = title
	alert.Detail = detail
	alert.UpdatedAt = now
	encoded, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	point = data.NewPointString("alert", kind, string(encoded))
	point.Origin = "easyems-alerts"
	return client.SendNodePoint(nc, nodeID, point, true)
}

type Monitor struct {
	nc   *nats.Conn
	stop chan struct{}
}

func NewMonitor(nc *nats.Conn) *Monitor {
	return &Monitor{nc: nc, stop: make(chan struct{})}
}

func (m *Monitor) Run() error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	if err := m.scan(); err != nil {
		return err
	}
	for {
		select {
		case <-m.stop:
			return nil
		case <-ticker.C:
			if err := m.scan(); err != nil {
				return err
			}
		}
	}
}

func (m *Monitor) Stop(error) {
	close(m.stop)
}

func (m *Monitor) scan() error {
	rules, err := client.GetNodes(m.nc, "all", "all", "decisionTable", false)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		message, _ := rule.Points.Text(data.PointTypeError, "")
		if err := Sync(m.nc, rule.ID, "rule-error", "critical", "规则运行异常", message, message != ""); err != nil {
			return err
		}
	}
	buses, err := client.GetNodes(m.nc, "all", "all", "modbus", false)
	if err != nil {
		return err
	}
	for _, bus := range buses {
		children, err := client.GetNodes(m.nc, bus.ID, "all", "", false)
		if err != nil {
			return err
		}
		disabled, _ := bus.Points.Value(data.PointTypeDisabled, "")
		if disabled != 0 || len(children) == 0 {
			if err := Sync(m.nc, bus.ID, "bus-offline", "critical", "连接中断", "", false); err != nil {
				return err
			}
			continue
		}
		heartbeat, found := bus.Points.Find("lastGoodPoll", "")
		poll, _ := bus.Points.Value(data.PointTypePollPeriod, "")
		timeout, _ := bus.Points.Value(data.PointTypeTimeout, "")
		maxAge := 5 * time.Second
		if value := 5 * time.Duration(poll) * time.Millisecond; value > maxAge {
			maxAge = value
		}
		if value := 3 * time.Duration(timeout) * time.Millisecond; value > maxAge {
			maxAge = value
		}
		stale := !found || heartbeat.Time.IsZero() || time.Since(heartbeat.Time) > maxAge || time.Since(heartbeat.Time) < -5*time.Second
		if !found {
			created, hasCreated := bus.Points.Find(data.PointTypeDescription, "")
			if hasCreated && time.Since(created.Time) < maxAge {
				stale = false
			}
		}
		if err := Sync(m.nc, bus.ID, "bus-offline", "critical", "Modbus 连接中断", "最近完整成功轮询已过期", stale); err != nil {
			return err
		}
	}
	points, err := client.GetNodes(m.nc, "all", "all", "", false)
	if err != nil {
		return err
	}
	for _, node := range points {
		if node.Type != data.NodeTypeVariable && node.Type != data.NodeTypeModbusIO {
			continue
		}
		text, _ := node.Points.Text("lastCommand", "")
		failed := false
		detail := ""
		if text != "" {
			var command struct {
				Status string `json:"status"`
			}
			if json.Unmarshal([]byte(text), &command) == nil {
				failed = command.Status == "failed" || command.Status == "unknown"
				detail = "命令状态：" + command.Status
			}
		}
		if err := Sync(m.nc, node.ID, "command-failed", "important", "控制结果异常", detail, failed); err != nil {
			return err
		}
	}
	return nil
}
