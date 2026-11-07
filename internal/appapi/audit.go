package appapi

import (
	"encoding/csv"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"easyems/internal/audit"
)

var auditActions = []string{"登录", "端口配置", "点表配置", "设备模型", "命令执行", "控制写入", "决策表", "报警处理", "账号管理", "站点设置", "验收", "其他"}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func shouldAudit(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	path := r.URL.Path
	return !strings.HasSuffix(path, "/health") && !strings.HasSuffix(path, "/login")
}

func auditAction(r *http.Request) string {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	parts := strings.Split(path, "/")
	first := ""
	if len(parts) > 0 {
		first = parts[0]
	}
	switch {
	case first == "modbus" && strings.Contains(path, "/commands"):
		return "命令执行"
	case first == "modbus" && strings.Contains(path, "/units/") && strings.HasSuffix(path, "/model"):
		return "设备模型"
	case first == "modbus" && strings.Contains(path, "/io"):
		return "点表配置"
	case first == "modbus":
		return "端口配置"
	case first == "ems" && len(parts) > 1 && parts[1] == "channels":
		return "端口配置"
	case first == "ems" && len(parts) > 1 && (parts[1] == "rules" || parts[1] == "allocators"):
		return "决策表"
	case first == "ems" && len(parts) > 1 && parts[1] == "alarms":
		return "报警处理"
	case first == "ems" && len(parts) > 1 && parts[1] == "acceptance":
		return "验收"
	case first == "nodes" && len(parts) >= 3 && (parts[2] == "value" || parts[2] == "manual"):
		return "控制写入"
	case first == "rules":
		return "决策表"
	case first == "alarms" || first == "alarm-types":
		return "报警处理"
	case first == "users":
		return "账号管理"
	case first == "tariff" || first == "history-settings":
		return "站点设置"
	case first == "templates":
		return "点表配置"
	default:
		return "其他"
	}
}

func auditObject(r *http.Request) string {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	parts := strings.Split(path, "/")
	switch {
	case len(parts) >= 3 && parts[0] == "nodes" && (parts[2] == "value" || parts[2] == "manual"):
		return "点位 " + parts[1]
	case len(parts) >= 5 && parts[0] == "modbus" && parts[2] == "units" && parts[4] == "commands":
		return "设备 " + parts[1] + "/" + parts[3] + " 命令"
	case len(parts) >= 5 && parts[0] == "modbus" && parts[2] == "units" && parts[4] == "model":
		return "设备 " + parts[1] + "/" + parts[3] + " 模型"
	case len(parts) >= 2 && parts[0] == "modbus" && len(parts) > 2 && parts[2] == "io":
		return "通道 " + parts[1] + " 点表"
	case len(parts) >= 2 && parts[0] == "modbus":
		return "通道 " + parts[1]
	case len(parts) == 2 && parts[0] == "ems" && parts[1] == "channels":
		return "通道配置"
	case len(parts) >= 3 && parts[0] == "ems" && parts[1] == "channels":
		return "通道 " + parts[2]
	case len(parts) >= 3 && parts[0] == "ems" && parts[1] == "allocators":
		return "分配器 " + parts[2]
	case len(parts) == 2 && parts[0] == "rules":
		return "规则配置"
	case len(parts) >= 4 && parts[0] == "ems" && parts[1] == "alarms":
		return "报警 " + parts[2] + " " + map[string]string{"ack": "确认", "close": "关闭", "maintenance": "维护"}[parts[3]]
	case len(parts) >= 4 && parts[0] == "rules":
		action := parts[2]
		if action == "actions" {
			action = parts[3]
		}
		label := map[string]string{"publish": "发布", "enable": "启用"}[action]
		if label != "" {
			return "规则 " + parts[1] + " " + label
		}
		return "规则 " + parts[1]
	case len(parts) >= 2 && parts[0] == "users":
		return "用户 " + parts[1]
	case parts[0] == "tariff":
		return "站点电价"
	case parts[0] == "history-settings":
		return "历史保留"
	case parts[0] == "ems" && len(parts) > 1 && parts[1] == "acceptance":
		return "验收报告"
	default:
		return path
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (h *Handler) recordAudit(actor string, r *http.Request, status int) {
	if h.audit == nil {
		return
	}
	result := "success"
	if status >= 400 {
		result = "failure"
	}
	if actor == "" {
		actor = "anonymous"
	}
	_ = h.audit.Append(audit.Record{
		Time: time.Now(), Actor: actor, Action: auditAction(r),
		Object: auditObject(r), Detail: "HTTP " + strconv.Itoa(status),
		Result: result, IP: clientIP(r),
	})
}

func (h *Handler) auditLogin(actor, result, detail string, r *http.Request) {
	if h.audit == nil {
		return
	}
	if actor == "" {
		actor = "anonymous"
	}
	_ = h.audit.Append(audit.Record{
		Time: time.Now(), Actor: actor, Action: "登录", Object: "本地登录",
		Detail: detail, Result: result, IP: clientIP(r),
	})
}

func (h *Handler) auditFilter(r *http.Request) (audit.Filter, error) {
	query := r.URL.Query()
	from, err := audit.ParseRange(query.Get("from"), false)
	if err != nil {
		return audit.Filter{}, err
	}
	to, err := audit.ParseRange(query.Get("to"), true)
	if err != nil {
		return audit.Filter{}, err
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	return audit.Filter{
		Actor: query.Get("actor"), Action: query.Get("action"),
		Result: query.Get("result"), From: from, To: to, Limit: limit,
	}, nil
}

func (h *Handler) auditList(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !identity.Policy.Admin {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	filter, err := h.auditFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_time")
		return
	}
	records, err := h.audit.Query(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "audit_unavailable")
		return
	}
	writeJSON(w, map[string]any{"records": records, "actions": auditActions})
}

func (h *Handler) auditExport(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !identity.Policy.Admin {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	filter, err := h.auditFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_time")
		return
	}
	records, err := h.audit.Query(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "audit_unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="easyems-audit.csv"`)
	writer := csv.NewWriter(w)
	_ = h.audit.WriteCSV(writer, records)
}
