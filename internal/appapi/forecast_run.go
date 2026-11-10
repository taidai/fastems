package appapi

import (
	"encoding/json"
	"net/http"
	"time"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// 光伏短期预测：按较慢周期读取已配置点位的历史，用移动平均 + 趋势外推
// 生成 pvForecastKW 供能力规则使用。样本不足或历史不可用时保持最近值，
// 并在说明中标注这是历史估计而非精确预测。

const forecastInputsPoint = "easyemsForecastInputs"
const defaultForecastInterval = 5 * time.Minute

// ForecastInputs 描述预测来源与周期。
type ForecastInputs struct {
	SourcePointID string `json:"source_point_id"`
	IntervalSec   int    `json:"interval_sec"`
	WindowMinutes int    `json:"window_minutes"`
}

func loadForecastInputs(node data.NodeEdge) (ForecastInputs, bool) {
	point, found := node.Points.Find(forecastInputsPoint, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return ForecastInputs{}, false
	}
	var inputs ForecastInputs
	if json.Unmarshal([]byte(point.Txt()), &inputs) != nil {
		return ForecastInputs{}, false
	}
	return inputs, true
}

func (inputs ForecastInputs) interval() time.Duration {
	if inputs.IntervalSec > 0 {
		return time.Duration(inputs.IntervalSec) * time.Second
	}
	return defaultForecastInterval
}

// RunForecastTick 为配置了预测来源的 EnergyUnit 刷新 pvForecastKW。
func (h *Handler) RunForecastTick(now time.Time) {
	reader, ok := h.users.(historyReader)
	if !ok {
		return
	}
	units, err := siotclient.GetNodes(h.nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return
	}
	h.forecastMu.Lock()
	if h.forecastLast == nil {
		h.forecastLast = make(map[string]time.Time)
	}
	h.forecastMu.Unlock()
	for _, unit := range units {
		inputs, ok := loadForecastInputs(unit)
		if !ok || inputs.SourcePointID == "" {
			continue
		}
		h.forecastMu.Lock()
		if last, exists := h.forecastLast[unit.ID]; exists && now.Sub(last) < inputs.interval() {
			h.forecastMu.Unlock()
			continue
		}
		h.forecastLast[unit.ID] = now
		h.forecastMu.Unlock()

		window := inputs.WindowMinutes
		if window <= 0 {
			window = 30
		}
		points, err := reader.PointHistory(inputs.SourcePointID, data.PointTypeValue, "", now.Add(-time.Duration(window)*time.Minute), now, 240)
		if err != nil || len(points) == 0 {
			continue
		}
		samples := make([]decision.ForecastSample, 0, len(points))
		for _, point := range points {
			samples = append(samples, decision.ForecastSample{KW: pointFloat(point)})
		}
		forecast := decision.Forecast(samples, 1)
		if len(forecast) == 0 {
			continue
		}
		value := forecast[0]
		if value < 0 {
			value = 0
		}
		point := data.NewPointFloat(PointTypePVForecastKW, "", value)
		point.Origin = "forecast"
		_ = siotclient.SendNodePoint(h.nc, unit.ID, point, true)
	}
}

// forecastInputs 读写预测来源配置。
func (h *Handler) forecastInputs(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	switch r.Method {
	case http.MethodGet:
		inputs, configured := loadForecastInputs(node)
		writeJSON(w, map[string]any{"inputs": inputs, "configured": configured})
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var inputs ForecastInputs
		if !decodeJSON(w, r, &inputs) {
			return
		}
		if inputs.SourcePointID != "" {
			if _, ok := h.node(w, identity, inputs.SourcePointID); !ok {
				return
			}
		}
		if inputs.IntervalSec < 0 || inputs.WindowMinutes < 0 {
			writeError(w, http.StatusBadRequest, "invalid_inputs")
			return
		}
		encoded, err := json.Marshal(inputs)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_inputs")
			return
		}
		point := data.NewPointString(forecastInputsPoint, "", string(encoded))
		point.Origin = identity.UserID
		if err := siotclient.SendNodePoint(h.nc, energyUnitID, point, true); err != nil {
			writeError(w, http.StatusBadGateway, "save_failed")
			return
		}
		writeJSON(w, map[string]bool{"saved": true})
	}
}
