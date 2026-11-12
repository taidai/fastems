package appapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simpleiot/simpleiot/data"

	"fastems/internal/audit"
	"fastems/internal/decision"
	"fastems/internal/ems/channel"
	"fastems/internal/ems/signal"
)

// 交付工作台的投运门槛：把"能不能投运"拆成可判定的几道门。
// 门槛只读、可轮询；预检是同一批门槛的一次快照，写入 preflight.jsonl 与审计。
// 这里不改变任何发布行为——发布动作要等项目包（A17/A19）。

const preflightFile = "preflight.jsonl"
const preflightAction = "交付预检"

type preflightGate struct {
	Key           string `json:"key"`
	Title         string `json:"title"`
	Status        string `json:"status"` // pass | warn | fail
	Detail        string `json:"detail"`
	Object        string `json:"object,omitempty"`
	NextStep      string `json:"next_step,omitempty"`
	Informational bool   `json:"informational,omitempty"` // 只展示，不参与总状态
}

type preflightReport struct {
	ID          string          `json:"id"`
	Site        string          `json:"site,omitempty"`
	GeneratedAt time.Time       `json:"generated_at"`
	Operator    string          `json:"operator,omitempty"`
	Status      string          `json:"status"`
	Summary     map[string]int  `json:"summary"`
	Gates       []preflightGate `json:"gates"`
}

// deliveryGates 只读门槛：供页面轮询，不写存储。
func (h *Handler) deliveryGates(w http.ResponseWriter, identity Identity, siteID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if !h.checkSite(w, identity, siteID) {
		return
	}
	report, err := h.buildPreflight(identity, siteID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "preflight_unavailable")
		return
	}
	writeJSON(w, report)
}

// runPreflight 生成快照：写 preflight.jsonl 与审计，返回同一次计算的结果。
func (h *Handler) runPreflight(w http.ResponseWriter, identity Identity, siteID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if !h.checkSite(w, identity, siteID) {
		return
	}
	report, err := h.buildPreflight(identity, siteID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "preflight_unavailable")
		return
	}
	now := time.Now()
	report.ID = "pf-" + now.Format("20060102-150405")
	report.Site = siteID
	report.GeneratedAt = now
	report.Operator = identity.Email
	if report.Operator == "" {
		report.Operator = identity.UserID
	}
	if err := h.appendPreflight(report); err != nil {
		writeError(w, http.StatusInternalServerError, "preflight_save_failed")
		return
	}
	if h.audit != nil {
		_ = h.audit.Append(audit.Record{
			Time: now, Actor: report.Operator, Action: preflightAction, Object: report.ID,
			Detail: fmt.Sprintf("状态 %s：通过 %d / 注意 %d / 阻断 %d", report.Status, report.Summary["pass"], report.Summary["warn"], report.Summary["fail"]),
			Result: preflightResult(report.Status),
		})
	}
	writeJSON(w, report)
}

// latestPreflight 返回最近一次快照；没有历史时 404，不伪造空报告。
func (h *Handler) latestPreflight(w http.ResponseWriter, identity Identity, siteID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if !h.checkSite(w, identity, siteID) {
		return
	}
	report, err := h.readLatestPreflight(siteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "preflight_read_failed")
		return
	}
	// 没有历史时返回 200 与 null，避免浏览器控制台把"还没有预检"记成错误。
	writeJSON(w, map[string]any{"latest": report})
}

func preflightResult(status string) string {
	if status == "fail" {
		return "failure"
	}
	return "success"
}

func (h *Handler) buildPreflight(identity Identity, siteID string) (preflightReport, error) {
	if h.ems == nil || h.ems.Service == nil {
		return preflightReport{}, fmt.Errorf("ems unavailable")
	}
	nodes, err := h.visibleNodes(identity)
	if err != nil {
		return preflightReport{}, err
	}
	nodes = scopeNodesToSite(nodes, siteID)
	now := time.Now()
	gates := make([]preflightGate, 0, 10)

	gates = append(gates, h.channelGate(nodes, now))
	gates = append(gates, h.pointGate(nodes, now))
	gates = append(gates, h.ruleGate(nodes))
	gates = append(gates, h.commandGate(nodes))
	gates = append(gates, h.manualGate(nodes))
	gates = append(gates, h.disabledGate(nodes))
	gates = append(gates, h.alarmGate())
	gates = append(gates, h.historyGate())
	gates = append(gates, preflightGate{
		Key: "backup", Title: "备份", Status: "warn", Informational: true,
		Detail: "尚未验证离线完整备份", NextStep: "停机复制数据目录并做一次恢复演练",
	})
	gates = append(gates, preflightGate{
		Key: "permissions", Title: "权限边界", Status: "pass",
		Detail: "服务端逐请求校验账号、角色与业务树范围",
	})

	summary := map[string]int{"pass": 0, "warn": 0, "fail": 0}
	status := "pass"
	for _, gate := range gates {
		if gate.Informational {
			continue
		}
		summary[gate.Status]++
		if gate.Status == "fail" {
			status = "fail"
		} else if gate.Status == "warn" && status == "pass" {
			status = "warn"
		}
	}
	return preflightReport{Site: siteID, GeneratedAt: now, Status: status, Summary: summary, Gates: gates}, nil
}

func (h *Handler) channelGate(nodes []data.NodeEdge, now time.Time) preflightGate {
	running := make(map[string]*channel.Channel)
	if h.ems.Service != nil {
		for _, ch := range h.ems.Service.Channels() {
			running[ch.ID] = ch
		}
	}
	var total, devices, offline, degraded, unknown, idle int
	var object string
	name := func(node data.NodeEdge) string {
		if name := node.Desc(); name != "" {
			return name
		}
		return node.ID
	}
	for _, node := range nodes {
		if node.Type != data.NodeTypeModbus {
			continue
		}
		total++
		ch, ok := running[node.ID]
		if !ok {
			// 通道节点已配置但运行时没有实例（地址/驱动未就绪），按离线处理，
			// 不能因为运行时列表为空就报“没有任何通道”。
			idle++
			if object == "" {
				object = name(node)
			}
			continue
		}
		for _, device := range ch.Devices {
			devices++
			switch ch.Health(device.ID, now, h.ems.MaxAge) {
			case channel.HealthOffline:
				offline++
				if object == "" {
					object = name(node)
				}
			case channel.HealthDegraded:
				degraded++
				if object == "" {
					object = name(node)
				}
			case channel.HealthUnknown:
				unknown++
				if object == "" {
					object = name(node)
				}
			}
		}
	}
	gate := preflightGate{Key: "channels", Title: "通道"}
	switch {
	case total == 0:
		gate.Status, gate.Detail, gate.NextStep = "fail", "还没有配置任何通道", "在接入建模里添加通道与设备"
	case idle == total:
		gate.Status, gate.Object = "fail", object
		gate.Detail = fmt.Sprintf("%d 条通道未启动", total)
		gate.NextStep = "检查通道地址与协议配置"
	case offline > 0 || idle > 0:
		gate.Status, gate.Object = "fail", object
		gate.Detail = fmt.Sprintf("%d 条通道：离线设备 %d、未启动 %d", total, offline, idle)
		gate.NextStep = "检查地址、串口与接线"
	case devices == 0:
		gate.Status = "warn"
		gate.Detail = fmt.Sprintf("%d 条通道尚未配置设备", total)
		gate.NextStep = "在通道下添加站号与点位"
	case degraded > 0 || unknown > 0:
		gate.Status, gate.Object = "warn", object
		gate.Detail = fmt.Sprintf("%d 条通道：不稳定 %d、状态未知 %d", total, degraded, unknown)
		gate.NextStep = "确认设备已至少成功轮询一次"
	default:
		gate.Status, gate.Detail = "pass", fmt.Sprintf("%d 条通道、%d 台设备全部正常", total, devices)
	}
	return gate
}

type preflightSample struct {
	quality signal.Quality
	at      time.Time
}

func (h *Handler) pointGate(nodes []data.NodeEdge, now time.Time) preflightGate {
	samples := make(map[string]preflightSample)
	for _, value := range h.ems.Service.Snapshot() {
		at := value.SourceTime
		if at.IsZero() {
			at = value.ReceivedTime
		}
		samples[value.PointID] = preflightSample{quality: value.Quality, at: at}
	}
	critical, criticalAge := criticalPointSet(nodes)
	var total, criticalBad, criticalStale, otherStale int
	var object string
	for _, node := range nodes {
		if node.Type != data.NodeTypeVariable && node.Type != data.NodeTypeModbusIO {
			continue
		}
		total++
		sample, ok := samples[node.ID]
		maxAge := h.ems.MaxAge
		if age, found := criticalAge[node.ID]; found && age > 0 {
			maxAge = age
		}
		bad := !ok || sample.quality != signal.Good
		stale := ok && !sample.at.IsZero() && now.Sub(sample.at) > maxAge
		if critical[node.ID] {
			if bad {
				criticalBad++
				if object == "" {
					object = node.Desc()
				}
			} else if stale {
				criticalStale++
				if object == "" {
					object = node.Desc()
				}
			}
			continue
		}
		if bad || stale {
			otherStale++
		}
	}
	gate := preflightGate{Key: "points", Title: "关键点位"}
	switch {
	case total == 0:
		gate.Status, gate.Detail, gate.NextStep = "fail", "还没有采集到任何点位", "先完成通道与点位映射"
	case criticalBad > 0 || criticalStale > 0:
		gate.Status, gate.Object = "fail", object
		gate.Detail = fmt.Sprintf("关键点位坏值 %d、过期 %d；其他点位异常 %d", criticalBad, criticalStale, otherStale)
		gate.NextStep = "检查关键点位的数据来源与时效"
	case otherStale > 0:
		gate.Status = "warn"
		gate.Detail = fmt.Sprintf("关键点位全部正常；其他点位异常 %d", otherStale)
		gate.NextStep = "确认非关键点位是否需要参与策略"
	default:
		gate.Status, gate.Detail = "pass", fmt.Sprintf("%d 个点位全部新鲜有效", total)
	}
	return gate
}

func (h *Handler) ruleGate(nodes []data.NodeEdge) preflightGate {
	total, enabled, unpublished := 0, 0, 0
	var object string
	for _, node := range nodes {
		if node.Type != "decisionTable" {
			continue
		}
		total++
		value, _ := node.Points.Value("enabled", "")
		if value == 0 {
			continue
		}
		enabled++
		applied, _ := node.Points.Text("applied", "")
		if applied == "" {
			unpublished++
			if object == "" {
				object = node.Desc()
			}
		}
	}
	gate := preflightGate{Key: "rules", Title: "规则发布"}
	switch {
	case total == 0:
		gate.Status, gate.Detail, gate.NextStep = "warn", "还没有决策表", "按需要在 EMS 业务里创建并发布"
	case unpublished > 0:
		gate.Status, gate.Object = "fail", object
		gate.Detail = fmt.Sprintf("已启用但未发布 %d 条（共 %d 条）", unpublished, total)
		gate.NextStep = "发布规则版本或停用草稿"
	default:
		gate.Status, gate.Detail = "pass", fmt.Sprintf("决策表 %d 条，已启用 %d 条", total, enabled)
	}
	return gate
}

func (h *Handler) commandGate(nodes []data.NodeEdge) preflightGate {
	var failed int
	var object string
	for _, node := range nodes {
		if node.Type != data.NodeTypeVariable && node.Type != data.NodeTypeModbusIO {
			continue
		}
		text, _ := node.Points.Text("lastCommand", "")
		if text == "" {
			continue
		}
		var command CommandResult
		if json.Unmarshal([]byte(text), &command) != nil {
			continue
		}
		if command.Status == "unknown" || command.Status == "failed" {
			failed++
			if object == "" {
				object = node.Desc()
			}
		}
	}
	gate := preflightGate{Key: "commands", Title: "命令结果"}
	if failed > 0 {
		gate.Status, gate.Object = "fail", object
		gate.Detail = fmt.Sprintf("存在 %d 条结果未知或失败的命令", failed)
		gate.NextStep = "在调度追溯里核对命令并重新下发"
		return gate
	}
	gate.Status, gate.Detail = "pass", "没有失败或结果未知的命令"
	return gate
}

func (h *Handler) manualGate(nodes []data.NodeEdge) preflightGate {
	var manual int
	for _, node := range nodes {
		if node.Type != data.NodeTypeVariable && node.Type != data.NodeTypeModbusIO {
			continue
		}
		value, _ := node.Points.Value("easyemsManual", "")
		if value != 0 {
			manual++
		}
	}
	gate := preflightGate{Key: "manual", Title: "人工接管"}
	if manual > 0 {
		gate.Status = "warn"
		gate.Detail = fmt.Sprintf("%d 个点位处于人工接管，自动调度不会覆盖", manual)
		gate.NextStep = "确认后恢复自动或保持人工运行"
		return gate
	}
	gate.Status, gate.Detail = "pass", "当前没有点位处于人工接管"
	return gate
}

func (h *Handler) disabledGate(nodes []data.NodeEdge) preflightGate {
	var disabled int
	for _, node := range nodes {
		value, _ := node.Points.Value(data.PointTypeDisabled, "")
		if value != 0 {
			disabled++
		}
	}
	gate := preflightGate{Key: "disabled", Title: "停用对象"}
	if disabled > 0 {
		gate.Status = "warn"
		gate.Detail = fmt.Sprintf("%d 个对象被停用", disabled)
		gate.NextStep = "确认停用是否属于本次交付范围"
		return gate
	}
	gate.Status, gate.Detail = "pass", "当前没有对象被停用"
	return gate
}

func (h *Handler) alarmGate() preflightGate {
	active := h.ems.Service.Alarms.Active()
	gate := preflightGate{Key: "alarms", Title: "活动报警"}
	if active > 0 {
		gate.Status = "warn"
		gate.Detail = fmt.Sprintf("活动报警 %d 项", active)
		gate.NextStep = "逐条确认并记录处理结论"
		return gate
	}
	gate.Status, gate.Detail = "pass", "没有活动报警"
	return gate
}

func (h *Handler) historyGate() preflightGate {
	points := len(h.ems.Service.History.Points())
	gate := preflightGate{Key: "history", Title: "历史记录"}
	if points == 0 {
		gate.Status, gate.Detail, gate.NextStep = "warn", "还没有历史点位", "确认点位已采集并写入历史"
		return gate
	}
	gate.Status, gate.Detail = "pass", fmt.Sprintf("已记录 %d 个点位的历史", points)
	return gate
}

// criticalPointSet 收集"关键点位"：启用的决策表引用、PCC 反馈、EnergyUnit 能力输入。
// 这些点位坏值或过期，控制就不再可信，必须阻断投运。
func criticalPointSet(nodes []data.NodeEdge) (map[string]bool, map[string]time.Duration) {
	points := make(map[string]bool)
	ages := make(map[string]time.Duration)
	add := func(id string) {
		if id != "" {
			points[id] = true
		}
	}
	for _, node := range nodes {
		switch node.Type {
		case "decisionTable":
			enabled, _ := node.Points.Value("enabled", "")
			if enabled == 0 {
				continue
			}
			applied, _ := node.Points.Text("applied", "")
			if applied == "" {
				continue
			}
			var document decision.Document
			if json.Unmarshal([]byte(applied), &document) != nil {
				continue
			}
			for _, target := range document.Bindings.Points {
				add(target.NodeID)
			}
			for _, target := range document.Bindings.Actions {
				add(target.NodeID)
			}
		case NodeTypePlant:
			if binding, ok := loadPCCFeedback(node); ok {
				add(binding.MeterPointID)
				for _, id := range binding.PhasePointIDs {
					add(id)
				}
			}
		case NodeTypeEnergyUnit:
			inputs, ok := loadCapabilityInputs(node)
			if !ok {
				continue
			}
			ids := []string{inputs.SOCPointID, inputs.TempPointID, inputs.ChargeLimitPointID, inputs.DischargeLimitPointID}
			for _, id := range ids {
				add(id)
				if id != "" {
					ages[id] = inputs.maxAge()
				}
			}
		}
	}
	return points, ages
}

func (h *Handler) appendPreflight(report preflightReport) error {
	if h.dataDir == "" {
		return nil
	}
	file, err := os.OpenFile(filepath.Join(h.dataDir, preflightFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	line, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = file.Write(append(line, '\n'))
	return err
}

func (h *Handler) readLatestPreflight(siteID string) (*preflightReport, error) {
	if h.dataDir == "" {
		return nil, nil
	}
	file, err := os.Open(filepath.Join(h.dataDir, preflightFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	var last string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var candidate preflightReport
		if json.Unmarshal([]byte(line), &candidate) != nil {
			continue
		}
		if siteID == "" || candidate.Site == siteID {
			last = line
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if last == "" {
		return nil, nil
	}
	var report preflightReport
	if err := json.Unmarshal([]byte(last), &report); err != nil {
		return nil, err
	}
	return &report, nil
}
