package appapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"easyems/internal/ems/runtime"
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
		if !identity.Policy.Admin {
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
