package appapi

import (
	"net/http"
	"strconv"
	"time"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/decision"
)

// 站级计划（Site Scheduler）：单站内一个总功率目标先按能力分配到各 Plant，
// 再由站内分配落到 EnergyUnit，作为计划目标进入统一仲裁——防逆流、需量
// 等硬优先级仍然胜出，不会出现第二套写设备路径。

func loadSiteTarget(node data.NodeEdge) (float64, bool) {
	point, found := node.Points.Find(PointTypeSiteTarget, "")
	if !found || point.Tombstone%2 != 0 {
		return 0, false
	}
	return pointFloat(point), true
}

// siteGoalEntry caches one site's unit goals within the 1-second dispatch
// cycle so each unit does not recompute the whole site.
type siteGoalEntry struct {
	at    time.Time
	goals map[string]decision.Goal
}

// SiteUnitGoals 计算站级目标落到每个 EnergyUnit 的计划值。
func (h *Handler) SiteUnitGoals(siteID string) map[string]decision.Goal {
	h.siteGoalMu.Lock()
	if entry, ok := h.siteGoalCache[siteID]; ok && time.Since(entry.at) < 800*time.Millisecond {
		cached := make(map[string]decision.Goal, len(entry.goals))
		for key, value := range entry.goals {
			cached[key] = value
		}
		h.siteGoalMu.Unlock()
		return cached
	}
	h.siteGoalMu.Unlock()
	goals := h.computeSiteUnitGoals(siteID)
	h.siteGoalMu.Lock()
	if h.siteGoalCache == nil {
		h.siteGoalCache = make(map[string]siteGoalEntry)
	}
	h.siteGoalCache[siteID] = siteGoalEntry{at: time.Now(), goals: goals}
	h.siteGoalMu.Unlock()
	return goals
}

func (h *Handler) computeSiteUnitGoals(siteID string) map[string]decision.Goal {
	goals := map[string]decision.Goal{}
	nodes, err := siotclient.GetNodes(h.nc, "all", siteID, "", false)
	if err != nil || len(nodes) == 0 {
		return goals
	}
	target, ok := loadSiteTarget(nodes[0])
	source := "site"
	// 96 点计划优先于单一站级目标：当前时段有计划时按计划走，没有计划才回落到站级目标。
	if plan, found := loadPlan96(nodes[0]); found {
		if value, has := plan.targetAt(time.Now()); has {
			target, ok, source = value, true, "plan"
		}
	}
	if !ok || target == 0 {
		return goals
	}
	plants, err := siotclient.GetNodes(h.nc, siteID, "all", NodeTypePlant, false)
	if err != nil || len(plants) == 0 {
		return goals
	}

	unitIDsByPlant := map[string][]string{}
	allIDs := []string{}
	sitePlants := make([]decision.SitePlant, 0, len(plants))
	for _, plant := range plants {
		units, err := siotclient.GetNodes(h.nc, plant.ID, "all", NodeTypeEnergyUnit, false)
		if err != nil {
			continue
		}
		ids := make([]string, 0, len(units))
		for _, unit := range units {
			ids = append(ids, unit.ID)
			allIDs = append(allIDs, unit.ID)
		}
		unitIDsByPlant[plant.ID] = ids
		sitePlants = append(sitePlants, decision.SitePlant{PlantID: plant.ID})
	}
	if len(allIDs) == 0 {
		return goals
	}
	caps, err := LoadCapabilityValues(h.nc, allIDs)
	if err != nil {
		return goals
	}
	for index, plant := range sitePlants {
		charge, discharge, ratedCharge, ratedDischarge := 0.0, 0.0, 0.0, 0.0
		online := false
		for _, unitID := range unitIDsByPlant[plant.PlantID] {
			fields := caps[unitID]
			rated := fields[decision.FieldRatedPower]
			ratedCharge += rated
			ratedDischarge += rated
			if status, found := fields[decision.FieldStatus]; found && status == 0 {
				continue
			}
			unitCharge, hasCharge := fields[decision.FieldAvailableCharge]
			unitDischarge, hasDischarge := fields[decision.FieldAvailableDischarge]
			if !hasCharge && !hasDischarge {
				unitCharge = fields[decision.FieldAvailablePower]
				unitDischarge = fields[decision.FieldAvailablePower]
			}
			if unitCharge > 0 || unitDischarge > 0 {
				online = true
			}
			charge += unitCharge
			discharge += unitDischarge
		}
		sitePlants[index].AvailableChargeKW = charge
		sitePlants[index].AvailableDischargeKW = discharge
		sitePlants[index].RatedChargeKW = ratedCharge
		sitePlants[index].RatedDischargeKW = ratedDischarge
		sitePlants[index].Online = online
	}

	allocations, err := decision.AllocateSite(target, sitePlants)
	if err != nil {
		return goals
	}
	for _, allocation := range allocations {
		if allocation.Excluded || allocation.SetpointKW == 0 {
			continue
		}
		members := make([]decision.EnergyMember, 0)
		for _, unitID := range unitIDsByPlant[allocation.PlantID] {
			members = append(members, decision.EnergyMember{EnergyUnitID: unitID})
		}
		unitAllocations, err := decision.AllocateEnergy(allocation.SetpointKW, members, caps)
		if err != nil {
			continue
		}
		for _, unitAllocation := range unitAllocations {
			if unitAllocation.Excluded {
				continue
			}
			goals[unitAllocation.EnergyUnitID] = decision.Goal{
				Priority: decision.GoalSchedule, RequestKW: unitAllocation.SetpointKW, Source: source,
			}
		}
	}
	return goals
}

// siteTarget 读写 Site 的总功率目标（kW，正=放电，负=充电；0=不计划）。
func (h *Handler) siteTarget(w http.ResponseWriter, r *http.Request, identity Identity, siteID string) {
	node, ok := h.node(w, identity, siteID)
	if !ok {
		return
	}
	if node.Type != NodeTypeSite {
		writeError(w, http.StatusBadRequest, "requires_site")
		return
	}
	switch r.Method {
	case http.MethodGet:
		target, configured := loadSiteTarget(node)
		writeJSON(w, map[string]any{"target_kw": target, "configured": configured})
	case http.MethodPut:
		if !canAction(identity, ActionConfigure) {
			writeError(w, http.StatusForbidden, "admin_required")
			return
		}
		var input struct {
			TargetKW float64 `json:"target_kw"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		point := data.NewPointFloat(PointTypeSiteTarget, "", input.TargetKW)
		point.Origin = identity.UserID
		if err := siotclient.SendNodePoint(h.nc, siteID, point, true); err != nil {
			writeError(w, http.StatusBadGateway, "save_failed")
			return
		}
		writeJSON(w, map[string]any{"saved": true, "target_kw": input.TargetKW, "target": strconv.FormatFloat(input.TargetKW, 'f', -1, 64)})
	}
}
