// Package emsapi exposes the runtime through the product HTTP API: channels,
// state, alarms, history and CSV export.
package emsapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"easyems/internal/contract"
	"easyems/internal/ems/bridge"
	"easyems/internal/ems/channel"
	"easyems/internal/ems/framelog"
	"easyems/internal/ems/modbusrt"
	"easyems/internal/ems/profile"
	"easyems/internal/ems/runtime"
	"easyems/internal/ems/signal"

	"easyems/internal/ems/alloc"
)

// Access is the authenticated caller as seen by this API.
type Access struct {
	UserID    string
	Actor     string
	Admin     bool
	Configure bool
	Operate   bool
}

func (a Access) actor() string {
	if a.Actor != "" {
		return a.Actor
	}
	return a.UserID
}

// Handler serves /api/ems/*.
type Handler struct {
	Service *runtime.Service
	MaxAge  time.Duration
	// OpenBus opens a channel transport. Tests replace it.
	OpenBus func(protocol, address string, timeout time.Duration) (channel.Bus, error)
	// Buses returns persisted channel configuration from product nodes.
	Buses func() ([]bridge.BusConfig, error)
	// Configure creates and deletes persisted channels. It is wired by the
	// product API so configuration lives in the same nodes it reads.
	Configure ChannelConfigurator
	// Profiles is the built-in device profile registry.
	Profiles *profile.Registry
	// PersistMaintenance is wired by the product API to save maintenance windows.
	PersistMaintenance func()
	// RestartChannel reopens one channel transport.
	RestartChannel func(string) error
}

// ChannelConfigurator persists channel configuration as product nodes.
type ChannelConfigurator interface {
	Create(access Access, input ChannelInput) (map[string]any, error)
	Delete(access Access, id string) error
}

// New returns the API handler with TCP/RTU transports.
func New(service *runtime.Service, maxAge time.Duration) *Handler {
	return &Handler{
		Service:  service,
		MaxAge:   maxAge,
		Profiles: profile.Default(),
		OpenBus: func(protocol, address string, timeout time.Duration) (channel.Bus, error) {
			switch protocol {
			case "tcp", "":
				return modbusrt.OpenTCP(address, timeout)
			case "rtu":
				cfg, err := modbusrt.ParseSerialAddress(address)
				if err != nil {
					return nil, err
				}
				return modbusrt.OpenRTUConfig(cfg, timeout)
			default:
				return nil, fmt.Errorf("unsupported protocol %q", protocol)
			}
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, access Access) {
	path := strings.Trim(r.URL.Path, "/")
	path = strings.TrimPrefix(path, "api/")
	if !strings.HasPrefix(path, "ems/") {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	path = strings.TrimPrefix(path, "ems/")
	parts := strings.Split(path, "/")
	switch {
	case path == "quality" && r.Method == http.MethodGet:
		h.quality(w)
	case path == "frames" && r.Method == http.MethodGet:
		h.frames(w, r, access)
	case path == "frames" && r.Method == http.MethodDelete:
		h.clearFrames(w, access)
	case path == "state" && r.Method == http.MethodGet:
		h.state(w)
	case path == "channels" && r.Method == http.MethodGet:
		h.channels(w)
	case path == "channels" && r.Method == http.MethodPost:
		h.createChannel(w, r, access)
	case len(parts) == 2 && parts[0] == "channels" && r.Method == http.MethodDelete:
		h.deleteChannel(w, access, parts[1])
	case len(parts) == 3 && parts[0] == "channels" && parts[2] == "restart" && r.Method == http.MethodPost:
		h.restartChannel(w, access, parts[1])
	case path == "alarms" && r.Method == http.MethodGet:
		h.alarms(w, r)
	case len(parts) == 3 && parts[0] == "alarms" && parts[2] == "ack" && r.Method == http.MethodPost:
		h.ackAlarm(w, r, access, parts[1])
	case len(parts) == 3 && parts[0] == "alarms" && parts[2] == "close" && r.Method == http.MethodPost:
		h.closeAlarm(w, access, parts[1])
	case len(parts) == 3 && parts[0] == "alarms" && parts[2] == "maintenance" && r.Method == http.MethodPost:
		h.maintainAlarm(w, r, access, parts[1])
	case len(parts) == 3 && parts[0] == "alarms" && parts[2] == "maintenance" && r.Method == http.MethodDelete:
		h.releaseAlarm(w, access, parts[1])
	case path == "history" && r.Method == http.MethodGet:
		h.history(w, r, false)
	case path == "history/export" && r.Method == http.MethodGet:
		h.history(w, r, true)
	case path == "runtime" && r.Method == http.MethodGet:
		h.runtimeStatus(w)
	case path == "scan" && r.Method == http.MethodPost:
		h.scan(w, r, access)
	case path == "identify" && r.Method == http.MethodPost:
		h.identify(w, r, access)
	case path == "profiles" && r.Method == http.MethodGet:
		h.profiles(w)
	case len(parts) == 3 && parts[0] == "profiles" && parts[2] == "points.csv" && r.Method == http.MethodGet:
		h.profileCSV(w, r, parts[1])
	case path == "allocation/preview" && r.Method == http.MethodPost:
		h.allocationPreview(w, r, access)
	case path == "acceptance/run" && r.Method == http.MethodPost:
		writeError(w, http.StatusNotImplemented, "not_implemented")
	case path == "acceptance/latest" && r.Method == http.MethodGet:
		writeError(w, http.StatusNotFound, "no_report")
	default:
		writeError(w, http.StatusNotFound, "not_found")
	}
}

type valueDTO struct {
	Kind   string   `json:"kind"`
	Number *float64 `json:"number,omitempty"`
	Bool   *bool    `json:"bool,omitempty"`
	Enum   string   `json:"enum,omitempty"`
}

func toDTO(value signal.Value) valueDTO {
	dto := valueDTO{Kind: kindName(value.Kind)}
	switch value.Kind {
	case signal.Number:
		number := value.Num
		dto.Number = &number
	case signal.Bool:
		flag := value.Bool
		dto.Bool = &flag
	case signal.Enum:
		dto.Enum = value.Enum
	}
	return dto
}

func kindName(kind signal.Kind) string {
	switch kind {
	case signal.Number:
		return "number"
	case signal.Bool:
		return "bool"
	case signal.Enum:
		return "enum"
	default:
		return "unknown"
	}
}

func (h *Handler) state(w http.ResponseWriter) {
	values := h.Service.Snapshot()
	type entry struct {
		DeviceID     string    `json:"device_id"`
		PointID      string    `json:"point_id"`
		Value        valueDTO  `json:"value"`
		Quality      string    `json:"quality"`
		SourceTime   time.Time `json:"source_time"`
		ReceivedTime time.Time `json:"received_time"`
	}
	entries := make([]entry, 0, len(values))
	for _, value := range values {
		entries = append(entries, entry{
			DeviceID: value.DeviceID, PointID: value.PointID,
			Value: toDTO(value.Value), Quality: value.Quality.String(),
			SourceTime: value.SourceTime, ReceivedTime: value.ReceivedTime,
		})
	}
	writeJSON(w, map[string]any{"values": entries})
}

type pointView struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Function string  `json:"function"`
	Address  int     `json:"address"`
	Format   string  `json:"format"`
	Scale    float64 `json:"scale"`
	Offset   float64 `json:"offset"`
	Kind     string  `json:"kind"`
	ReadOnly bool    `json:"read_only"`
}

type deviceView struct {
	ID       string      `json:"id"`
	ServerID int         `json:"server_id"`
	Health   string      `json:"health"`
	Points   []pointView `json:"points"`
}

type channelView struct {
	ID        string       `json:"id"`
	NodeID    string       `json:"node_id"`
	Persisted bool         `json:"persisted"`
	Protocol  string       `json:"protocol"`
	Address   string       `json:"address"`
	PeriodMS  int          `json:"period_ms"`
	Devices   []deviceView `json:"devices"`
}

func (h *Handler) channels(w http.ResponseWriter) {
	now := time.Now()
	running := make(map[string]*channel.Channel)
	for _, ch := range h.Service.Channels() {
		running[ch.ID] = ch
	}
	views := make([]channelView, 0)
	seen := make(map[string]bool)

	if h.Buses != nil {
		if configs, err := h.Buses(); err == nil {
			for _, config := range configs {
				view := channelView{
					ID: config.ChannelID, NodeID: config.ChannelID, Persisted: true,
					Protocol: strings.ToUpper(config.Protocol), Address: config.Address,
					PeriodMS: int(config.Period / time.Millisecond),
				}
				ch := running[config.ChannelID]
				for _, dev := range config.Devices {
					item := deviceView{ID: dev.ID, ServerID: int(dev.ServerID), Health: string(channel.HealthUnknown)}
					if ch != nil {
						item.Health = string(ch.Health(dev.ID, now, h.MaxAge))
					}
					for _, point := range dev.Points {
						item.Points = append(item.Points, pointView{
							ID: point.ID, Name: point.Name, Function: string(point.Function),
							Address: int(point.Address), Format: point.Format,
							Scale: point.Scale, Offset: point.Offset,
							Kind: kindName(point.Kind), ReadOnly: point.ReadOnly,
						})
					}
					view.Devices = append(view.Devices, item)
				}
				views = append(views, view)
				seen[config.ChannelID] = true
			}
		}
	}

	for id, ch := range running {
		if seen[id] {
			continue
		}
		view := channelView{ID: id, NodeID: id, Persisted: false, Protocol: "TCP"}
		for _, dev := range ch.Devices {
			item := deviceView{
				ID: dev.ID, ServerID: int(dev.ServerID),
				Health: string(ch.Health(dev.ID, now, h.MaxAge)),
			}
			for _, point := range dev.Points {
				item.Points = append(item.Points, pointView{
					ID: point.ID, Name: point.Name, Function: string(point.Function),
					Address: int(point.Address), Format: point.Format,
					Scale: point.Scale, Offset: point.Offset,
					Kind: kindName(point.Kind), ReadOnly: point.ReadOnly,
				})
			}
			view.Devices = append(view.Devices, item)
		}
		views = append(views, view)
	}

	sort.Slice(views, func(i, j int) bool { return views[i].ID < views[j].ID })
	writeJSON(w, map[string]any{"channels": views})
}

func (h *Handler) restartChannel(w http.ResponseWriter, access Access, id string) {
	if !access.Admin && !access.Configure {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if h.RestartChannel == nil {
		writeError(w, http.StatusNotImplemented, "restart_unavailable")
		return
	}
	if err := h.RestartChannel(id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, map[string]any{"restarted": true, "channel_id": id})
}

func (h *Handler) runtimeStatus(w http.ResponseWriter) {
	channels := h.Service.Channels()
	devices := 0
	points := 0
	for _, ch := range channels {
		devices += len(ch.Devices)
		for _, device := range ch.Devices {
			points += len(device.Points)
		}
	}
	writeJSON(w, map[string]any{
		"source": "nodes", "channels": len(channels),
		"devices": devices, "points": points,
	})
}

type PointInput struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Function string  `json:"function"`
	Address  int     `json:"address"`
	Format   string  `json:"format"`
	Scale    float64 `json:"scale"`
	Offset   float64 `json:"offset"`
	Kind     string  `json:"kind"`
	ReadOnly bool    `json:"read_only"`
	Units    string  `json:"units"`
}

type DeviceInput struct {
	ID       string       `json:"id"`
	ServerID int          `json:"server_id"`
	Points   []PointInput `json:"points"`
}

type ChannelInput struct {
	ID        string        `json:"id"`
	Parent    string        `json:"parent"`
	Protocol  string        `json:"protocol"`
	Address   string        `json:"address"`
	Baud      int           `json:"baud"`
	PeriodMS  int           `json:"period_ms"`
	TimeoutMS int           `json:"timeout_ms"`
	Devices   []DeviceInput `json:"devices"`
}

func (h *Handler) createChannel(w http.ResponseWriter, r *http.Request, access Access) {
	if !access.Admin && !access.Configure {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input ChannelInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Parent == "" || input.Address == "" || len(input.Devices) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_channel")
		return
	}
	if h.Configure == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented")
		return
	}
	result, err := h.Configure.Create(access, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, result)
}

func convertPoint(input PointInput) (channel.Point, error) {
	kind := signal.Number
	switch input.Kind {
	case "number", "":
		kind = signal.Number
	case "bool":
		kind = signal.Bool
	default:
		return channel.Point{}, fmt.Errorf("unsupported point kind %q", input.Kind)
	}
	if input.Address < 0 || input.Address > 65535 {
		return channel.Point{}, fmt.Errorf("point %s: address out of range", input.ID)
	}
	function := channel.Function(input.Function)
	switch function {
	case channel.Coil, channel.DiscreteInput, channel.HoldingRegister, channel.InputRegister:
	default:
		return channel.Point{}, fmt.Errorf("point %s: unsupported function %q", input.ID, input.Function)
	}
	return channel.Point{
		ID: input.ID, Name: input.Name, Function: function, Address: uint16(input.Address),
		Format: input.Format, Scale: input.Scale, Offset: input.Offset,
		Kind: kind, ReadOnly: input.ReadOnly,
	}, nil
}

func (h *Handler) deleteChannel(w http.ResponseWriter, access Access, id string) {
	if !access.Admin && !access.Configure {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if h.Configure == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented")
		return
	}
	if err := h.Configure.Delete(access, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	// Stop the runtime channel immediately; the bridge also drops it on the
	// next reconcile, this just avoids a visible half-deleted state.
	h.Service.RemoveChannel(id)
	writeJSON(w, map[string]any{"deleted": id})
}

// ScanInput is a read-only slave scan request.
type ScanInput struct {
	Protocol  string `json:"protocol"`
	Address   string `json:"address"`
	Baud      int    `json:"baud"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	TimeoutMS int    `json:"timeout_ms"`
}

// IdentifyInput asks for a profile match or a manual profile choice.
type IdentifyInput struct {
	Protocol  string `json:"protocol"`
	Address   string `json:"address"`
	Baud      int    `json:"baud"`
	ServerID  int    `json:"server_id"`
	ProfileID string `json:"profile_id"`
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request, access Access) {
	if !access.Admin && !access.Configure {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input ScanInput
	if !decodeJSON(w, r, &input) {
		return
	}
	protocol := strings.ToLower(input.Protocol)
	if protocol == "" {
		protocol = "tcp"
	}
	if input.Address == "" {
		writeError(w, http.StatusBadRequest, "invalid_address")
		return
	}
	from, to := input.From, input.To
	if from == 0 {
		from = 1
	}
	if to == 0 {
		to = 16
	}
	if from < 1 || to > 247 || from > to {
		writeError(w, http.StatusBadRequest, "invalid_range")
		return
	}
	if to-from+1 > 64 {
		writeError(w, http.StatusBadRequest, "range_too_large")
		return
	}
	timeout := input.TimeoutMS
	if timeout == 0 {
		timeout = 300
	}
	if timeout < 50 || timeout > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_timeout")
		return
	}
	if h.busy(protocol, input.Address) {
		writeError(w, http.StatusConflict, "channel_busy")
		return
	}
	bus, err := h.OpenBus(protocol, input.Address, time.Duration(timeout)*time.Millisecond)
	if err != nil {
		writeError(w, http.StatusBadGateway, "connect_failed: "+err.Error())
		return
	}
	if closer, ok := bus.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}

	type finding struct {
		ServerID  int    `json:"server_id"`
		Responded bool   `json:"responded"`
		LatencyMS int64  `json:"latency_ms"`
		Probe     string `json:"probe"`
		Error     string `json:"error"`
	}
	found := make([]finding, 0)
	for unit := from; unit <= to; unit++ {
		start := time.Now()
		probe := "holdingRegister:0"
		_, probeErr := bus.ReadHoldingRegisters(byte(unit), 0, 1)
		if probeErr != nil {
			probe = "inputRegister:0"
			_, probeErr = bus.ReadInputRegisters(byte(unit), 0, 1)
		}
		if probeErr == nil {
			found = append(found, finding{
				ServerID: unit, Responded: true,
				LatencyMS: time.Since(start).Milliseconds(), Probe: probe,
			})
		}
	}
	writeJSON(w, map[string]any{"scanned": to - from + 1, "found": found, "errors": []string{}})
}

func (h *Handler) identify(w http.ResponseWriter, r *http.Request, access Access) {
	if !access.Admin && !access.Configure {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input IdentifyInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.ServerID < 1 || input.ServerID > 247 {
		writeError(w, http.StatusBadRequest, "invalid_server")
		return
	}
	if h.Profiles == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented")
		return
	}
	if input.ProfileID != "" {
		chosen, ok := h.Profiles.Get(input.ProfileID)
		if !ok {
			writeError(w, http.StatusNotFound, "unknown_profile")
			return
		}
		writeJSON(w, map[string]any{
			"server_id": input.ServerID, "matched": true,
			"profile": chosen, "candidates": []any{}, "detail": "人工选择 Profile",
		})
		return
	}

	protocol := strings.ToLower(input.Protocol)
	if protocol == "" {
		protocol = "tcp"
	}
	var bus channel.Bus
	read := func(identity profile.Identity) ([]uint16, error) {
		if bus == nil {
			opened, err := h.OpenBus(protocol, input.Address, 500*time.Millisecond)
			if err != nil {
				return nil, err
			}
			bus = opened
		}
		switch identity.Function {
		case channel.HoldingRegister:
			return bus.ReadHoldingRegisters(byte(input.ServerID), identity.Address, identity.Count)
		case channel.InputRegister:
			return bus.ReadInputRegisters(byte(input.ServerID), identity.Address, identity.Count)
		default:
			return nil, fmt.Errorf("unsupported identity function %q", identity.Function)
		}
	}
	candidates := h.Profiles.Identify(read)
	if closer, ok := bus.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	if len(candidates) == 0 {
		writeJSON(w, map[string]any{
			"server_id": input.ServerID, "matched": false, "profile": nil,
			"candidates": []any{}, "detail": "没有读到内置 Profile 的特征寄存器，可人工选择或继续手动配置",
		})
		return
	}
	type candidateDTO struct {
		Profile  profile.Profile `json:"profile"`
		Score    int             `json:"score"`
		Evidence string          `json:"evidence"`
		Strong   bool            `json:"strong"`
	}
	dtos := make([]candidateDTO, 0, len(candidates))
	for _, candidate := range candidates {
		dtos = append(dtos, candidateDTO{Profile: candidate.Profile, Score: candidate.Score, Evidence: candidate.Evidence, Strong: candidate.Strong})
	}
	top := candidates[0]
	if top.Strong {
		writeJSON(w, map[string]any{
			"server_id": input.ServerID, "matched": true,
			"profile": top.Profile, "candidates": dtos, "detail": "自动匹配：" + top.Evidence,
		})
		return
	}
	writeJSON(w, map[string]any{
		"server_id": input.ServerID, "matched": false, "profile": nil,
		"candidates": dtos, "detail": "读到特征寄存器但未识别到型号文本，请按候选人工确认：" + top.Evidence,
	})
}

func (h *Handler) profiles(w http.ResponseWriter) {
	list := []profile.Profile{}
	if h.Profiles != nil {
		list = h.Profiles.List()
	}
	writeJSON(w, map[string]any{"profiles": list})
}

func (h *Handler) profileCSV(w http.ResponseWriter, r *http.Request, id string) {
	if h.Profiles == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented")
		return
	}
	chosen, ok := h.Profiles.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown_profile")
		return
	}
	serverID := 1
	if text := r.URL.Query().Get("server_id"); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 1 || parsed > 247 {
			writeError(w, http.StatusBadRequest, "invalid_server")
			return
		}
		serverID = parsed
	}
	data := chosen.CSV(serverID)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+"-points.csv"))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// AllocationMember is one member of an allocation preview.
type AllocationMember struct {
	ID                 string  `json:"id"`
	Online             bool    `json:"online"`
	Faulted            bool    `json:"faulted"`
	Disabled           bool    `json:"disabled"`
	MinPower           float64 `json:"min_power"`
	MaxCharge          float64 `json:"max_charge"`
	MaxDischarge       float64 `json:"max_discharge"`
	AvailableCharge    float64 `json:"available_charge"`
	AvailableDischarge float64 `json:"available_discharge"`
}

// AllocationInput is a calculation-only request.
type AllocationInput struct {
	Target  float64            `json:"target"`
	Members []AllocationMember `json:"members"`
}

// allocationPreview computes the split without touching any device.
func (h *Handler) allocationPreview(w http.ResponseWriter, r *http.Request, _ Access) {
	var input AllocationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.Members) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_members")
		return
	}
	members := make([]alloc.Member, 0, len(input.Members))
	for _, member := range input.Members {
		members = append(members, alloc.Member{
			ID: member.ID, Online: member.Online, Faulted: member.Faulted, Disabled: member.Disabled,
			MinPower: member.MinPower, MaxCharge: member.MaxCharge, MaxDischarge: member.MaxDischarge,
			AvailableCharge: member.AvailableCharge, AvailableDischarge: member.AvailableDischarge,
		})
	}
	result, err := alloc.Allocate(input.Target, members)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_target")
		return
	}
	writeJSON(w, map[string]any{"preview": true, "result": result})
}

func (h *Handler) busy(protocol, address string) bool {
	if h.Buses == nil {
		return false
	}
	configs, err := h.Buses()
	if err != nil {
		return false
	}
	for _, config := range configs {
		if !strings.EqualFold(config.Protocol, protocol) {
			continue
		}
		if protocol == "rtu" {
			scanPort, _, _ := strings.Cut(address, "@")
			busPort, _, _ := strings.Cut(config.Address, "@")
			if scanPort == busPort {
				return true
			}
			continue
		}
		if strings.EqualFold(config.Address, address) {
			return true
		}
	}
	return false
}

func (h *Handler) alarms(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	alarms := h.Service.Alarms.List()
	if state == "active" {
		active := make([]any, 0, len(alarms))
		for _, alarm := range alarms {
			if alarm.Active {
				active = append(active, alarm)
			}
		}
		writeJSON(w, map[string]any{"alarms": active})
		return
	}
	writeJSON(w, map[string]any{"alarms": alarms})
}

func (h *Handler) ackAlarm(w http.ResponseWriter, r *http.Request, access Access, id string) {
	var input struct {
		Note string `json:"note"`
	}
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &input) {
			return
		}
	}
	alarm, err := h.Service.Alarms.AcknowledgeWithNote(time.Now(), id, access.actor(), input.Note)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, alarm)
}

func (h *Handler) closeAlarm(w http.ResponseWriter, access Access, id string) {
	alarm, err := h.Service.Alarms.Close(time.Now(), id)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, alarm)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request, export bool) {
	deviceID := r.URL.Query().Get("device_id")
	pointID := r.URL.Query().Get("point_id")
	if deviceID == "" || pointID == "" {
		writeError(w, http.StatusBadRequest, "device_id_and_point_id_required")
		return
	}
	start, err := parseTime(r.URL.Query().Get("start"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_start")
		return
	}
	end, err := parseTime(r.URL.Query().Get("end"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_end")
		return
	}
	limit := 500
	if text := r.URL.Query().Get("limit"); text != "" {
		limit, err = strconv.Atoi(text)
		if err != nil || limit < 1 || limit > 5000 {
			writeError(w, http.StatusBadRequest, "invalid_limit")
			return
		}
	}
	samples := h.Service.History.Query(deviceID, pointID, start, end, limit)
	if export {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="history.csv"`)
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"时间", "设备", "点位", "数值", "质量"})
		for _, sample := range samples {
			_ = writer.Write([]string{
				sample.SourceTime.Format(time.RFC3339), sample.DeviceID, sample.PointID,
				sample.Value.String(), sample.Quality.String(),
			})
		}
		writer.Flush()
		return
	}
	type entry struct {
		Value      valueDTO  `json:"value"`
		Quality    string    `json:"quality"`
		SourceTime time.Time `json:"source_time"`
	}
	entries := make([]entry, 0, len(samples))
	for _, sample := range samples {
		entries = append(entries, entry{
			Value: toDTO(sample.Value), Quality: sample.Quality.String(), SourceTime: sample.SourceTime,
		})
	}
	writeJSON(w, map[string]any{"device_id": deviceID, "point_id": pointID, "samples": entries})
}

func parseTime(text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, text)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (h *Handler) frames(w http.ResponseWriter, r *http.Request, access Access) {
	if !access.Operate && !access.Configure && !access.Admin {
		writeError(w, http.StatusForbidden, "operator_required")
		return
	}
	limit := 200
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	frames := framelog.Default.List(limit)
	out := make([]contract.Frame, 0, len(frames))
	for _, frame := range frames {
		out = append(out, contract.Frame{
			ID: frame.ID, Time: frame.Time, Peer: frame.Peer,
			Direction: frame.Direction, Hex: frame.Hex,
		})
	}
	writeJSON(w, map[string]any{"frames": out})
}

func (h *Handler) clearFrames(w http.ResponseWriter, access Access) {
	if !access.Operate && !access.Configure && !access.Admin {
		writeError(w, http.StatusForbidden, "operator_required")
		return
	}
	framelog.Default.Clear()
	writeJSON(w, map[string]bool{"cleared": true})
}

func (h *Handler) quality(w http.ResponseWriter) {
	now := time.Now()
	channels := h.Service.Channels()
	channelStats := make([]contract.QualityStats, 0, len(channels))
	deviceStats := make([]contract.QualityStats, 0)
	for _, ch := range channels {
		aggregate := contract.QualityStats{ChannelID: ch.ID, UpdatedAt: now}
		var totalLatency, maxLatency float64
		var latencyCount int
		for _, device := range ch.Devices {
			stats := ch.Stats(device.ID)
			item := contract.QualityStats{
				ChannelID: ch.ID, DeviceID: device.ID, Health: string(ch.Health(device.ID, now, h.MaxAge)),
				Polls: stats.Requests, Failures: stats.Failures,
				LastLatencyMS: stats.LatencyLastMS, LastSuccess: stats.LastSuccess,
				LastError: stats.LastError, UpdatedAt: now,
			}
			if stats.Requests > 0 {
				item.SuccessRate = float64(stats.Requests-stats.Failures) / float64(stats.Requests) * 100
			}
			if stats.LatencyCount > 0 {
				item.AvgLatencyMS = stats.LatencyTotalMS / float64(stats.LatencyCount)
			}
			item.MaxLatencyMS = stats.LatencyMaxMS
			deviceStats = append(deviceStats, item)
			aggregate.Polls += stats.Requests
			aggregate.Failures += stats.Failures
			totalLatency += stats.LatencyTotalMS
			latencyCount += stats.LatencyCount
			if stats.LatencyMaxMS > maxLatency {
				maxLatency = stats.LatencyMaxMS
			}
			if stats.LastSuccess.After(aggregate.LastSuccess) {
				aggregate.LastSuccess = stats.LastSuccess
			}
			if stats.LastError != "" {
				aggregate.LastError = stats.LastError
			}
		}
		if aggregate.Polls > 0 {
			aggregate.SuccessRate = float64(aggregate.Polls-aggregate.Failures) / float64(aggregate.Polls) * 100
		}
		if latencyCount > 0 {
			aggregate.AvgLatencyMS = totalLatency / float64(latencyCount)
		}
		aggregate.MaxLatencyMS = maxLatency
		channelStats = append(channelStats, aggregate)
	}
	writeJSON(w, map[string]any{"channels": channelStats, "devices": deviceStats})
}

func (h *Handler) maintainAlarm(w http.ResponseWriter, r *http.Request, access Access, id string) {
	if !access.Operate && !access.Configure && !access.Admin {
		writeError(w, http.StatusForbidden, "operator_required")
		return
	}
	var input struct {
		Until  string `json:"until"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	until, err := time.Parse(time.RFC3339, input.Until)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_maintenance_until")
		return
	}
	alarm, err := h.Service.Alarms.Shelve(time.Now(), id, input.Reason, access.actor(), until)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	h.persistMaintenance()
	writeJSON(w, alarm)
}

func (h *Handler) releaseAlarm(w http.ResponseWriter, access Access, id string) {
	if !access.Operate && !access.Configure && !access.Admin {
		writeError(w, http.StatusForbidden, "operator_required")
		return
	}
	alarm, err := h.Service.Alarms.Release(time.Now(), id)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	h.persistMaintenance()
	writeJSON(w, alarm)
}

func (h *Handler) persistMaintenance() {
	if h.PersistMaintenance != nil {
		h.PersistMaintenance()
	}
}
