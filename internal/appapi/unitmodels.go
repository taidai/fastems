package appapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/contract"
	"fastiot/internal/ems/command"
	"fastiot/internal/ems/device"
	"fastiot/internal/ems/profile"
	"fastiot/internal/ems/signal"
)

const unitModelPoint = "easyemsUnitModel"

type unitModelConfig struct {
	Name           string            `json:"name"`
	ProfileID      string            `json:"profile_id"`
	BusinessNodeID string            `json:"business_node_id,omitempty"`
	PointMap       map[string]string `json:"point_map"`
	Verified       bool              `json:"verified"`
	Verifier       string            `json:"verifier,omitempty"`
	VerifiedAt     string            `json:"verified_at,omitempty"`
	DeviceSerial   string            `json:"device_serial,omitempty"`
	Verification   string            `json:"verification,omitempty"`
}

type unitModelPointView struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ServerID int     `json:"server_id"`
	Address  int     `json:"address"`
	IOType   string  `json:"io_type"`
	ReadOnly bool    `json:"read_only"`
	Unit     string  `json:"unit,omitempty"`
	Scale    float64 `json:"scale"`
}

type unitModelView struct {
	ChannelID     string               `json:"channel_id"`
	ServerID      int                  `json:"server_id"`
	Model         *unitModelConfig     `json:"model,omitempty"`
	Profiles      []profile.Profile    `json:"profiles"`
	ProfileErrors []string             `json:"profile_errors,omitempty"`
	Points        []unitModelPointView `json:"points"`
	Commands      []unitModelCommand   `json:"commands"`
	Missing       []string             `json:"missing,omitempty"`
}

type unitModelParameter struct {
	Name      string  `json:"name"`
	Label     string  `json:"label"`
	PointID   string  `json:"point_id,omitempty"`
	PointName string  `json:"point_name"`
	Unit      string  `json:"unit,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	HasRange  bool    `json:"has_range,omitempty"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Default   float64 `json:"default"`
	Available bool    `json:"available"`
	Reason    string  `json:"reason,omitempty"`
}

type unitModelCommand struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	PointID    string               `json:"point_id,omitempty"`
	PointName  string               `json:"point_name"`
	Value      float64              `json:"value"`
	Parameters []unitModelParameter `json:"parameters,omitempty"`
	Available  bool                 `json:"available"`
	Reason     string               `json:"reason,omitempty"`
}

func (h *Handler) unitModel(w http.ResponseWriter, r *http.Request, identity Identity, channelID string, server int) {
	channel, ok := h.node(w, identity, channelID)
	if !ok {
		return
	}
	if channel.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	if h.ems == nil || h.ems.Profiles == nil {
		writeError(w, http.StatusServiceUnavailable, "ems_unavailable")
		return
	}
	points := h.unitPoints(channel.ID)
	model, hasModel := unitModelFor(channel, server)
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, unitModelResponse(channel.ID, server, model, hasModel, points, h.ems.Profiles))
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input unitModelConfig
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := h.saveUnitModel(channel, server, input, points); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, unitModelResponse(channel.ID, server, input, true, points, h.ems.Profiles))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) unitPoints(channelID string) []unitModelPointView {
	nodes, err := client.GetNodes(h.nc, channelID, "all", "", false)
	if err != nil {
		return nil
	}
	points := make([]unitModelPointView, 0)
	for _, node := range nodes {
		if node.Type != data.NodeTypeModbusIO || node.Parent != channelID {
			continue
		}
		server, _ := node.Points.ValueInt(data.PointTypeID, "")
		address, _ := node.Points.ValueInt(data.PointTypeAddress, "")
		points = append(points, unitModelPointView{
			ID: node.ID, Name: textPoint(node.Points, data.PointTypeDescription),
			ServerID: server, Address: address,
			IOType:   textPoint(node.Points, data.PointTypeModbusIOType),
			ReadOnly: readOnlyNode(node),
			Unit:     textPoint(node.Points, data.PointTypeUnits),
			Scale:    numberPoint(node.Points, data.PointTypeScale),
		})
	}
	return points
}

func unitModelFor(channel data.NodeEdge, server int) (unitModelConfig, bool) {
	point, ok := channel.Points.Find(unitModelPoint, strconv.Itoa(server))
	if !ok || point.Tombstone%2 != 0 {
		return unitModelConfig{}, false
	}
	var model unitModelConfig
	if json.Unmarshal([]byte(point.Txt()), &model) != nil {
		return unitModelConfig{}, false
	}
	if model.PointMap == nil {
		model.PointMap = map[string]string{}
	}
	return model, true
}

func unitModelResponse(channelID string, server int, model unitModelConfig, hasModel bool, points []unitModelPointView, registry *profile.Registry) unitModelView {
	if points == nil {
		points = []unitModelPointView{}
	}
	view := unitModelView{
		ChannelID: channelID, ServerID: server,
		Points: points, Profiles: registry.List(),
		ProfileErrors: registry.LoadErrors,
		Commands:      []unitModelCommand{},
	}
	if hasModel {
		view.Model = &model
	}
	if hasModel {
		if prof, ok := registry.Get(model.ProfileID); ok {
			seen := map[string]bool{}
			for _, spec := range prof.CommandSpecs {
				pointID := model.PointMap[spec.Point]
				item := unitModelCommand{ID: spec.ID, Name: spec.Name, PointID: pointID, PointName: spec.Point, Value: spec.Value}
				available := true
				reason := ""
				if spec.Point != "" {
					if pointID == "" {
						available = false
						reason = "未映射点位"
						if !seen[spec.Point] {
							view.Missing = append(view.Missing, spec.Point)
							seen[spec.Point] = true
						}
					} else if point, ok := findPoint(points, pointID); !ok {
						available = false
						reason = "映射点位不存在"
					} else if point.ReadOnly {
						available = false
						reason = "点位为只读"
					}
				}
				for _, parameter := range spec.Parameters {
					mappedID := model.PointMap[parameter.Point]
					param := unitModelParameter{
						Name: parameter.Name, Label: parameter.Label, PointID: mappedID,
						PointName: parameter.Point, Unit: parameter.Unit, Kind: parameter.Kind,
						HasRange: parameter.HasRange, Min: parameter.Min, Max: parameter.Max, Default: parameter.Default,
					}
					if mappedID == "" {
						param.Reason = "未映射点位"
						available = false
						reason = "参数点位未映射"
						if !seen[parameter.Point] {
							view.Missing = append(view.Missing, parameter.Point)
							seen[parameter.Point] = true
						}
					} else if point, ok := findPoint(points, mappedID); !ok {
						param.Reason = "映射点位不存在"
						available = false
						reason = "参数点位不存在"
					} else if point.ReadOnly {
						param.Reason = "点位为只读"
						available = false
						reason = "参数点位为只读"
					} else {
						param.Available = true
					}
					item.Parameters = append(item.Parameters, param)
				}
				if spec.Point == "" && len(spec.Parameters) == 0 {
					available = false
					reason = "命令未配置写点"
				}
				item.Available = available
				item.Reason = reason
				view.Commands = append(view.Commands, item)
			}
		}
	}
	return view
}

func (h *Handler) saveUnitModel(channel data.NodeEdge, server int, input unitModelConfig, points []unitModelPointView) error {
	if server < 1 || server > 247 {
		return fmt.Errorf("invalid station %d", server)
	}
	if input.ProfileID == "" {
		return fmt.Errorf("profile is required")
	}
	if _, ok := h.ems.Profiles.Get(input.ProfileID); !ok {
		return fmt.Errorf("unknown profile %s", input.ProfileID)
	}
	byID := map[string]unitModelPointView{}
	for _, point := range points {
		byID[point.ID] = point
	}
	cleaned := map[string]string{}
	for name, id := range input.PointMap {
		point, ok := byID[id]
		if !ok || point.ServerID != server {
			return fmt.Errorf("point %s is not under station %d", id, server)
		}
		cleaned[name] = id
	}
	input.PointMap = cleaned
	if input.Name == "" {
		input.Name = "站号 " + strconv.Itoa(server)
	}
	if input.Verified && (strings.TrimSpace(input.Verifier) == "" || strings.TrimSpace(input.VerifiedAt) == "" || strings.TrimSpace(input.DeviceSerial) == "") {
		return fmt.Errorf("verified model requires verifier, date and device serial")
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	point := data.NewPointString(unitModelPoint, strconv.Itoa(server), string(encoded))
	point.Origin = "easyems"
	return client.SendNodePoint(h.nc, channel.ID, point, true)
}

func (h *Handler) commandProfile(deviceID string) (device.Profile, bool) {
	if h.ems == nil || h.ems.Profiles == nil {
		return device.Profile{}, false
	}
	channelID, serverText, ok := strings.Cut(deviceID, "/")
	if !ok {
		return device.Profile{}, false
	}
	server, err := strconv.Atoi(serverText)
	if err != nil {
		return device.Profile{}, false
	}
	channelNodes, err := client.GetNodes(h.nc, "all", channelID, "", false)
	if err != nil || len(channelNodes) == 0 {
		return device.Profile{}, false
	}
	channel := channelNodes[0]
	model, ok := unitModelFor(channel, server)
	if !ok {
		return device.Profile{}, false
	}
	prof, ok := h.ems.Profiles.Get(model.ProfileID)
	if !ok {
		return device.Profile{}, false
	}
	points := h.unitPoints(channelID)
	byID := map[string]unitModelPointView{}
	for _, point := range points {
		byID[point.ID] = point
	}
	profilePoints := make([]device.Point, 0, len(prof.PointSpecs))
	kinds := map[string]signal.Kind{}
	seenPoints := map[string]bool{}
	for _, spec := range prof.PointSpecs {
		id := model.PointMap[spec.Name]
		point, ok := byID[id]
		if !ok || seenPoints[id] {
			continue
		}
		kind := signal.Number
		if point.IOType == data.PointValueModbusCoil || point.IOType == data.PointValueModbusDiscreteInput {
			kind = signal.Bool
		}
		kinds[id] = kind
		seenPoints[id] = true
		profilePoints = append(profilePoints, device.Point{
			ID: id, Kind: kind, Unit: point.Unit, ReadOnly: point.ReadOnly,
		})
	}
	commands := make([]device.Command, 0, len(prof.CommandSpecs))
	for _, spec := range prof.CommandSpecs {
		var writes []device.Write
		var readback []device.Readback
		valid := true
		if spec.Point != "" {
			id := model.PointMap[spec.Point]
			if id == "" {
				continue
			}
			if _, ok := byID[id]; !ok {
				continue
			}
			kind := kinds[id]
			value := signal.Num(spec.Value)
			if kind == signal.Bool {
				value = signal.Flag(spec.Value != 0)
			}
			writes = []device.Write{{Point: id, Value: value}}
			readback = []device.Readback{{Point: id, Op: "eq", Value: value}}
		}
		parameters := make([]device.Parameter, 0, len(spec.Parameters))
		for _, parameter := range spec.Parameters {
			id := model.PointMap[parameter.Point]
			point, ok := byID[id]
			if id == "" || !ok || point.ReadOnly {
				valid = false
				break
			}
			kind := kinds[id]
			parameters = append(parameters, device.Parameter{
				Name: parameter.Name, Label: parameter.Label, Point: id, Kind: kind,
				Unit: parameter.Unit, HasRange: parameter.HasRange,
				Min: parameter.Min, Max: parameter.Max, Default: parameter.Default,
			})
		}
		if !valid || (len(writes) == 0 && len(parameters) == 0) {
			continue
		}
		commands = append(commands, device.Command{
			ID: spec.ID, Name: spec.Name, Timeout: 5 * time.Second,
			Writes: writes, Readback: readback, Parameters: parameters,
		})
	}
	if len(commands) == 0 {
		return device.Profile{}, false
	}
	return device.Profile{ID: model.ProfileID, Vendor: prof.Vendor, Model: prof.Model, Points: profilePoints, Commands: commands}, true
}

func (h *Handler) unitCommands(w http.ResponseWriter, r *http.Request, identity Identity, channelID string, server int, requestID string) {
	if !canAction(identity, ActionCommand) {
		writeError(w, http.StatusForbidden, "operator_required")
		return
	}
	channel, ok := h.node(w, identity, channelID)
	if !ok {
		return
	}
	if channel.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	deviceID := fmt.Sprintf("%s/%d", channelID, server)
	switch {
	case r.Method == http.MethodGet && requestID == "":
		outcomes := h.ems.Service.Commands().List()
		list := make([]contract.CommandOutcome, 0)
		for _, outcome := range outcomes {
			if outcome.DeviceID == deviceID {
				list = append(list, commandOutcome(outcome))
			}
		}
		writeJSON(w, map[string]any{"commands": list})
		return
	case r.Method == http.MethodGet && requestID != "":
		outcome, ok := h.ems.Service.Commands().Get(requestID)
		if !ok || outcome.DeviceID != deviceID {
			writeError(w, http.StatusNotFound, "command_not_found")
			return
		}
		writeJSON(w, commandOutcome(outcome))
		return
	case r.Method == http.MethodPost && requestID == "":
		var input struct {
			RequestID  string             `json:"request_id"`
			CommandID  string             `json:"command_id"`
			Parameters map[string]float64 `json:"parameters"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if input.RequestID == "" || input.CommandID == "" {
			writeError(w, http.StatusBadRequest, "request_id_and_command_required")
			return
		}
		model, ok := unitModelFor(channel, server)
		if !ok {
			writeError(w, http.StatusConflict, "unit_model_not_configured")
			return
		}
		prof, ok := h.commandProfile(deviceID)
		if !ok {
			writeError(w, http.StatusConflict, "command_points_not_mapped")
			return
		}
		if _, ok := prof.Command(input.CommandID); !ok {
			writeError(w, http.StatusBadRequest, "command_not_available")
			return
		}
		if h.ems.Service.State.Health(deviceID) == "offline" {
			writeError(w, http.StatusConflict, "device_offline")
			return
		}
		outcome, err := h.ems.Service.Commands().Submit(time.Now(), command.Request{
			ID: input.RequestID, DeviceID: deviceID, CommandID: input.CommandID, Actor: identity.UserID,
			Parameters: input.Parameters,
		})
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		_ = model
		writeJSON(w, commandOutcome(outcome))
		return
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func commandOutcome(outcome command.Outcome) contract.CommandOutcome {
	return contract.CommandOutcome{
		RequestID: outcome.RequestID, DeviceID: outcome.DeviceID, CommandID: outcome.CommandID,
		Actor: outcome.Actor, State: string(outcome.State), Reason: outcome.Reason,
		StartedAt: outcome.StartedAt, UpdatedAt: outcome.UpdatedAt,
	}
}

func findPoint(points []unitModelPointView, id string) (unitModelPointView, bool) {
	for _, point := range points {
		if point.ID == id {
			return point, true
		}
	}
	return unitModelPointView{}, false
}

func hasPoint(points []unitModelPointView, id string) bool {
	_, ok := findPoint(points, id)
	return ok
}

func textPoint(points data.Points, typ string) string {
	point, ok := points.Find(typ, "")
	if !ok {
		return ""
	}
	return point.Txt()
}

func numberPoint(points data.Points, typ string) float64 {
	value, _ := points.Value(typ, "")
	return value
}

func readOnlyNode(node data.NodeEdge) bool {
	return numberPoint(node.Points, data.PointTypeReadOnly) != 0
}
