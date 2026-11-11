package appapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/decision"
)

// 项目复用：从一个已交付的站点复制工程资产到新站点。
// 复用的是点表、Profile 绑定、规则、画面与验收配置；地址、凭据和控制授权一律不带入。
// 引用其他节点的配置（PCC 电表、能力输入、功率设定点、规则绑定）需要重新绑定，
// 无法映射的引用会被移除并列入"待确认"，绝不保留旧站 ID 冒充可用。

type reuseItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

type reusePreview struct {
	Source  string         `json:"source"`
	Reuse   []reuseItem    `json:"reuse"`
	Change  []reuseItem    `json:"change"`
	Confirm []reuseItem    `json:"confirm"`
	Counts  map[string]int `json:"counts"`
}

type reuseRequest struct {
	Source string `json:"source"`
	Name   string `json:"name"`
}

type reuseResult struct {
	SiteID  string         `json:"site_id"`
	Created int            `json:"created"`
	Reuse   []reuseItem    `json:"reuse"`
	Change  []reuseItem    `json:"change"`
	Confirm []reuseItem    `json:"confirm"`
	Counts  map[string]int `json:"counts"`
}

const reuseSetpointPoint = "power_setpoint_id"

// 这些类型的节点不进入复用：账号、凭据与登记令牌属于旧站身份，不能复制。
var reuseSkipTypes = map[string]bool{
	"user": true, "deviceCred": true, "authToken": true, "provisioning": true, "enrollment": true, "deviceKey": true,
}

// 运行时值不复制，否则新站会带着旧站的读数与命令状态上线。
var reuseRuntimePoints = map[string]bool{
	"value": true, "quality": true, "time": true, "error": true, "errorCount": true,
	"lastCommand": true, "lastMatch": true, "lastGoodPoll": true, "easyemsManual": true,
	"tombstone": true, "nodeType": true,
}

var reuseSecretTokens = []string{"password", "pass", "token", "secret", "credential", "nkey", "apikey"}

// modbus 节点的连接参数必须重新填写。
var reuseConnectionPoints = map[string]bool{"uri": true, "port": true, "baud": true, "id": true}

func isSecretPoint(typ string) bool {
	lower := strings.ToLower(typ)
	for _, token := range reuseSecretTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func nodeName(node data.NodeEdge) string {
	if name := node.Desc(); name != "" {
		return name
	}
	return node.ID
}

func (h *Handler) reusePreviewHandler(w http.ResponseWriter, identity Identity, source string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	preview, err := h.buildReusePreview(identity, source)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, preview)
}

func (h *Handler) reuseProjectHandler(w http.ResponseWriter, identity Identity, req reuseRequest) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	result, err := h.createProjectFromSource(identity, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, result)
}

func (h *Handler) loadReuseSource(identity Identity, source string) ([]data.NodeEdge, error) {
	if source == "" {
		return nil, fmt.Errorf("source_required")
	}
	if !h.allows(identity, source) {
		return nil, fmt.Errorf("source_forbidden")
	}
	nodes, err := h.loadSubtree(source)
	if err != nil || len(nodes) == 0 {
		return nil, fmt.Errorf("source_not_found")
	}
	if nodes[0].ID != source || nodes[0].Type != NodeTypeSite {
		return nil, fmt.Errorf("source_must_be_site")
	}
	return nodes, nil
}

// loadSubtree 逐层取出节点及其全部后代；节点的子树查询不会自动递归。
func (h *Handler) loadSubtree(rootID string) ([]data.NodeEdge, error) {
	root, err := client.GetNodes(h.nc, "all", rootID, "", false)
	if err != nil || len(root) == 0 {
		return nil, err
	}
	nodes := []data.NodeEdge{root[0]}
	queue := []string{rootID}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		children, err := client.GetNodes(h.nc, parent, "all", "", false)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			nodes = append(nodes, child)
			queue = append(queue, child.ID)
		}
	}
	return nodes, nil
}

func (h *Handler) buildReusePreview(identity Identity, source string) (reusePreview, error) {
	nodes, err := h.loadReuseSource(identity, source)
	if err != nil {
		return reusePreview{}, err
	}
	preview := reusePreview{Source: source, Reuse: []reuseItem{}, Change: []reuseItem{}, Confirm: []reuseItem{}, Counts: map[string]int{}}
	skipped := skippedNodes(nodes)
	for _, node := range nodes {
		if skipped[node.ID] {
			continue
		}
		item := reuseItem{ID: node.ID, Type: node.Type, Name: nodeName(node)}
		switch {
		case node.Type == NodeTypeSite:
			item.Reason = "新站点需要新的名称"
			preview.Change = append(preview.Change, item)
		case node.Type == data.NodeTypeModbus:
			item.Reason = "连接地址与端口需要重新填写"
			preview.Change = append(preview.Change, item)
		case len(referenceReasons(node)) > 0:
			item.Reason = strings.Join(referenceReasons(node), "；")
			preview.Confirm = append(preview.Confirm, item)
		default:
			preview.Reuse = append(preview.Reuse, item)
		}
	}
	preview.Counts = map[string]int{"reuse": len(preview.Reuse), "change": len(preview.Change), "confirm": len(preview.Confirm)}
	return preview, nil
}

func (h *Handler) createProjectFromSource(identity Identity, req reuseRequest) (reuseResult, error) {
	if strings.TrimSpace(req.Name) == "" {
		return reuseResult{}, fmt.Errorf("name_required")
	}
	nodes, err := h.loadReuseSource(identity, req.Source)
	if err != nil {
		return reuseResult{}, err
	}
	source := nodes[0]
	parent := source.Parent
	if parent == "" || !h.allows(identity, parent) {
		return reuseResult{}, fmt.Errorf("target_parent_unavailable")
	}

	order := orderByDepth(nodes)
	skipped := skippedNodes(nodes)
	idMap := make(map[string]string, len(nodes))
	result := reuseResult{Reuse: []reuseItem{}, Change: []reuseItem{}, Confirm: []reuseItem{}, Counts: map[string]int{}}

	// 第一遍：建节点，复制不依赖其他节点的配置。
	for _, node := range order {
		if skipped[node.ID] {
			continue
		}
		newID := data.NewID()
		newParent := parent
		if node.ID != source.ID {
			mapped, ok := idMap[node.Parent]
			if !ok {
				continue
			}
			newParent = mapped
		}
		points := copyConfigPoints(node)
		if node.ID == source.ID {
			points = setDescription(points, req.Name)
			result.SiteID = newID
		}
		if err := client.SendNode(h.nc, data.NodeEdge{ID: newID, Type: node.Type, Parent: newParent, Points: points}, identity.UserID); err != nil {
			return reuseResult{}, fmt.Errorf("clone_failed: %w", err)
		}
		idMap[node.ID] = newID
		result.Created++
	}

	// 第二遍：重写引用；无法映射的引用被移除并列入待确认。
	for _, node := range order {
		newID, ok := idMap[node.ID]
		if !ok {
			continue
		}
		points, unresolved := rewriteReferences(node, idMap)
		if len(points) > 0 {
			if err := client.SendPoints(h.nc, client.SubjectNodePoints(newID), points, false); err != nil {
				return reuseResult{}, fmt.Errorf("rewrite_failed: %w", err)
			}
		}
		for _, ref := range unresolved {
			result.Confirm = append(result.Confirm, reuseItem{
				ID: ref, Type: "reference", Name: ref,
				Reason: fmt.Sprintf("%s 引用的对象未随项目复制，需要重新绑定", nodeName(node)),
			})
		}
	}

	preview, err := h.buildReusePreview(identity, req.Source)
	if err == nil {
		result.Reuse = preview.Reuse
		result.Change = preview.Change
		result.Confirm = append(result.Confirm, preview.Confirm...)
	}
	result.Counts = map[string]int{"created": result.Created, "confirm": len(result.Confirm), "reuse": len(result.Reuse), "change": len(result.Change)}
	return result, nil
}

// skippedNodes 标记不参与复用的节点及其整棵子树。
func skippedNodes(nodes []data.NodeEdge) map[string]bool {
	byID := make(map[string]data.NodeEdge, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	skipped := make(map[string]bool)
	for _, node := range nodes {
		if !reuseSkipTypes[node.Type] {
			continue
		}
		skipped[node.ID] = true
	}
	// 把跳过节点的后代一并跳过。
	changed := true
	for changed {
		changed = false
		for _, node := range nodes {
			if skipped[node.ID] || node.Parent == "" {
				continue
			}
			if skipped[node.Parent] {
				skipped[node.ID] = true
				changed = true
			}
		}
	}
	return skipped
}

// orderByDepth 保证父节点先于子节点创建。
func orderByDepth(nodes []data.NodeEdge) []data.NodeEdge {
	byParent := make(map[string][]data.NodeEdge)
	var roots []data.NodeEdge
	for _, node := range nodes {
		byParent[node.Parent] = append(byParent[node.Parent], node)
	}
	seen := make(map[string]bool)
	if len(nodes) > 0 {
		roots = append(roots, nodes[0])
	}
	ordered := make([]data.NodeEdge, 0, len(nodes))
	var walk func(node data.NodeEdge)
	walk = func(node data.NodeEdge) {
		if seen[node.ID] {
			return
		}
		seen[node.ID] = true
		ordered = append(ordered, node)
		children := byParent[node.ID]
		sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
		for _, child := range children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return ordered
}

func copyConfigPoints(node data.NodeEdge) data.Points {
	points := make(data.Points, 0, len(node.Points))
	for _, p := range node.Points {
		if reuseRuntimePoints[p.Type] || isSecretPoint(p.Type) {
			continue
		}
		if node.Type == data.NodeTypeModbus && reuseConnectionPoints[p.Type] {
			continue
		}
		if hasNodeReference(node, p) {
			continue // 第二遍统一重写
		}
		points = append(points, p)
	}
	return points
}

func setDescription(points data.Points, name string) data.Points {
	for i := range points {
		if points[i].Type == "description" {
			points[i] = data.NewPointString("description", "0", name)
			return points
		}
	}
	return append(points, data.NewPointString("description", "0", name))
}

func hasNodeReference(node data.NodeEdge, p data.Point) bool {
	switch p.Type {
	case PointTypePCCFeedback, capabilityInputsPoint, reuseSetpointPoint, "applied", "draft":
		return true
	}
	return false
}

func referenceReasons(node data.NodeEdge) []string {
	reasons := []string{}
	for _, p := range node.Points {
		switch p.Type {
		case PointTypePCCFeedback:
			var binding decision.PCCFeedback
			if json.Unmarshal([]byte(p.Txt()), &binding) == nil && (binding.MeterPointID != "" || len(binding.PhasePointIDs) > 0) {
				reasons = append(reasons, "PCC 反馈电表与三相点位需要重新绑定")
			}
		case capabilityInputsPoint:
			var inputs CapabilityInputs
			if json.Unmarshal([]byte(p.Txt()), &inputs) == nil && (inputs.SOCPointID != "" || inputs.TempPointID != "" || inputs.ChargeLimitPointID != "" || inputs.DischargeLimitPointID != "") {
				reasons = append(reasons, "能力输入点位需要重新绑定")
			}
		case reuseSetpointPoint:
			if p.Txt() != "" {
				reasons = append(reasons, "功率设定点位需要重新绑定")
			}
		case "applied", "draft":
			if node.Type == "decisionTable" && strings.TrimSpace(p.Txt()) != "" {
				reasons = append(reasons, "规则绑定点位需要重新确认")
			}
		}
	}
	return reasons
}

// rewriteReferences 依据旧→新 ID 映射改写配置；返回可发送的点与无法解析的引用。
func rewriteReferences(node data.NodeEdge, idMap map[string]string) (data.Points, []string) {
	var points data.Points
	var unresolved []string
	for _, p := range node.Points {
		switch p.Type {
		case PointTypePCCFeedback:
			var binding decision.PCCFeedback
			if json.Unmarshal([]byte(p.Txt()), &binding) != nil {
				continue
			}
			if binding.MeterPointID != "" {
				if mapped, ok := idMap[binding.MeterPointID]; ok {
					binding.MeterPointID = mapped
				} else {
					unresolved = append(unresolved, binding.MeterPointID)
					binding.MeterPointID = ""
				}
			}
			phases := make([]string, 0, len(binding.PhasePointIDs))
			for _, id := range binding.PhasePointIDs {
				if mapped, ok := idMap[id]; ok {
					phases = append(phases, mapped)
				} else if id != "" {
					unresolved = append(unresolved, id)
				}
			}
			binding.PhasePointIDs = phases
			encoded, _ := json.Marshal(binding)
			points = append(points, data.NewPointString(PointTypePCCFeedback, "0", string(encoded)))
		case capabilityInputsPoint:
			var inputs CapabilityInputs
			if json.Unmarshal([]byte(p.Txt()), &inputs) != nil {
				continue
			}
			rewrite := func(id string) string {
				if id == "" {
					return ""
				}
				if mapped, ok := idMap[id]; ok {
					return mapped
				}
				unresolved = append(unresolved, id)
				return ""
			}
			inputs.SOCPointID = rewrite(inputs.SOCPointID)
			inputs.TempPointID = rewrite(inputs.TempPointID)
			inputs.ChargeLimitPointID = rewrite(inputs.ChargeLimitPointID)
			inputs.DischargeLimitPointID = rewrite(inputs.DischargeLimitPointID)
			encoded, _ := json.Marshal(inputs)
			points = append(points, data.NewPointString(capabilityInputsPoint, "0", string(encoded)))
		case reuseSetpointPoint:
			old := p.Txt()
			if old == "" {
				continue
			}
			if mapped, ok := idMap[old]; ok {
				points = append(points, data.NewPointString(reuseSetpointPoint, "0", mapped))
			} else {
				unresolved = append(unresolved, old)
			}
		case "applied", "draft":
			if node.Type != "decisionTable" || strings.TrimSpace(p.Txt()) == "" {
				continue
			}
			var document decision.Document
			if json.Unmarshal([]byte(p.Txt()), &document) != nil {
				continue
			}
			rewriteTargets := func(targets map[string]decision.Target) map[string]decision.Target {
				updated := make(map[string]decision.Target, len(targets))
				for key, target := range targets {
					if target.NodeID == "" {
						updated[key] = target
						continue
					}
					if mapped, ok := idMap[target.NodeID]; ok {
						target.NodeID = mapped
						updated[key] = target
					} else {
						unresolved = append(unresolved, target.NodeID)
					}
				}
				return updated
			}
			document.Bindings.Points = rewriteTargets(document.Bindings.Points)
			document.Bindings.Actions = rewriteTargets(document.Bindings.Actions)
			encoded, _ := json.Marshal(document)
			points = append(points, data.NewPointString(p.Type, "0", string(encoded)))
		}
	}
	return points, unresolved
}
