package appapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
	"easyems/internal/ems/signal"
)

// 15 分钟滑动需量：用 PCC 主表的历史样本计算窗口需量，缺 bin 时明确不可用。
// 瞬时功率仍用于防逆流与独立限值；预测性削峰只使用窗口需量。

type plantDemand struct {
	PlantID      string                `json:"plant_id"`
	PlantName    string                `json:"plant_name"`
	MeterPointID string                `json:"meter_point_id,omitempty"`
	UpperLimitKW float64               `json:"upper_limit_kw,omitempty"`
	Window       decision.DemandWindow `json:"window"`
	Reason       string                `json:"reason,omitempty"`
}

func (h *Handler) plantDemandWindow(binding decision.PCCFeedback, now time.Time) (decision.DemandWindow, error) {
	if h.ems == nil || h.ems.Service == nil {
		return decision.DemandWindow{}, fmt.Errorf("ems_unavailable")
	}
	if binding.MeterPointID == "" {
		return decision.DemandWindow{}, fmt.Errorf("meter_required")
	}
	samples := h.ems.Service.History.QueryByPoint(binding.MeterPointID, now.Add(-16*time.Minute), now, 5000)
	points := make([]decision.DemandSample, 0, len(samples))
	sign := binding.Sign
	if sign == 0 {
		sign = 1
	}
	for _, sample := range samples {
		if sample.Quality != signal.Good || sample.Value.Kind != signal.Number {
			continue
		}
		at := sample.SourceTime
		if at.IsZero() {
			at = sample.ReceivedTime
		}
		points = append(points, decision.DemandSample{Time: at, KW: sample.Value.Num * sign})
	}
	return decision.SlidingDemand(points, now), nil
}

func (h *Handler) demandHandler(w http.ResponseWriter, identity Identity, siteID string) {
	if siteID == "" {
		writeError(w, http.StatusBadRequest, "site_required")
		return
	}
	if !h.allows(identity, siteID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	plants, err := client.GetNodes(h.nc, siteID, "all", NodeTypePlant, false)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "plants_unavailable")
		return
	}
	now := time.Now()
	results := make([]plantDemand, 0, len(plants))
	for _, plant := range plants {
		if !h.allows(identity, plant.ID) {
			continue
		}
		item := plantDemand{PlantID: plant.ID, PlantName: nodeName(plant)}
		binding, _, ok := ReadPlantPCC(h.nc, plant.ID)
		if !ok || binding.MeterPointID == "" {
			item.Reason = "未绑定 PCC 反馈电表"
			results = append(results, item)
			continue
		}
		item.MeterPointID = binding.MeterPointID
		item.UpperLimitKW = binding.UpperLimitKW
		window, err := h.plantDemandWindow(binding, now)
		if err != nil {
			item.Reason = err.Error()
			results = append(results, item)
			continue
		}
		item.Window = window
		if !window.Available {
			item.Reason = fmt.Sprintf("滑动窗口缺 %d 个分钟 bin，不做预测性削峰", len(window.MissingBins))
		}
		results = append(results, item)
	}
	writeJSON(w, map[string]any{"site": siteID, "plants": results, "generated_at": now})
}

// 96 点计划：15 分钟一个时段，null 表示该时段没有计划目标（不是 0）。

const plan96Point = "easyemsPlan96"

type plan96 struct {
	Version int        `json:"version"`
	Slots   []*float64 `json:"slots"`
}

func loadPlan96(node data.NodeEdge) (plan96, bool) {
	point, found := node.Points.Find(plan96Point, "")
	if !found || point.Tombstone%2 != 0 || point.Txt() == "" {
		return plan96{}, false
	}
	var plan plan96
	if json.Unmarshal([]byte(point.Txt()), &plan) != nil {
		return plan96{}, false
	}
	if len(plan.Slots) != 96 {
		return plan96{}, false
	}
	return plan, true
}

func (p plan96) targetAt(t time.Time) (float64, bool) {
	index := (t.Hour()*60 + t.Minute()) / 15
	if index < 0 || index >= len(p.Slots) || p.Slots[index] == nil {
		return 0, false
	}
	return *p.Slots[index], true
}

func (h *Handler) plan96Handler(w http.ResponseWriter, identity Identity, siteID string) {
	if !h.allows(identity, siteID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	node, ok := h.node(w, identity, siteID)
	if !ok {
		return
	}
	if node.Type != NodeTypeSite {
		writeError(w, http.StatusBadRequest, "requires_site")
		return
	}
	plan, found := loadPlan96(node)
	if !found {
		plan = plan96{Version: 1, Slots: make([]*float64, 96)}
	}
	writeJSON(w, plan)
}

type plan96Request struct {
	Site  string     `json:"site"`
	Slots []*float64 `json:"slots"`
}

func (h *Handler) savePlan96Handler(w http.ResponseWriter, identity Identity, req plan96Request) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if len(req.Slots) != 96 {
		writeError(w, http.StatusBadRequest, "slots_must_be_96")
		return
	}
	for _, slot := range req.Slots {
		if slot != nil && (*slot > 1e6 || *slot < -1e6) {
			writeError(w, http.StatusBadRequest, "slot_out_of_range")
			return
		}
	}
	node, ok := h.node(w, identity, req.Site)
	if !ok {
		return
	}
	if node.Type != NodeTypeSite {
		writeError(w, http.StatusBadRequest, "requires_site")
		return
	}
	plan := plan96{Version: 1, Slots: req.Slots}
	encoded, err := marshalString(plan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode_failed")
		return
	}
	if err := client.SendNodePoints(h.nc, req.Site, data.Points{
		data.NewPointString(plan96Point, "0", encoded),
	}, true); err != nil {
		writeError(w, http.StatusBadGateway, "save_failed")
		return
	}
	writeJSON(w, plan)
}

func marshalString(value any) (string, error) {
	encoded, err := json.Marshal(value)
	return string(encoded), err
}
