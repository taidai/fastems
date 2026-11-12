package appapi

import (
	"encoding/json"
	"os"
	"path/filepath"

	"fastiot/internal/ems/alarm"
)

const alarmMaintenanceFile = "alarm-maintenance.json"

// LoadAlarmMaintenance restores maintenance windows before the watcher starts.
func LoadAlarmMaintenance(dataDir string, engine *alarm.Engine) {
	if engine == nil || dataDir == "" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, alarmMaintenanceFile))
	if err != nil {
		return
	}
	var windows map[string]alarm.Maintenance
	if json.Unmarshal(raw, &windows) == nil {
		engine.ImportMaintenance(windows)
	}
}

func (h *Handler) persistAlarmMaintenance() {
	if h.dataDir == "" || h.ems == nil || h.ems.Service == nil || h.ems.Service.Alarms == nil {
		return
	}
	encoded, err := json.Marshal(h.ems.Service.Alarms.ExportMaintenance())
	if err != nil {
		return
	}
	_ = os.MkdirAll(h.dataDir, 0700)
	_ = os.WriteFile(filepath.Join(h.dataDir, alarmMaintenanceFile), encoded, 0600)
}

// PersistAlarmMaintenance is the callback used by the EMS API.
func (h *Handler) PersistAlarmMaintenance() { h.persistAlarmMaintenance() }
