package appapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/simpleiot/simpleiot/api"
	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/audit"
	"easyems/internal/decision"
	"easyems/internal/emsapi"
)

type Handler struct {
	nc          *nats.Conn
	users       api.UserAuthority
	ready       *atomic.Bool
	commandMu   sync.Mutex
	ownershipMu sync.Mutex
	ems         *emsapi.Handler
	opcua       *OpcuaManager

	siteGoalMu    sync.Mutex
	siteGoalCache map[string]siteGoalEntry

	forecastMu   sync.Mutex
	forecastLast map[string]time.Time

	acceptanceMu     sync.Mutex
	latestAcceptance *acceptanceReport
	audit            *audit.Store
	dataDir          string
	alarmEventsMu    sync.Mutex

	sessionMu      sync.Mutex
	sessions       map[string]sessionState
	sessionTimeout time.Duration
	maxSessions    int
}

func New(nc *nats.Conn, users api.UserAuthority, ready *atomic.Bool) *Handler {
	return &Handler{
		nc: nc, users: users, ready: ready,
		sessions:       map[string]sessionState{},
		sessionTimeout: 30 * time.Minute,
		maxSessions:    5,
	}
}

// SetEMS mounts the EMS runtime API under /api/ems/.
// SetAudit wires the local audit store. A nil store disables audit.
func (h *Handler) SetAudit(store *audit.Store) { h.audit = store }

// SetDataDir tells the handler where local runtime settings are stored.
func (h *Handler) SetDataDir(dir string) { h.dataDir = dir; h.applyAuditSettings() }

// SetOpcua 注入 OPC-UA runner 管理器。
func (h *Handler) SetOpcua(manager *OpcuaManager) { h.opcua = manager }

func (h *Handler) SetEMS(handler *emsapi.Handler) {
	h.ems = handler
	if handler != nil && handler.Service != nil {
		handler.Service.Commands().SetProfiles(h.commandProfile)
		handler.Service.Alarms.OnChange = h.recordAlarmChange
	}
}

// SetSessionPolicy tunes idle session expiry and the per-account concurrent
// session ceiling. Non-positive values keep the current defaults.
func (h *Handler) SetSessionPolicy(timeout time.Duration, maxSessions int) {
	if timeout > 0 {
		h.sessionTimeout = timeout
	}
	if maxSessions > 0 {
		h.maxSessions = maxSessions
	}
}

type sessionState struct {
	userID   string
	lastSeen time.Time
}

func (h *Handler) resolveSession(id string) (string, bool) {
	h.sessionMu.Lock()
	defer h.sessionMu.Unlock()
	if h.sessions == nil {
		return "", false
	}
	state, ok := h.sessions[id]
	if !ok {
		return "", false
	}
	if h.sessionTimeout > 0 && time.Since(state.lastSeen) > h.sessionTimeout {
		delete(h.sessions, id)
		return "", false
	}
	state.lastSeen = time.Now()
	h.sessions[id] = state
	return state.userID, true
}

var errSessionLimit = errors.New("session_limit_reached")

func (h *Handler) registerSession(userID string) (string, error) {
	h.sessionMu.Lock()
	defer h.sessionMu.Unlock()
	if h.sessions == nil {
		h.sessions = map[string]sessionState{}
	}
	now := time.Now()
	for id, state := range h.sessions {
		if h.sessionTimeout > 0 && now.Sub(state.lastSeen) > h.sessionTimeout {
			delete(h.sessions, id)
		}
	}
	if h.maxSessions > 0 {
		active := 0
		for _, state := range h.sessions {
			if state.userID == userID {
				active++
			}
		}
		if active >= h.maxSessions {
			return "", errSessionLimit
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	h.sessions[id] = sessionState{userID: userID, lastSeen: now}
	return id, nil
}

type AccessPolicy struct {
	Admin       bool     `json:"admin"`
	Role        string   `json:"role"`
	RootNodeIDs []string `json:"root_node_ids"`
	Actions     []string `json:"actions,omitempty"`
}

type Identity struct {
	UserID  string       `json:"user_id"`
	Email   string       `json:"email,omitempty"`
	Policy  AccessPolicy `json:"policy"`
	Anchors []string     `json:"-"`
}

// Implementers commission the station: channels, devices, points, profiles,
// rules, screens, allocators and acceptance. Admins additionally manage users
// and delivery. Operators only run and acknowledge within their scope.
func canConfigure(identity Identity) bool {
	return identity.Policy.Admin || identity.Policy.Role == "implementer"
}

func canOperateIdentity(identity Identity) bool {
	return identity.Policy.Admin || identity.Policy.Role == "operator" || identity.Policy.Role == "implementer"
}

// Action constants. A role is only a shortcut that expands to a default action
// set; an explicit Actions whitelist on the policy overrides the role.
const (
	ActionConfigure     = "configure"      // 通道/设备/点位/规则草稿/画面/分配器/聚合/报警类型/验收/模板
	ActionWriteValue    = "write_value"    // 控制写入（含人工接管）
	ActionCommand       = "command"        // 命令执行与通信报文诊断
	ActionRulePublish   = "rule_publish"   // 规则发布/启用/恢复
	ActionAlarmAck      = "alarm_ack"      // 报警确认/关闭
	ActionAlarmMaintain = "alarm_maintain" // 报警维护/结束维护
	ActionDelete        = "delete"         // 删除节点/通道
	ActionExport        = "export"         // 导出点表/验证/证据/历史
	ActionAccountManage = "account_manage" // 账号管理（创建/重置密码）
	ActionAuditView     = "audit_view"     // 审计查看/导出
	ActionSiteSettings  = "site_settings"  // 站点设置（电价/历史保留）
)

// allActions is the canonical action list, exposed to the frontend and reused
// to build the resolved capability set passed to the EMS API.
var allActions = []string{
	ActionConfigure, ActionWriteValue, ActionCommand, ActionRulePublish,
	ActionAlarmAck, ActionAlarmMaintain, ActionDelete, ActionExport,
	ActionAccountManage, ActionAuditView, ActionSiteSettings,
}

// canAction is the single permission decision point. Admin always passes; an
// explicit whitelist overrides the role default; otherwise the role's default
// action set applies. Unknown actions are denied.
func canAction(identity Identity, action string) bool {
	if identity.Policy.Admin {
		return true
	}
	if len(identity.Policy.Actions) > 0 {
		for _, allowed := range identity.Policy.Actions {
			if allowed == action {
				return true
			}
		}
		return false
	}
	return roleAllows(identity.Policy, action)
}

// roleAllows expands a role into its default action set via the two kept
// role-default predicates, so action granularity never drifts from them.
func roleAllows(policy AccessPolicy, action string) bool {
	switch action {
	case ActionConfigure, ActionRulePublish, ActionDelete, ActionExport:
		return canConfigure(Identity{Policy: policy})
	case ActionWriteValue, ActionCommand, ActionAlarmAck, ActionAlarmMaintain:
		return canOperateIdentity(Identity{Policy: policy})
	case ActionAccountManage, ActionAuditView, ActionSiteSettings:
		return policy.Admin
	default:
		return false
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	w = status
	actor := ""
	defer func() {
		if h.audit != nil && shouldAudit(r) {
			h.recordAudit(actor, r, status.status)
		}
	}()
	path := strings.Trim(r.URL.Path, "/")
	if path == "health" {
		if !h.ready.Load() {
			writeError(w, http.StatusServiceUnavailable, "starting")
			return
		}
		writeJSON(w, map[string]any{"ready": true, "version": "0.1.0"})
		return
	}
	if !h.ready.Load() {
		writeError(w, http.StatusServiceUnavailable, "starting")
		return
	}
	if path == "login" && r.Method == http.MethodPost {
		h.login(w, r)
		return
	}
	identity, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	actor = identity.Email
	if actor == "" {
		actor = identity.UserID
	}
	if h.ems != nil && strings.HasPrefix(path, "ems/") &&
		!strings.HasPrefix(path, "ems/screen/") &&
		!strings.HasPrefix(path, "ems/acceptance/") &&
		!strings.HasPrefix(path, "ems/allocators/") {
		actions := map[string]bool{}
		for _, action := range allActions {
			actions[action] = canAction(identity, action)
		}
		h.ems.ServeHTTP(w, r, emsapi.Access{
			UserID: identity.UserID, Actor: identity.Email, Admin: identity.Policy.Admin,
			Configure:     actions[ActionConfigure],
			Command:       actions[ActionCommand],
			AlarmAck:      actions[ActionAlarmAck],
			AlarmMaintain: actions[ActionAlarmMaintain],
			Delete:        actions[ActionDelete],
			AllowNode:     func(id string) bool { return h.allows(identity, id) },
		})
		return
	}
	parts := strings.Split(path, "/")
	switch {
	case path == "ws":
		h.serveWS(w, r, identity)
	case path == "metrics" && r.Method == http.MethodGet:
		h.serveMetrics(w, identity)
	case path == "snapshot" && r.Method == http.MethodGet:
		h.snapshot(w, identity)
	case path == "audit-settings" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.auditSettings(w, r, identity)
	case path == "audit" && r.Method == http.MethodGet:
		h.auditList(w, r, identity)
	case path == "audit/export" && r.Method == http.MethodGet:
		h.auditExport(w, r, identity)
	case path == "history-settings" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.historySettings(w, r, identity)
	case path == "profile-verifications" && r.Method == http.MethodGet:
		h.profileVerifications(w, r, identity)
	case path == "alarm-stats" && r.Method == http.MethodGet:
		h.alarmStats(w, r, identity)
	case path == "device-ledger" && r.Method == http.MethodGet:
		h.deviceLedger(w, r, identity)
	case path == "delivery/check" && r.Method == http.MethodGet:
		h.deliveryCheck(w, identity)
	case path == "delivery/evidence" && r.Method == http.MethodGet:
		h.deliveryEvidence(w, r, identity)
	case path == "delivery/gates" && r.Method == http.MethodGet:
		h.deliveryGates(w, identity)
	case path == "delivery/preflight" && r.Method == http.MethodPost:
		h.runPreflight(w, identity)
	case path == "delivery/preflight" && r.Method == http.MethodGet:
		h.latestPreflight(w, identity)
	case path == "projects/reuse/preview" && r.Method == http.MethodGet:
		h.reusePreviewHandler(w, identity, r.URL.Query().Get("source"))
	case path == "projects/reuse" && r.Method == http.MethodPost:
		var req reuseRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		h.reuseProjectHandler(w, identity, req)
	case path == "projects/publish" && r.Method == http.MethodPost:
		var req publishRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		h.publishHandler(w, identity, req.Site, req.Note)
	case path == "projects/versions" && r.Method == http.MethodGet:
		h.versionsHandler(w, identity, r.URL.Query().Get("site"))
	case path == "projects/rollback" && r.Method == http.MethodPost:
		var req rollbackRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		h.rollbackHandler(w, identity, req.Site, req.Version)
	case path == "demand" && r.Method == http.MethodGet:
		h.demandHandler(w, identity, r.URL.Query().Get("site"))
	case path == "projects/plan96" && r.Method == http.MethodGet:
		h.plan96Handler(w, identity, r.URL.Query().Get("site"))
	case path == "projects/plan96" && r.Method == http.MethodPut:
		var req plan96Request
		if !decodeJSON(w, r, &req) {
			return
		}
		h.savePlan96Handler(w, identity, req)
	case path == "history" && r.Method == http.MethodGet:
		h.history(w, r, identity, false)
	case path == "history/export" && r.Method == http.MethodGet:
		h.history(w, r, identity, true)
	case path == "templates" && r.Method == http.MethodPost:
		h.saveTemplate(w, r, identity)
	case len(parts) == 3 && parts[0] == "templates" && parts[2] == "apply" && r.Method == http.MethodPost:
		h.applyTemplate(w, r, identity, parts[1])
	case path == "modbus" && r.Method == http.MethodPost:
		h.createModbus(w, r, identity)
	case path == "modbus/test" && r.Method == http.MethodPost:
		h.testModbus(w, r, identity)
	case len(parts) == 3 && parts[0] == "modbus" && parts[2] == "line-test" && r.Method == http.MethodPost:
		h.lineTestModbus(w, r, identity, parts[1])
	case len(parts) == 2 && parts[0] == "modbus" && r.Method == http.MethodPut:
		h.updateModbus(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "modbus" && parts[1] == "io" && r.Method == http.MethodPut:
		h.updateModbusIO(w, r, identity, parts[2])
	case len(parts) == 4 && parts[0] == "modbus" && parts[2] == "io" && parts[3] == "export" && r.Method == http.MethodGet:
		h.exportModbusCSV(w, identity, parts[1])
	case len(parts) == 4 && parts[0] == "modbus" && parts[2] == "io" && parts[3] == "import" && r.Method == http.MethodPost:
		h.importModbusCSV(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "modbus" && parts[2] == "io" && r.Method == http.MethodPost:
		h.createModbusIO(w, r, identity, parts[1])
	case len(parts) == 5 && parts[0] == "modbus" && parts[2] == "units" && parts[4] == "model" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		server, err := strconv.Atoi(parts[3])
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_station")
			return
		}
		h.unitModel(w, r, identity, parts[1], server)
	case len(parts) == 4 && parts[0] == "modbus" && parts[2] == "units" && parts[3] == "apply" && r.Method == http.MethodPost:
		h.applyProfilesHandler(w, r, identity, parts[1])
	case len(parts) == 5 && parts[0] == "modbus" && parts[2] == "units" && parts[4] == "commands" && (r.Method == http.MethodGet || r.Method == http.MethodPost):
		server, err := strconv.Atoi(parts[3])
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_station")
			return
		}
		h.unitCommands(w, r, identity, parts[1], server, "")
	case len(parts) == 6 && parts[0] == "modbus" && parts[2] == "units" && parts[4] == "commands" && r.Method == http.MethodGet:
		server, err := strconv.Atoi(parts[3])
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_station")
			return
		}
		h.unitCommands(w, r, identity, parts[1], server, parts[5])
	case path == "nodes" && r.Method == http.MethodPost:
		h.createNode(w, r, identity)
	case len(parts) == 2 && parts[0] == "nodes" && r.Method == http.MethodPatch:
		h.updateNode(w, r, identity, parts[1])
	case len(parts) == 2 && parts[0] == "nodes" && r.Method == http.MethodDelete:
		h.deleteNode(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "nodes" && parts[2] == "move" && r.Method == http.MethodPost:
		h.moveNode(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "nodes" && parts[2] == "value" && r.Method == http.MethodPost:
		h.writeValue(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "nodes" && parts[2] == "manual" && r.Method == http.MethodPost:
		h.setManual(w, r, identity, parts[1])
	case path == "rules" && r.Method == http.MethodPost:
		h.createRule(w, r, identity)
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "draft" && r.Method == http.MethodPut:
		h.saveDraft(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "publish" && r.Method == http.MethodPost:
		h.publishRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "copy" && r.Method == http.MethodPost:
		h.copyRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "restore" && r.Method == http.MethodPost:
		h.restoreRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "enable" && r.Method == http.MethodPost:
		h.enableRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "test" && r.Method == http.MethodPost:
		h.testRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "scenarios" && r.Method == http.MethodPost:
		h.runScenarios(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "diff" && r.Method == http.MethodPost:
		h.diffRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "replay" && r.Method == http.MethodPost:
		h.replayRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "rules" && parts[2] == "deps" && r.Method == http.MethodPost:
		h.ruleDeps(w, r, identity, parts[1])
	case path == "tariff" && r.Method == http.MethodPut:
		h.setTariff(w, r, identity)
	case path == "aggregate" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.aggregate(w, r, identity)
	case path == "energy-units" && r.Method == http.MethodGet:
		h.listEnergyUnits(w, identity)
	case path == "energy-units" && r.Method == http.MethodPost:
		h.createEnergyUnit(w, r, identity)
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "rule" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.energyRule(w, r, identity, parts[1])
	case len(parts) == 4 && parts[0] == "energy-units" && parts[2] == "rule" && parts[3] == "publish" && r.Method == http.MethodPost:
		h.publishCapabilityRule(w, r, identity, parts[1])
	case len(parts) == 4 && parts[0] == "energy-units" && parts[2] == "rule" && parts[3] == "simulate" && r.Method == http.MethodPost:
		h.simulateCapabilityRule(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "allocator" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.energyAllocator(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "devices" && r.Method == http.MethodGet:
		h.listEnergyDevices(w, identity, parts[1])
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "devices" && r.Method == http.MethodPost:
		h.createEnergyDevice(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "capability-inputs" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.capabilityInputs(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "forecast-inputs" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.forecastInputs(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "dispatch-records" && r.Method == http.MethodGet:
		h.dispatchRecords(w, identity, parts[1])
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "events" && r.Method == http.MethodPost:
		h.energyControl(w, r, identity, parts[1], "events")
	case len(parts) == 3 && parts[0] == "energy-units" && parts[2] == "mode" && r.Method == http.MethodPut:
		h.energyControl(w, r, identity, parts[1], "mode")
	case path == "resource-groups" && r.Method == http.MethodPost:
		h.createResourceGroup(w, r, identity)
	case path == "resource-groups" && r.Method == http.MethodGet:
		h.listResourceGroups(w, identity)
	case len(parts) == 3 && parts[0] == "resource-groups" && parts[2] == "dispatch" && r.Method == http.MethodPost:
		h.dispatchResourceGroup(w, r, identity, parts[1])
	case path == "opcua/channels" && (r.Method == http.MethodGet || r.Method == http.MethodPost):
		h.opcuaChannels(w, r, identity)
	case len(parts) == 4 && parts[0] == "opcua" && parts[1] == "channels" && parts[3] == "points" && (r.Method == http.MethodGet || r.Method == http.MethodPost):
		h.opcuaPoints(w, r, identity, parts[2])
	case len(parts) == 3 && parts[0] == "sites" && parts[2] == "target" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.siteTarget(w, r, identity, parts[1])
	case path == "plants" && r.Method == http.MethodGet:
		h.listPlants(w, identity)
	case len(parts) == 3 && parts[0] == "plants" && parts[2] == "pcc" && r.Method == http.MethodGet:
		h.getPlantPCC(w, identity, parts[1])
	case len(parts) == 3 && parts[0] == "plants" && parts[2] == "pcc" && r.Method == http.MethodPut:
		h.savePlantPCC(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "plants" && parts[2] == "phase-verdict" && r.Method == http.MethodGet:
		h.phaseVerdict(w, identity, parts[1])
	case len(parts) == 3 && parts[0] == "plants" && parts[2] == "strategy" && (r.Method == http.MethodGet || r.Method == http.MethodPut):
		h.plantStrategy(w, r, identity, parts[1])
	case path == "safety" && r.Method == http.MethodPut:
		h.putSafety(w, r, identity)
	case path == "alarm-types":
		h.alarmTypes(w, r, identity)
	case path == "users" && r.Method == http.MethodPost:
		h.createUser(w, r, identity)
	case path == "users" && r.Method == http.MethodGet:
		h.listUsers(w, identity)
	case len(parts) == 2 && parts[0] == "users" && r.Method == http.MethodPut:
		h.updateUserPolicy(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "users" && parts[2] == "password" && r.Method == http.MethodPost:
		h.resetUserPassword(w, r, identity, parts[1])
	case len(parts) == 3 && parts[0] == "ems" && parts[1] == "screen" && r.Method == http.MethodGet:
		h.getScreen(w, identity, parts[2])
	case len(parts) == 3 && parts[0] == "ems" && parts[1] == "screen" && r.Method == http.MethodPut:
		h.putScreen(w, r, identity, parts[2])
	case path == "ems/acceptance/run" && r.Method == http.MethodPost:
		h.runAcceptance(w, identity)
	case path == "ems/acceptance/latest" && r.Method == http.MethodGet:
		h.latestAcceptanceReport(w)
	case len(parts) == 3 && parts[0] == "ems" && parts[1] == "allocators" && r.Method == http.MethodGet:
		h.getAllocator(w, identity, parts[2])
	case len(parts) == 3 && parts[0] == "ems" && parts[1] == "allocators" && r.Method == http.MethodPut:
		h.putAllocator(w, r, identity, parts[2])
	case len(parts) == 4 && parts[0] == "ems" && parts[1] == "allocators" && parts[3] == "simulate" && r.Method == http.MethodPost:
		h.simulateAllocator(w, r, identity, parts[2])
	case len(parts) == 4 && parts[0] == "ems" && parts[1] == "allocators" && parts[3] == "history" && r.Method == http.MethodGet:
		h.allocatorHistory(w, identity, parts[2])
	default:
		writeError(w, http.StatusNotFound, "not_found")
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		h.auditLogin("", "failure", "invalid request", r)
		return
	}
	nodes, err := client.UserCheck(h.nc, input.Email, input.Password)
	if err != nil {
		h.auditLogin(input.Email, "failure", "authentication unavailable", r)
		writeError(w, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	var token string
	for _, node := range nodes {
		if point, ok := node.Points.Find(data.PointTypeToken, ""); ok {
			token = point.Txt()
		}
	}
	if token == "" {
		h.auditLogin(input.Email, "failure", "invalid credentials", r)
		writeError(w, http.StatusForbidden, "invalid_login")
		return
	}
	userID, _, ok := h.users.UserFromToken(token)
	if !ok {
		h.auditLogin(input.Email, "failure", "invalid credentials", r)
		writeError(w, http.StatusForbidden, "invalid_login")
		return
	}
	sessionID, err := h.registerSession(userID)
	if err != nil {
		h.auditLogin(input.Email, "failure", "session limit reached", r)
		writeError(w, http.StatusConflict, "session_limit_reached")
		return
	}
	h.auditLogin(input.Email, "success", "login", r)
	writeJSON(w, map[string]string{"token": sessionID})
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	sessionID, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || sessionID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return Identity{}, false
	}
	userID, ok := h.resolveSession(sessionID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return Identity{}, false
	}
	anchors := h.users.UserAnchors(userID)
	nodes, err := client.GetNodes(h.nc, "all", userID, "", false)
	if err != nil || len(nodes) == 0 || len(anchors) == 0 {
		writeError(w, http.StatusForbidden, "no_access")
		return Identity{}, false
	}
	email := ""
	for _, node := range nodes {
		if point, ok := node.Points.Find(data.PointTypeEmail, ""); ok && email == "" {
			email = point.Txt()
		}
	}
	var policy AccessPolicy
	for _, node := range nodes {
		if point, ok := node.Points.Find("easyemsAccess", ""); ok {
			if json.Unmarshal([]byte(point.Txt()), &policy) == nil {
				break
			}
		}
	}
	if !policy.Admin && policy.Role != "implementer" && policy.Role != "operator" && policy.Role != "viewer" {
		writeError(w, http.StatusForbidden, "no_access")
		return Identity{}, false
	}
	if !policy.Admin {
		allowed := make(map[string]bool, len(policy.RootNodeIDs))
		for _, root := range policy.RootNodeIDs {
			allowed[root] = true
		}
		kept := anchors[:0]
		for _, anchor := range anchors {
			if allowed[anchor] {
				kept = append(kept, anchor)
			}
		}
		anchors = kept
		if len(anchors) == 0 {
			writeError(w, http.StatusForbidden, "no_access")
			return Identity{}, false
		}
	}
	return Identity{UserID: userID, Email: email, Policy: policy, Anchors: anchors}, true
}

func (h *Handler) snapshot(w http.ResponseWriter, identity Identity) {
	nodes, err := h.visibleNodes(identity)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "snapshot_unavailable")
		return
	}
	writeJSON(w, map[string]any{
		"user": identity, "roots": identity.Anchors, "nodes": data.RedactNodes(nodes),
		"session_timeout_seconds": int(h.sessionTimeout / time.Second),
	})
}

func (h *Handler) visibleNodes(identity Identity) ([]data.NodeEdge, error) {
	allowedTypes := map[string]bool{
		data.NodeTypeDevice: true, data.NodeTypeGroup: true, data.NodeTypeVariable: true,
		data.NodeTypeModbus: true, data.NodeTypeModbusIO: true,
		"decisionTable": true, "deviceTemplate": true,
		// 领域模型三层节点：不在白名单会被剪断整棵子树，前端看不到。
		NodeTypeSite: true, NodeTypePlant: true, NodeTypeEnergyUnit: true, NodeTypeResourceGroup: true,
		NodeTypeOpcua: true, NodeTypeOpcuaPoint: true,
		"mqtt": true, "mqttSub": true,
	}
	nodes := []data.NodeEdge{}
	seen := map[string]bool{}
	for _, root := range identity.Anchors {
		anchorNodes, err := client.GetNodes(h.nc, "all", root, "", false)
		if err == nil {
			for _, node := range anchorNodes {
				if node.ID == root && allowedTypes[node.Type] {
					nodes = append(nodes, node)
				}
			}
		}
		queue := []string{root}
		for len(queue) > 0 {
			parent := queue[0]
			queue = queue[1:]
			if seen[parent] {
				continue
			}
			seen[parent] = true
			children, err := client.GetNodes(h.nc, parent, "all", "", false)
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				if !allowedTypes[child.Type] {
					continue
				}
				nodes = append(nodes, child)
				queue = append(queue, child.ID)
			}
		}
	}
	return nodes, nil
}

func (h *Handler) createNode(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Parent      string   `json:"parent"`
		Type        string   `json:"type"`
		Description string   `json:"description"`
		Value       *float64 `json:"value"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Description == "" || !h.allows(identity, input.Parent) {
		writeError(w, http.StatusBadRequest, "invalid_node")
		return
	}
	if input.Type != data.NodeTypeGroup && input.Type != data.NodeTypeVariable {
		writeError(w, http.StatusBadRequest, "unsupported_node_type")
		return
	}
	if input.Value != nil && !finite(*input.Value) {
		writeError(w, http.StatusBadRequest, "invalid_value")
		return
	}
	node := data.NodeEdge{
		ID: data.NewID(), Type: input.Type, Parent: input.Parent,
		Points: data.Points{data.NewPointString(data.PointTypeDescription, "", input.Description)},
	}
	if input.Type == data.NodeTypeVariable {
		value := 0.0
		if input.Value != nil {
			value = *input.Value
		}
		node.Points = append(node.Points,
			data.NewPointString("businessId", "", data.NewID()),
			data.NewPointFloat(data.PointTypeValue, "", value))
	}
	if err := client.SendNode(h.nc, node, identity.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "create_failed")
		return
	}
	writeJSON(w, map[string]string{"id": node.ID})
}

func (h *Handler) writeValue(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionWriteValue) {
		writeError(w, http.StatusForbidden, "operation_not_allowed")
		return
	}
	var input struct {
		Value     float64 `json:"value"`
		PointType string  `json:"point_type"`
		RequestID string  `json:"request_id"`
	}
	if !decodeJSON(w, r, &input) || !finite(input.Value) {
		writeError(w, http.StatusBadRequest, "invalid_value")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	if input.PointType == "" {
		input.PointType = data.PointTypeValueSet
	}
	if node.Type == data.NodeTypeVariable {
		if input.PointType != data.PointTypeValue && input.PointType != data.PointTypeValueSet {
			writeError(w, http.StatusBadRequest, "invalid_point_type")
			return
		}
	} else if node.Type == data.NodeTypeModbusIO {
		if input.PointType != data.PointTypeValueSet {
			writeError(w, http.StatusBadRequest, "invalid_point_type")
			return
		}
		if readOnly, _ := node.Points.Value(data.PointTypeReadOnly, ""); readOnly != 0 {
			writeError(w, http.StatusForbidden, "read_only")
			return
		}
		ioType, _ := node.Points.Text(data.PointTypeModbusIOType, "")
		if ioType != data.PointValueModbusCoil && ioType != data.PointValueModbusHoldingRegister {
			writeError(w, http.StatusForbidden, "read_only")
			return
		}
		if ioType == data.PointValueModbusCoil && input.Value != 0 && input.Value != 1 {
			writeError(w, http.StatusBadRequest, "coil_value_must_be_binary")
			return
		}
	} else {
		writeError(w, http.StatusForbidden, "operation_not_allowed")
		return
	}
	if disabled, _ := node.Points.Value(data.PointTypeDisabled, ""); disabled != 0 {
		writeError(w, http.StatusConflict, "node_disabled")
		return
	}
	h.commandMu.Lock()
	defer h.commandMu.Unlock()
	if len(input.RequestID) > 128 {
		writeError(w, http.StatusBadRequest, "invalid_request_id")
		return
	}
	if input.RequestID == "" {
		input.RequestID = data.NewID()
	}
	if existing, found, err := h.existingCommand(id, input.RequestID, input.Value); err != nil {
		writeError(w, http.StatusConflict, "request_id_conflict")
		return
	} else if found {
		writeJSON(w, existing)
		return
	}
	result := CommandResult{
		RequestID: input.RequestID, Status: "accepted", Target: input.Value,
		Actor: identity.UserID, CreatedAt: time.Now(),
	}
	point := data.NewPointFloat(input.PointType, "", input.Value)
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		result.Status = "failed"
		_ = h.recordCommand(id, result)
		writeError(w, http.StatusBadGateway, "command_not_accepted")
		return
	}
	if err := h.recordCommand(id, result); err != nil {
		writeError(w, http.StatusInternalServerError, "command_state_unavailable")
		return
	}
	writeJSON(w, h.waitForReadback(id, result))
}

func (h *Handler) setManual(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionWriteValue) {
		writeError(w, http.StatusForbidden, "operation_not_allowed")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	var input struct {
		Manual *bool `json:"manual"`
	}
	if !decodeJSON(w, r, &input) || input.Manual == nil {
		writeError(w, http.StatusBadRequest, "invalid_manual_state")
		return
	}
	value := data.BoolToFloat(*input.Manual)
	target := decision.Target{NodeID: id, PointType: data.PointTypeValueSet}
	if err := decision.ValidateTarget(h.nc, node.Parent, target, true, &value); err != nil {
		writeError(w, http.StatusForbidden, "operation_not_allowed")
		return
	}
	point := data.NewPointFloat("easyemsManual", "", value)
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]any{"manual": *input.Manual})
}

func (h *Handler) createRule(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Parent      string            `json:"parent"`
		Description string            `json:"description"`
		Document    decision.Document `json:"document"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Description == "" || !h.allows(identity, input.Parent) || input.Document.Validate() != nil {
		writeError(w, http.StatusBadRequest, "invalid_rule")
		return
	}
	encoded, _ := json.Marshal(input.Document)
	id := data.NewID()
	err := client.SendNodeType(h.nc, decision.DecisionTable{
		ID: id, Parent: input.Parent, Description: input.Description, Draft: string(encoded),
	}, identity.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create_failed")
		return
	}
	writeJSON(w, map[string]string{"id": id})
}

func (h *Handler) saveDraft(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	h.writeRuleDocument(w, r, identity, id, "draft")
}

func (h *Handler) writeRuleDocument(w http.ResponseWriter, r *http.Request, identity Identity, id, pointType string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	rule, ok := h.rule(w, identity, id)
	if !ok {
		return
	}
	var document decision.Document
	if !decodeJSON(w, r, &document) {
		return
	}
	if err := document.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if pointType == "applied" && !h.documentInScope(rule, document) {
		writeError(w, http.StatusForbidden, "target_out_of_scope")
		return
	}
	encoded, _ := json.Marshal(document)
	point := data.NewPointString(pointType, "", string(encoded))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]any{"saved": true})
}

func (h *Handler) enableRule(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionRulePublish) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	rule, ok := h.rule(w, identity, id)
	if !ok {
		return
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Enabled {
		var document decision.Document
		applied, _ := rule.Points.Text("applied", "")
		if applied == "" || json.Unmarshal([]byte(applied), &document) != nil || document.Validate() != nil {
			writeError(w, http.StatusBadRequest, "publish_required")
			return
		}
		h.ownershipMu.Lock()
		defer h.ownershipMu.Unlock()
		if err := decision.ValidateOutputOwnership(h.nc, id, document); err != nil {
			writeError(w, http.StatusConflict, "output_owned")
			return
		}
	}
	point := data.NewPointFloat("enabled", "", data.BoolToFloat(input.Enabled))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]any{"enabled": input.Enabled})
}

// setTariff stores the station's flat electricity price on the root node.
// It is deliberately one number: enough to estimate today's grid cost from an
// existing kWh point without inventing a billing engine.
func (h *Handler) setTariff(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionSiteSettings) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Root  string  `json:"root"`
		Price float64 `json:"price"`
	}
	if !decodeJSON(w, r, &input) || input.Root == "" || !finite(input.Price) || input.Price < 0 || input.Price > 100 {
		writeError(w, http.StatusBadRequest, "invalid_tariff")
		return
	}
	if !h.allows(identity, input.Root) {
		writeError(w, http.StatusForbidden, "no_access")
		return
	}
	point := data.NewPointFloat("easyemsTariff", "", input.Price)
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, input.Root, point, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]any{"saved": true, "price": input.Price})
}

func (h *Handler) testRule(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if _, ok := h.rule(w, identity, id); !ok {
		return
	}
	var input struct {
		Document   decision.Document  `json:"document"`
		Inputs     map[string]float64 `json:"inputs"`
		TextInputs map[string]string  `json:"text_inputs"`
		At         string             `json:"at"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := input.Document.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, value := range input.Inputs {
		if !finite(value) {
			writeError(w, http.StatusBadRequest, "invalid_value")
			return
		}
	}
	at, err := decision.ParseSimulationTime(input.At, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := input.Document.Simulate(input.Inputs, input.TextInputs, at)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// This endpoint only evaluates the pure function and never sends points.
	writeJSON(w, result)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionAccountManage) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Parent   string   `json:"parent"`
		Email    string   `json:"email"`
		Password string   `json:"password"`
		Role     string   `json:"role"`
		Actions  []string `json:"actions"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !h.allows(identity, input.Parent) || input.Email == "" || len(input.Password) < 12 ||
		(input.Role != "admin" && input.Role != "implementer" && input.Role != "operator" && input.Role != "viewer") ||
		!validActions(input.Actions) {
		writeError(w, http.StatusBadRequest, "invalid_user")
		return
	}
	users, err := client.GetNodesType[client.User](h.nc, "all", "all")
	if err == nil {
		for _, user := range users {
			if user.Email == input.Email {
				writeError(w, http.StatusConflict, "user_exists")
				return
			}
		}
	}
	id := data.NewID()
	err = client.SendNodeType(h.nc, client.User{
		ID: id, Parent: input.Parent, Email: input.Email, Pass: input.Password,
	}, identity.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create_failed")
		return
	}
	policy := AccessPolicy{Admin: input.Role == "admin", Role: input.Role, RootNodeIDs: []string{input.Parent}, Actions: input.Actions}
	encoded, _ := json.Marshal(policy)
	point := data.NewPointString("easyemsAccess", "", string(encoded))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		_ = client.DeleteNode(h.nc, id, input.Parent, identity.UserID)
		writeError(w, http.StatusInternalServerError, "create_failed")
		return
	}
	writeJSON(w, map[string]string{"id": id})
}

func validActions(actions []string) bool {
	known := map[string]bool{}
	for _, action := range allActions {
		known[action] = true
	}
	seen := map[string]bool{}
	for _, action := range actions {
		if !known[action] || seen[action] {
			return false
		}
		seen[action] = true
	}
	return true
}

type userDTO struct {
	ID     string       `json:"id"`
	Email  string       `json:"email"`
	Policy AccessPolicy `json:"policy"`
}

func (h *Handler) listUsers(w http.ResponseWriter, identity Identity) {
	if !canAction(identity, ActionAccountManage) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	users, err := client.GetNodes(h.nc, "all", "all", data.NodeTypeUser, false)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "users_unavailable")
		return
	}
	out := make([]userDTO, 0, len(users))
	for _, node := range users {
		email, _ := node.Points.Text(data.PointTypeEmail, "")
		var policy AccessPolicy
		if point, ok := node.Points.Find("easyemsAccess", ""); ok {
			_ = json.Unmarshal([]byte(point.Txt()), &policy)
		}
		out = append(out, userDTO{ID: node.ID, Email: email, Policy: policy})
	}
	writeJSON(w, map[string]any{"users": out, "actions": allActions})
}

func (h *Handler) updateUserPolicy(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionAccountManage) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	nodes, err := client.GetNodes(h.nc, "all", id, data.NodeTypeUser, false)
	if err != nil || len(nodes) == 0 {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	var policy AccessPolicy
	if point, ok := nodes[0].Points.Find("easyemsAccess", ""); ok {
		_ = json.Unmarshal([]byte(point.Txt()), &policy)
	}
	var input struct {
		Role    string   `json:"role"`
		Actions []string `json:"actions"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if (input.Role != "admin" && input.Role != "implementer" && input.Role != "operator" && input.Role != "viewer") ||
		!validActions(input.Actions) {
		writeError(w, http.StatusBadRequest, "invalid_user")
		return
	}
	if id == identity.UserID && input.Role != "admin" {
		writeError(w, http.StatusConflict, "cannot_demote_self")
		return
	}
	policy.Admin = input.Role == "admin"
	policy.Role = input.Role
	policy.Actions = input.Actions
	encoded, _ := json.Marshal(policy)
	point := data.NewPointString("easyemsAccess", "", string(encoded))
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

// resetUserPassword lets an administrator set a new local password for another
// account. The password is written to the user node only and never enters the
// audit log or any export; existing sessions of the target account are revoked.
func (h *Handler) resetUserPassword(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionAccountManage) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) || len(input.Password) < 12 {
		writeError(w, http.StatusBadRequest, "invalid_password")
		return
	}
	if id == identity.UserID {
		writeError(w, http.StatusBadRequest, "reset_self_not_allowed")
		return
	}
	existing, err := client.GetNodes(h.nc, "all", id, data.NodeTypeUser, false)
	if err != nil || len(existing) == 0 {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	point := data.NewPointString(data.PointTypePass, "", input.Password)
	point.Origin = identity.UserID
	if err := client.SendNodePoint(h.nc, id, point, true); err != nil {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	h.sessionMu.Lock()
	for sessionID, state := range h.sessions {
		if state.userID == id {
			delete(h.sessions, sessionID)
		}
	}
	h.sessionMu.Unlock()
	writeJSON(w, map[string]bool{"reset": true})
}

func (h *Handler) node(w http.ResponseWriter, identity Identity, id string) (data.NodeEdge, bool) {
	if !h.allows(identity, id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return data.NodeEdge{}, false
	}
	nodes, err := client.GetNodes(h.nc, "all", id, "", false)
	if err != nil || len(nodes) == 0 {
		writeError(w, http.StatusNotFound, "not_found")
		return data.NodeEdge{}, false
	}
	return nodes[0], true
}

func (h *Handler) rule(w http.ResponseWriter, identity Identity, id string) (data.NodeEdge, bool) {
	node, ok := h.node(w, identity, id)
	if !ok {
		return node, false
	}
	if node.Type != "decisionTable" {
		writeError(w, http.StatusNotFound, "not_found")
		return data.NodeEdge{}, false
	}
	return node, true
}

func (h *Handler) allows(identity Identity, id string) bool {
	for _, anchor := range identity.Anchors {
		if id == anchor || h.users.IsUnder(id, anchor) {
			return true
		}
	}
	return false
}

func (h *Handler) documentInScope(rule data.NodeEdge, document decision.Document) bool {
	for _, row := range document.Table.Rows {
		for _, condition := range row.Conditions {
			if condition.Kind == "time" {
				continue
			}
			if decision.ValidateTarget(h.nc, rule.Parent, document.Bindings.Points[condition.PointID], false, nil) != nil {
				return false
			}
		}
		for _, action := range row.Actions {
			target := document.Bindings.Actions[action.ActionID]
			if decision.ValidateTarget(h.nc, rule.Parent, target, true, &action.Value) != nil {
				return false
			}
			if _, err := decision.LoadAllocator(h.nc, target.NodeID); err == nil {
				if decision.ValidateAllocationOwnership(h.nc, rule.ID, target.NodeID) != nil {
					return false
				}
			}
		}
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
