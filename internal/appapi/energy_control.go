package appapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/audit"
	"fastiot/internal/decision"
	"fastiot/internal/ems/command"
)

// 控制器人工操作：启停、检修、故障确认与自动/人工模式。启停命令走共享
// command.Manager，只有设备的真实回读确认后才推进柜体状态；前端不能
// 直接断言运行结果。

const pendingCommandPoint = "easyemsPendingCommand"

type pendingCommand struct {
	RequestID string    `json:"request_id"`
	Event     string    `json:"event"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
}

func loadPendingCommand(node data.NodeEdge) (pendingCommand, bool) {
	point, found := node.Points.Find(pendingCommandPoint, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return pendingCommand{}, false
	}
	var pending pendingCommand
	if json.Unmarshal([]byte(point.Txt()), &pending) != nil || pending.RequestID == "" {
		return pendingCommand{}, false
	}
	return pending, true
}

// resolveEnergyDevice 找到 EnergyUnit 的第一台已映射功率点位的柜内设备及其
// 运行时设备 ID（<通道>/<站号>）。有歧义时返回 false，不猜设备。
func (h *Handler) resolveEnergyDevice(node data.NodeEdge) (deviceNode data.NodeEdge, runtimeID string, ok bool) {
	devices, err := siotclient.GetNodes(h.nc, node.ID, "all", data.NodeTypeDevice, false)
	if err != nil {
		return data.NodeEdge{}, "", false
	}
	found := []data.NodeEdge{}
	for _, device := range devices {
		if device.Type != data.NodeTypeDevice {
			continue
		}
		if _, has := device.Points.Find(PointTypePowerSetpoint, ""); has {
			found = append(found, device)
		}
	}
	if len(found) != 1 {
		return data.NodeEdge{}, "", false
	}
	deviceNode = found[0]
	point, _ := deviceNode.Points.Find(PointTypePowerSetpoint, "")
	ioNodes, err := siotclient.GetNodes(h.nc, "all", point.Txt(), "", false)
	if err != nil || len(ioNodes) == 0 {
		return data.NodeEdge{}, "", false
	}
	ioNode := ioNodes[0]
	server := 0
	if idPoint, has := ioNode.Points.Find(data.PointTypeID, ""); has {
		server = int(pointFloat(idPoint))
	}
	if ioNode.Parent == "" || server < 1 || server > 247 {
		return data.NodeEdge{}, "", false
	}
	return deviceNode, ioNode.Parent + "/" + strconv.Itoa(server), true
}

// energyControl 处理 /energy-units/{id}/events 与 /energy-units/{id}/mode。
func (h *Handler) energyControl(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID, action string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	switch action {
	case "mode":
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !canAction(identity, ActionCommand) {
			writeError(w, http.StatusForbidden, "command_required")
			return
		}
		var input struct {
			Mode string `json:"mode"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		manual := 0.0
		switch input.Mode {
		case "manual":
			manual = 1
		case "auto":
			manual = 0
		default:
			writeError(w, http.StatusBadRequest, "mode_must_be_auto_or_manual")
			return
		}
		point := data.NewPointFloat(manualModePoint, "", manual)
		point.Origin = identity.UserID
		if err := siotclient.SendNodePoint(h.nc, energyUnitID, point, true); err != nil {
			writeError(w, http.StatusBadGateway, "save_failed")
			return
		}
		writeJSON(w, map[string]any{"mode": input.Mode})
	case "events":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if !canAction(identity, ActionCommand) {
			writeError(w, http.StatusForbidden, "command_required")
			return
		}
		var input struct {
			Event     string `json:"event"`
			RequestID string `json:"request_id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		h.applyControlEvent(w, identity, node, input.Event, input.RequestID)
	default:
		writeError(w, http.StatusNotFound, "not_found")
	}
}

func (h *Handler) applyControlEvent(w http.ResponseWriter, identity Identity, node data.NodeEdge, event, requestID string) {
	switch event {
	case "start", "stop":
		h.submitCabinetCommand(w, identity, node, event, requestID)
	case "fault_clear", "enter_maintenance", "exit_maintenance":
		eventName := decision.CabinetEvent(event)
		if event == "fault_clear" {
			eventName = decision.EvFaultCleared
		}
		next, ok, err := ApplyCabinetEvent(h.nc, node, eventName)
		if err != nil {
			writeError(w, http.StatusBadGateway, "save_failed")
			return
		}
		if !ok {
			writeError(w, http.StatusConflict, "invalid_state_transition")
			return
		}
		writeJSON(w, map[string]any{"state": next})
	default:
		writeError(w, http.StatusBadRequest, "unknown_event")
	}
}

func (h *Handler) submitCabinetCommand(w http.ResponseWriter, identity Identity, node data.NodeEdge, event, requestID string) {
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "request_id_required")
		return
	}
	current := LoadCabinetState(node)
	next, allowed := decision.Transition(current, decision.CabinetEvent(event))
	if !allowed {
		writeError(w, http.StatusConflict, "invalid_state_transition")
		return
	}
	manager := h.commands()
	if manager == nil {
		writeError(w, http.StatusServiceUnavailable, "command_runtime_unavailable")
		return
	}
	_, runtimeID, ok := h.resolveEnergyDevice(node)
	if !ok {
		writeError(w, http.StatusBadRequest, "energy_unit_has_no_unique_power_device")
		return
	}
	profile, ok := h.commandProfile(runtimeID)
	if !ok {
		writeError(w, http.StatusBadRequest, "device_profile_missing")
		return
	}
	if _, has := profile.Command(event); !has {
		writeError(w, http.StatusBadRequest, "profile_has_no_"+event+"_command")
		return
	}
	outcome, err := manager.Submit(time.Now(), command.Request{
		ID: requestID, DeviceID: runtimeID, CommandID: event, Actor: identity.UserID,
		Source: "manual", Stage: "cabinet", DispatchID: requestID,
	})
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := SetCabinetState(h.nc, node.ID, next); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	pending := pendingCommand{RequestID: requestID, Event: event, Actor: identity.UserID, At: time.Now()}
	encoded, _ := json.Marshal(pending)
	point := data.NewPointString(pendingCommandPoint, "", string(encoded))
	point.Origin = identity.UserID
	_ = siotclient.SendNodePoint(h.nc, node.ID, point, true)
	writeJSON(w, map[string]any{"state": next, "request_id": requestID, "command_state": outcome.State})
}

// ReconcileCabinetCommands 把已确认/失败/未知的启停命令落到柜体状态。
// 由周期调度器每秒调用一次。
func (h *Handler) ReconcileCabinetCommands() {
	manager := h.commands()
	if manager == nil {
		return
	}
	units, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return
	}
	for _, unit := range units {
		pending, ok := loadPendingCommand(unit)
		if !ok {
			continue
		}
		outcome, found := manager.Get(pending.RequestID)
		if !found {
			continue
		}
		var event decision.CabinetEvent
		switch outcome.State {
		case command.Confirmed:
			if pending.Event == "start" {
				event = decision.EvStartConfirmed
			} else {
				event = decision.EvStopConfirmed
			}
		case command.Failed, command.Unknown:
			// 启动失败回到待机；停机未确认按故障处理，禁止自动调度。
			if pending.Event == "start" {
				_, _, _ = ApplyCabinetEvent(h.nc, unit, decision.EvStopConfirmed)
			} else {
				_, _, _ = ApplyCabinetEvent(h.nc, unit, decision.EvFault)
			}
			h.clearPending(unit.ID)
			if h.audit != nil {
				_ = h.audit.Append(auditRecord(pending, string(outcome.State), outcome.Reason))
			}
			continue
		default:
			continue
		}
		_, _, _ = ApplyCabinetEvent(h.nc, unit, event)
		h.clearPending(unit.ID)
		if h.audit != nil {
			_ = h.audit.Append(auditRecord(pending, string(outcome.State), ""))
		}
	}
}

func (h *Handler) clearPending(unitID string) {
	point := data.NewPointString(pendingCommandPoint, "", "")
	point.Origin = "easyems"
	_ = siotclient.SendNodePoint(h.nc, unitID, point, true)
}

func auditRecord(pending pendingCommand, state, reason string) audit.Record {
	return audit.Record{
		Time: time.Now(), Actor: pending.Actor, Action: "控制写入",
		Object: "能源设备 " + pending.Event,
		Detail: strings.TrimSpace(state + " " + reason), Result: "success",
	}
}
