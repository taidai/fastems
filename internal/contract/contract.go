// Package contract holds API data structures shared with the generated
// TypeScript client. Keep these types free of runtime-only fields.
package contract

import "time"

// CommandParameter is one operator-supplied value of a named command.
type CommandParameter struct {
	Name    string  `json:"name"`
	Label   string  `json:"label"`
	PointID string  `json:"point_id"`
	Unit    string  `json:"unit,omitempty"`
	Kind    string  `json:"kind"` // number | bool | enum
	Min     float64 `json:"min,omitempty"`
	Max     float64 `json:"max,omitempty"`
	Default float64 `json:"default,omitempty"`
}

// CommandDefinition is one named command exposed by a unit model.
type CommandDefinition struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Description     string             `json:"description,omitempty"`
	Parameters      []CommandParameter `json:"parameters,omitempty"`
	TimeoutMS       int                `json:"timeout_ms"`
	RequiresConfirm bool               `json:"requires_confirm"`
}

// UnitModel binds a business device to a Modbus channel/unit and profile.
type UnitModel struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	BusinessNodeID string              `json:"business_node_id,omitempty"`
	ChannelID      string              `json:"channel_id"`
	ServerID       int                 `json:"server_id"`
	ProfileID      string              `json:"profile_id"`
	PointMap       map[string]string   `json:"point_map"`
	Commands       []CommandDefinition `json:"commands,omitempty"`
	Verified       bool                `json:"verified"`
	Verification   string              `json:"verification,omitempty"`
}

// CommandRequest submits one named command. RequestID deduplicates retries.
type CommandRequest struct {
	RequestID  string             `json:"request_id"`
	DeviceID   string             `json:"device_id"`
	CommandID  string             `json:"command_id"`
	Parameters map[string]float64 `json:"parameters,omitempty"`
}

// CommandOutcome is the lifecycle state of one command request.
type CommandOutcome struct {
	RequestID string    `json:"request_id"`
	DeviceID  string    `json:"device_id"`
	CommandID string    `json:"command_id"`
	Actor     string    `json:"actor"`
	State     string    `json:"state"` // accepted | executing | confirmed | failed | unknown
	Reason    string    `json:"reason,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditRecord is one business audit entry.
type AuditRecord struct {
	ID     string    `json:"id"`
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Object string    `json:"object,omitempty"`
	Detail string    `json:"detail,omitempty"`
	Result string    `json:"result"` // success | failure
	IP     string    `json:"ip,omitempty"`
}

// Frame is one captured Modbus request/response frame.
type Frame struct {
	ID        int64     `json:"id"`
	Time      time.Time `json:"time"`
	ChannelID string    `json:"channel_id,omitempty"`
	Peer      string    `json:"peer,omitempty"`
	Direction string    `json:"direction"` // tx | rx
	Hex       string    `json:"hex"`
}

// QualityStats is a rolling communication-quality summary.
type QualityStats struct {
	ChannelID     string    `json:"channel_id"`
	DeviceID      string    `json:"device_id,omitempty"`
	Health        string    `json:"health,omitempty"`
	Polls         int       `json:"polls"`
	Failures      int       `json:"failures"`
	SuccessRate   float64   `json:"success_rate"`
	AvgLatencyMS  float64   `json:"avg_latency_ms"`
	MaxLatencyMS  float64   `json:"max_latency_ms"`
	LastLatencyMS float64   `json:"last_latency_ms"`
	LastSuccess   time.Time `json:"last_success,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Rolling windows come from in-memory minute buckets and reset on
	// restart; cumulative counters stay available above.
	Window5m *WindowQuality `json:"window_5m,omitempty"`
	Window1h *WindowQuality `json:"window_1h,omitempty"`
}

// WindowQuality is one rolling quality window.
type WindowQuality struct {
	Minutes      int            `json:"minutes"`
	Polls        int            `json:"polls"`
	Failures     int            `json:"failures"`
	SuccessRate  float64        `json:"success_rate"`
	AvgLatencyMS float64        `json:"avg_latency_ms"`
	MaxLatencyMS float64        `json:"max_latency_ms"`
	Errors       map[string]int `json:"errors,omitempty"`
}
