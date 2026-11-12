package appapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fastiot/internal/ems/runtime"
)

const historyLimitFile = "history-limit"

type historySetting struct {
	Limit int `json:"limit"`
}

// ReadHistoryLimit reads the persisted per-point history cap.
func ReadHistoryLimit(dataDir string) int {
	limit := 20000
	raw, err := os.ReadFile(filepath.Join(dataDir, historyLimitFile))
	if err != nil {
		return limit
	}
	if value, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && value >= 1000 && value <= 200000 {
		return value
	}
	return limit
}

// ApplyRuntimeSettings loads persisted runtime settings at startup.
func ApplyRuntimeSettings(dataDir string, service *runtime.Service) {
	if service == nil {
		return
	}
	service.SetHistoryLimit(ReadHistoryLimit(dataDir))
}

func (h *Handler) historySettings(w http.ResponseWriter, r *http.Request, identity Identity) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, historySetting{Limit: ReadHistoryLimit(h.dataDir)})
	case http.MethodPut:
		if !canAction(identity, ActionSiteSettings) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input historySetting
		if !decodeJSON(w, r, &input) {
			return
		}
		if input.Limit < 1000 || input.Limit > 200000 {
			writeError(w, http.StatusBadRequest, "invalid_history_limit")
			return
		}
		if h.dataDir == "" {
			writeError(w, http.StatusInternalServerError, "data_dir_unavailable")
			return
		}
		if err := os.MkdirAll(h.dataDir, 0700); err != nil {
			writeError(w, http.StatusInternalServerError, "save_failed")
			return
		}
		if err := os.WriteFile(filepath.Join(h.dataDir, historyLimitFile), []byte(strconv.Itoa(input.Limit)+"\n"), 0600); err != nil {
			writeError(w, http.StatusInternalServerError, "save_failed")
			return
		}
		if h.ems != nil && h.ems.Service != nil {
			h.ems.Service.SetHistoryLimit(input.Limit)
		}
		writeJSON(w, map[string]any{"limit": input.Limit, "restart_required": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

// 审计保留设置：轮换大小、备份个数与按天保留。写入后立即生效，并在
// 轮换时清理过期记录；查询与导出包含仍保留的轮换文件。
const auditSettingsFile = "audit-settings.json"

// AuditSetting 是审计存储配置。
type AuditSetting struct {
	MaxMB         int `json:"max_mb"`
	Backups       int `json:"backups"`
	RetentionDays int `json:"retention_days"`
}

// ReadAuditSetting 读取持久化的审计存储配置。
func ReadAuditSetting(dataDir string) AuditSetting {
	setting := AuditSetting{MaxMB: 5, Backups: 1, RetentionDays: 0}
	raw, err := os.ReadFile(filepath.Join(dataDir, auditSettingsFile))
	if err != nil {
		return setting
	}
	if json.Unmarshal(raw, &setting) != nil {
		return AuditSetting{MaxMB: 5, Backups: 1, RetentionDays: 0}
	}
	if setting.MaxMB < 1 || setting.MaxMB > 200 {
		setting.MaxMB = 5
	}
	if setting.Backups < 0 || setting.Backups > 10 {
		setting.Backups = 1
	}
	if setting.RetentionDays < 0 || setting.RetentionDays > 3650 {
		setting.RetentionDays = 0
	}
	return setting
}

func (h *Handler) applyAuditSettings() {
	if h.audit == nil || h.dataDir == "" {
		return
	}
	setting := ReadAuditSetting(h.dataDir)
	h.audit.Configure(int64(setting.MaxMB)<<20, setting.Backups, setting.RetentionDays)
}

func (h *Handler) auditSettings(w http.ResponseWriter, r *http.Request, identity Identity) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, ReadAuditSetting(h.dataDir))
	case http.MethodPut:
		if !canAction(identity, ActionSiteSettings) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input AuditSetting
		if !decodeJSON(w, r, &input) {
			return
		}
		if input.MaxMB < 1 || input.MaxMB > 200 || input.Backups < 0 || input.Backups > 10 || input.RetentionDays < 0 || input.RetentionDays > 3650 {
			writeError(w, http.StatusBadRequest, "invalid_audit_settings")
			return
		}
		if h.dataDir == "" {
			writeError(w, http.StatusInternalServerError, "data_dir_unavailable")
			return
		}
		if err := os.MkdirAll(h.dataDir, 0700); err != nil {
			writeError(w, http.StatusInternalServerError, "save_failed")
			return
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_audit_settings")
			return
		}
		if err := os.WriteFile(filepath.Join(h.dataDir, auditSettingsFile), encoded, 0600); err != nil {
			writeError(w, http.StatusInternalServerError, "save_failed")
			return
		}
		h.applyAuditSettings()
		writeJSON(w, input)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}
