package appapi

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"fastems/internal/ems/command"
)

const (
	commandLogFile = "commands.jsonl"
	commandLogKeep = 500
)

// commandLogRecord is the persisted form of a command outcome: the audit-facing
// fields only, never write payloads beyond state and reason.
type commandLogRecord struct {
	RequestID   string    `json:"request_id"`
	DeviceID    string    `json:"device_id"`
	CommandID   string    `json:"command_id,omitempty"`
	Actor       string    `json:"actor,omitempty"`
	State       string    `json:"state"`
	Reason      string    `json:"reason,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DispatchID  string    `json:"dispatch_id,omitempty"`
	Source      string    `json:"source,omitempty"`
	Stage       string    `json:"stage,omitempty"`
	FallbackOf  string    `json:"fallback_of,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
}

// WireCommandHistory replays persisted command outcomes into the manager and
// hooks new state changes back to <dataDir>/commands.jsonl. The log keeps the
// newest commandLogKeep requests (last state per request id wins) so command
// history survives restarts without growing without bound.
func WireCommandHistory(dataDir string, manager *command.Manager) {
	if dataDir == "" || manager == nil {
		return
	}
	path := filepath.Join(dataDir, commandLogFile)
	records, order := readCommandLog(path)
	if len(order) > commandLogKeep {
		order = order[len(order)-commandLogKeep:]
	}
	outcomes := make([]command.Outcome, 0, len(order))
	for _, id := range order {
		record := records[id]
		outcomes = append(outcomes, command.Outcome{
			RequestID: record.RequestID, DeviceID: record.DeviceID, CommandID: record.CommandID,
			Actor: record.Actor, State: command.State(record.State), Reason: record.Reason,
			StartedAt: record.StartedAt, UpdatedAt: record.UpdatedAt,
			DispatchID: record.DispatchID, Source: record.Source, Stage: record.Stage,
			FallbackOf: record.FallbackOf, Fingerprint: record.Fingerprint,
		})
	}
	manager.Preload(outcomes)
	rewriteCommandLog(path, records, order)
	manager.Persist = func(outcome command.Outcome) {
		// 自动功率调度每秒一次，明细已进入调度追溯；这里只持久化人工/命名
		// 命令，避免命令历史被自动写入淹没。
		if outcome.Actor == "" && outcome.CommandID == "" && outcome.Source != "manual" {
			return
		}
		appendCommandLog(path, commandLogRecord{
			RequestID: outcome.RequestID, DeviceID: outcome.DeviceID, CommandID: outcome.CommandID,
			Actor: outcome.Actor, State: string(outcome.State), Reason: outcome.Reason,
			StartedAt: outcome.StartedAt, UpdatedAt: outcome.UpdatedAt,
			DispatchID: outcome.DispatchID, Source: outcome.Source, Stage: outcome.Stage,
			FallbackOf: outcome.FallbackOf, Fingerprint: outcome.Fingerprint,
		})
	}
}

func readCommandLog(path string) (map[string]commandLogRecord, []string) {
	records := map[string]commandLogRecord{}
	var order []string
	file, err := os.Open(path)
	if err != nil {
		return records, order
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record commandLogRecord
		if json.Unmarshal(scanner.Bytes(), &record) != nil || record.RequestID == "" {
			continue
		}
		if _, exists := records[record.RequestID]; !exists {
			order = append(order, record.RequestID)
		}
		records[record.RequestID] = record
	}
	return records, order
}

func rewriteCommandLog(path string, records map[string]commandLogRecord, order []string) {
	if len(order) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	file, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	for _, id := range order {
		encoded, err := json.Marshal(records[id])
		if err != nil {
			continue
		}
		_, _ = file.Write(append(encoded, '\n'))
	}
	_ = file.Close()
	_ = os.Rename(path+".tmp", path)
}

func appendCommandLog(path string, record commandLogRecord) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}
	_, _ = file.Write(append(encoded, '\n'))
}
