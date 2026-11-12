package appapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/ems/profile"
)

// 批量应用 Profile：把一个厂商 Profile 应用到一批站号，自动按逻辑点名称匹配物理点位，
// 并给出逐点判定（exact / scaled / missing）与因此不可用的控制。
// 缺回读、点位只读或未映射的控制会被明确阻断，不会用默认点冒充。

type profilePointVerdict struct {
	Name         string  `json:"name"`
	PointID      string  `json:"point_id,omitempty"`
	Verdict      string  `json:"verdict"` // exact | scaled | missing
	ProfileScale float64 `json:"profile_scale"`
	PointScale   float64 `json:"point_scale,omitempty"`
	ReadOnly     bool    `json:"read_only,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

type profileApplyResult struct {
	Server    int                   `json:"server"`
	ProfileID string                `json:"profile_id"`
	Mapped    int                   `json:"mapped"`
	Exact     int                   `json:"exact"`
	Scaled    int                   `json:"scaled"`
	Missing   []string              `json:"missing"`
	Blocked   []string              `json:"blocked"`
	Points    []profilePointVerdict `json:"points"`
	Model     unitModelConfig       `json:"-"`
}

type profileApplyRequest struct {
	ProfileID string `json:"profile_id"`
	Servers   []int  `json:"servers"`
	DryRun    bool   `json:"dry_run"`
}

type profileApplyResponse struct {
	ProfileID string               `json:"profile_id"`
	DryRun    bool                 `json:"dry_run"`
	Results   []profileApplyResult `json:"results"`
	Applied   int                  `json:"applied"`
}

func normalizePointName(name string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "　", "", "／", "/")
	return strings.ToLower(replacer.Replace(strings.TrimSpace(name)))
}

// applyProfile 计算一个站号的映射结果；existing 用于保持业务节点等不随换厂商变化的绑定。
func applyProfile(server int, prof profile.Profile, points []unitModelPointView, existing unitModelConfig) profileApplyResult {
	result := profileApplyResult{
		Server: server, ProfileID: prof.ID,
		Missing: []string{}, Blocked: []string{}, Points: []profilePointVerdict{},
		Model: unitModelConfig{
			Name: prof.Vendor + " " + prof.Model, ProfileID: prof.ID,
			BusinessNodeID: existing.BusinessNodeID,
			PointMap:       map[string]string{}, Verified: false,
		},
	}
	byName := make(map[string]unitModelPointView)
	for _, point := range points {
		if point.ServerID != server {
			continue
		}
		key := normalizePointName(point.Name)
		if key == "" {
			continue
		}
		if _, exists := byName[key]; !exists {
			byName[key] = point
		}
	}
	readOnly := make(map[string]bool)
	for _, spec := range prof.PointSpecs {
		point, ok := byName[normalizePointName(spec.Name)]
		verdict := profilePointVerdict{Name: spec.Name, ProfileScale: spec.Scale}
		if !ok {
			verdict.Verdict = "missing"
			verdict.Reason = "没有同名物理点位"
			result.Missing = append(result.Missing, spec.Name)
		} else {
			verdict.PointID = point.ID
			verdict.PointScale = point.Scale
			verdict.ReadOnly = point.ReadOnly
			if point.Scale != spec.Scale {
				verdict.Verdict = "scaled"
				verdict.Reason = fmt.Sprintf("缩放不一致：Profile %g，点位 %g", spec.Scale, point.Scale)
				result.Scaled++
			} else {
				verdict.Verdict = "exact"
				result.Exact++
			}
			result.Mapped++
			result.Model.PointMap[spec.Name] = point.ID
			readOnly[spec.Name] = point.ReadOnly
		}
		result.Points = append(result.Points, verdict)
	}

	blocked := make(map[string]bool)
	addBlocked := func(message string) {
		if !blocked[message] {
			blocked[message] = true
			result.Blocked = append(result.Blocked, message)
		}
	}
	checkPoint := func(owner, pointName string) {
		if pointName == "" {
			return
		}
		if _, ok := result.Model.PointMap[pointName]; !ok {
			addBlocked(fmt.Sprintf("%s：未映射 %s", owner, pointName))
			return
		}
		if readOnly[pointName] {
			addBlocked(fmt.Sprintf("%s：点位只读 %s", owner, pointName))
		}
	}
	for _, command := range prof.CommandSpecs {
		owner := fmt.Sprintf("命令《%s》", command.Name)
		if command.Point == "" && len(command.Parameters) == 0 {
			addBlocked(owner + "：未配置写点")
			continue
		}
		checkPoint(owner, command.Point)
		for _, parameter := range command.Parameters {
			checkPoint(owner, parameter.Point)
		}
	}
	return result
}

// applyProfilesHandler 处理 POST /api/modbus/{channel}/units/apply。
func (h *Handler) applyProfilesHandler(w http.ResponseWriter, r *http.Request, identity Identity, channelID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	channel, ok := h.node(w, identity, channelID)
	if !ok {
		return
	}
	if channel.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	if h.ems == nil || h.ems.Profiles == nil {
		writeError(w, http.StatusServiceUnavailable, "ems_unavailable")
		return
	}
	var input profileApplyRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	prof, ok := h.ems.Profiles.Get(input.ProfileID)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown_profile")
		return
	}
	if len(input.Servers) == 0 {
		writeError(w, http.StatusBadRequest, "servers_required")
		return
	}
	points := h.unitPoints(channel.ID)
	response := profileApplyResponse{ProfileID: prof.ID, DryRun: input.DryRun, Results: []profileApplyResult{}}
	seen := make(map[int]bool)
	for _, server := range input.Servers {
		if server < 1 || server > 247 || seen[server] {
			continue
		}
		seen[server] = true
		existing, _ := unitModelFor(channel, server)
		result := applyProfile(server, prof, points, existing)
		if !input.DryRun {
			if err := h.saveUnitModel(channel, server, result.Model, points); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			response.Applied++
		}
		result.Model = unitModelConfig{}
		response.Results = append(response.Results, result)
	}
	if len(response.Results) == 0 {
		writeError(w, http.StatusBadRequest, "servers_required")
		return
	}
	writeJSON(w, response)
}
