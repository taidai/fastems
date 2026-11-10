package appapi

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// 动态能力刷新（CAP-03/CAP-06）：从 EnergyUnit 配置的 BMS/SOC/温度点位读取
// 带时效的实测，分充/放方向合成能力。过期或无效输入必须关闭相应方向，
// 不能继续沿用上一秒仍可用的旧能力。

const capabilityInputsPoint = "easyemsCapabilityInputs"
const defaultCapabilityMaxAge = 5 * time.Second

// CapabilityInputs 描述动态能力的来源、边界与执行参数。
type CapabilityInputs struct {
	SOCPointID            string  `json:"soc_point_id"`
	TempPointID           string  `json:"temp_point_id"`
	ChargeLimitPointID    string  `json:"charge_limit_point_id"`
	DischargeLimitPointID string  `json:"discharge_limit_point_id"`
	SOCMin                float64 `json:"soc_min"`
	SOCMax                float64 `json:"soc_max"`
	TempDerate            float64 `json:"temp_derate"`
	TempLimit             float64 `json:"temp_limit"`
	MaxAgeMS              int64   `json:"max_age_ms"`
	RampRateKW            float64 `json:"ramp_rate_kw"`
	DeadbandKW            float64 `json:"deadband_kw"`
}

func loadCapabilityInputs(node data.NodeEdge) (CapabilityInputs, bool) {
	point, found := node.Points.Find(capabilityInputsPoint, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return CapabilityInputs{}, false
	}
	var inputs CapabilityInputs
	if json.Unmarshal([]byte(point.Txt()), &inputs) != nil {
		return CapabilityInputs{}, false
	}
	return inputs, true
}

func (inputs CapabilityInputs) maxAge() time.Duration {
	if inputs.MaxAgeMS <= 0 {
		return defaultCapabilityMaxAge
	}
	return time.Duration(inputs.MaxAgeMS) * time.Millisecond
}

func (inputs CapabilityInputs) rampParams() decision.RampParams {
	params := decision.DefaultRampParams()
	if inputs.RampRateKW > 0 {
		params.RampRateKW = inputs.RampRateKW
	}
	if inputs.DeadbandKW > 0 {
		params.DeadbandKW = inputs.DeadbandKW
	}
	return params
}

// readPointSample 读取一个带时效的数值样本。空时间、未来异常时间、停用、
// 非有限数和过期值都视为无效，原因仅供业务展示，不包含连接信息。
func readPointSample(nc *nats.Conn, nodeID string, now time.Time, maxAge time.Duration) (float64, bool, string) {
	if nodeID == "" {
		return 0, false, "未配置点位"
	}
	nodes, err := siotclient.GetNodes(nc, "all", nodeID, "", false)
	if err != nil || len(nodes) == 0 {
		return 0, false, "点位不存在"
	}
	point, found := nodes[0].Points.Find(data.PointTypeValue, "")
	if !found || point.Tombstone%2 != 0 {
		return 0, false, "点位已停用或无数值"
	}
	value := 0.0
	if point.Numeric() {
		value, err = point.ValueFloat()
	} else {
		value, err = strconv.ParseFloat(point.Txt(), 64)
	}
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, "点位不是有效数值"
	}
	if point.Time.IsZero() {
		return 0, false, "点位缺少采样时间"
	}
	if point.Time.After(now.Add(5 * time.Second)) {
		return 0, false, "点位采样时间异常"
	}
	if now.Sub(point.Time) > maxAge {
		return 0, false, "点位数据过期"
	}
	return value, true, ""
}

// RefreshCapabilities 扫描所有配置了动态能力来源的 EnergyUnit 并刷新能力。
func RefreshCapabilities(nc *nats.Conn) {
	refreshCapabilitiesAt(nc, time.Now())
}

func refreshCapabilitiesAt(nc *nats.Conn, now time.Time) {
	units, err := siotclient.GetNodes(nc, "all", "all", NodeTypeEnergyUnit, false)
	if err != nil {
		return
	}
	for _, unit := range units {
		inputs, ok := loadCapabilityInputs(unit)
		if !ok || inputs.SOCPointID == "" {
			continue
		}
		rated := 0.0
		if point, found := unit.Points.Find(PointTypeRatedPowerKW, ""); found {
			rated = pointFloat(point)
		}

		reasons := []string{}
		soc, socOK, reason := readPointSample(nc, inputs.SOCPointID, now, inputs.maxAge())
		if !socOK {
			reasons = append(reasons, "SOC "+reason)
		}
		temp, tempOK := 0.0, true
		if inputs.TempPointID != "" {
			temp, tempOK, reason = readPointSample(nc, inputs.TempPointID, now, inputs.maxAge())
			if !tempOK {
				reasons = append(reasons, "温度 "+reason)
			}
		}

		chargeAvailable, dischargeAvailable := rated, rated
		chargeSourceOK, dischargeSourceOK := true, true
		if inputs.ChargeLimitPointID != "" {
			chargeAvailable, chargeSourceOK, reason = readPointSample(nc, inputs.ChargeLimitPointID, now, inputs.maxAge())
			if !chargeSourceOK {
				reasons = append(reasons, "BMS 允许充电 "+reason)
			}
		}
		if inputs.DischargeLimitPointID != "" {
			dischargeAvailable, dischargeSourceOK, reason = readPointSample(nc, inputs.DischargeLimitPointID, now, inputs.maxAge())
			if !dischargeSourceOK {
				reasons = append(reasons, "BMS 允许放电 "+reason)
			}
		}

		charge, discharge := 0.0, 0.0
		chargeOK, dischargeOK := false, false
		if socOK && tempOK {
			base := decision.CapabilityLimit{
				Rated: rated, SOC: soc, SOCMin: inputs.SOCMin, SOCMax: inputs.SOCMax,
				Temp: temp, TempDerate: inputs.TempDerate, TempLimit: inputs.TempLimit,
			}
			if chargeSourceOK {
				limit := base
				limit.Available = chargeAvailable
				charge, chargeOK = decision.DynamicCharge(limit)
			}
			if dischargeSourceOK {
				limit := base
				limit.Available = dischargeAvailable
				discharge, dischargeOK = decision.DynamicDischarge(limit)
			}
		}

		capability := Capability{
			Kind: "discharge", Direction: "none", Allowed: chargeOK || dischargeOK,
			ChargeMaxKW: charge, DischargeMaxKW: discharge, RampKWs: inputs.rampParams().RampRateKW,
		}
		switch {
		case chargeOK && dischargeOK:
			capability.Direction = "bidir"
		case chargeOK:
			capability.Direction = "charge"
		case dischargeOK:
			capability.Direction = "discharge"
		}
		capability.PowerMaxKW = math.Max(charge, discharge)
		if len(reasons) > 0 {
			capability.LimitReason = strings.Join(reasons, "；")
		} else if !chargeOK || !dischargeOK {
			capability.LimitReason = "SOC 越界、温度约束或 BMS 允许功率为 0"
		}
		text, err := EncodeCapability(capability)
		if err != nil {
			continue
		}
		point := data.NewPointString(PointTypeCapability, "", text)
		point.Origin = "capability-refresh"
		_ = siotclient.SendNodePoint(nc, unit.ID, point, true)
	}
}

// capabilityInputs 读写动态能力来源配置。
func (h *Handler) capabilityInputs(w http.ResponseWriter, r *http.Request, identity Identity, energyUnitID string) {
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
		inputs, ok := loadCapabilityInputs(node)
		writeJSON(w, map[string]any{"inputs": inputs, "configured": ok})
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var inputs CapabilityInputs
		if !decodeJSON(w, r, &inputs) {
			return
		}
		if inputs.SOCPointID == "" {
			writeError(w, http.StatusBadRequest, "soc_point_required")
			return
		}
		if inputs.MaxAgeMS < 0 || inputs.RampRateKW < 0 || inputs.DeadbandKW < 0 || inputs.SOCMin < 0 || inputs.SOCMax > 100 || inputs.SOCMin >= inputs.SOCMax {
			writeError(w, http.StatusBadRequest, "invalid_inputs")
			return
		}
		for _, pointID := range []string{inputs.SOCPointID, inputs.TempPointID, inputs.ChargeLimitPointID, inputs.DischargeLimitPointID} {
			if pointID == "" {
				continue
			}
			if _, ok := h.node(w, identity, pointID); !ok {
				return
			}
		}
		encoded, err := json.Marshal(inputs)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_inputs")
			return
		}
		point := data.NewPointString(capabilityInputsPoint, "", string(encoded))
		point.Origin = identity.UserID
		if err := siotclient.SendNodePoint(h.nc, energyUnitID, point, true); err != nil {
			writeError(w, http.StatusBadGateway, "save_failed")
			return
		}
		writeJSON(w, map[string]bool{"saved": true})
	}
}
