package appapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"easyems/internal/audit"
)

// 项目版本：把当前站点的工程配置冻结成一个可回退的版本。
// 发布只写版本文件与审计，不改变正在运行的配置，因此不存在"半个版本"；
// 回退/升级按版本重放配置点，历史（审计、报警、命令）不动，也不会回放旧命令。

const projectVersionAction = "项目发布"
const projectRollbackAction = "项目回退"

type versionNode struct {
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	Parent string      `json:"parent"`
	Points data.Points `json:"points"`
}

type projectVersion struct {
	ID          string        `json:"id"`
	SiteID      string        `json:"site_id"`
	SiteName    string        `json:"site_name"`
	CreatedAt   time.Time     `json:"created_at"`
	Operator    string        `json:"operator"`
	Note        string        `json:"note,omitempty"`
	Preflight   string        `json:"preflight_status,omitempty"`
	PreflightID string        `json:"preflight_id,omitempty"`
	Nodes       []versionNode `json:"nodes"`
}

type versionSummary struct {
	ID        string    `json:"id"`
	SiteID    string    `json:"site_id"`
	SiteName  string    `json:"site_name"`
	CreatedAt time.Time `json:"created_at"`
	Operator  string    `json:"operator"`
	Note      string    `json:"note,omitempty"`
	Preflight string    `json:"preflight_status,omitempty"`
	Nodes     int       `json:"nodes"`
}

type rollbackResult struct {
	Version   string `json:"version"`
	Updated   int    `json:"updated"`
	Created   int    `json:"created"`
	Extra     int    `json:"extra"`
	Preflight string `json:"preflight_status,omitempty"`
}

type publishRequest struct {
	Site string `json:"site"`
	Note string `json:"note"`
}

type rollbackRequest struct {
	Site    string `json:"site"`
	Version string `json:"version"`
}

func (h *Handler) projectDir(siteID string) string {
	return filepath.Join(h.dataDir, "projects", siteID)
}

// snapshotSite 冻结当前工程配置；运行时值与凭据不进入版本。
func (h *Handler) snapshotSite(identity Identity, siteID, note string) (projectVersion, error) {
	nodes, err := h.loadReuseSource(identity, siteID)
	if err != nil {
		return projectVersion{}, err
	}
	skipped := skippedNodes(nodes)
	version := projectVersion{
		ID:        "pv-" + data.NewID(),
		SiteID:    siteID,
		SiteName:  nodeName(nodes[0]),
		CreatedAt: time.Now(),
		Operator:  identity.Email,
		Note:      strings.TrimSpace(note),
	}
	if version.Operator == "" {
		version.Operator = identity.UserID
	}
	if report, err := h.buildPreflight(identity); err == nil {
		version.Preflight = report.Status
		version.PreflightID = report.ID
	}
	for _, node := range orderByDepth(nodes) {
		if skipped[node.ID] {
			continue
		}
		points := make(data.Points, 0, len(node.Points))
		for _, p := range node.Points {
			if reuseRuntimePoints[p.Type] || isSecretPoint(p.Type) {
				continue
			}
			points = append(points, p)
		}
		version.Nodes = append(version.Nodes, versionNode{ID: node.ID, Type: node.Type, Parent: node.Parent, Points: points})
	}
	return version, nil
}

func (h *Handler) saveVersion(version projectVersion) error {
	if h.dataDir == "" {
		return fmt.Errorf("data_dir_unavailable")
	}
	dir := h.projectDir(version.SiteID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, version.ID+".tmp")
	final := filepath.Join(dir, version.ID+".json")
	if err := os.WriteFile(tmp, encoded, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (h *Handler) loadVersion(siteID, versionID string) (projectVersion, error) {
	if versionID == "" || strings.ContainsAny(versionID, "/\\.") {
		return projectVersion{}, fmt.Errorf("invalid_version")
	}
	raw, err := os.ReadFile(filepath.Join(h.projectDir(siteID), versionID+".json"))
	if err != nil {
		return projectVersion{}, err
	}
	var version projectVersion
	if err := json.Unmarshal(raw, &version); err != nil {
		return projectVersion{}, err
	}
	return version, nil
}

func (h *Handler) listVersions(siteID string) ([]versionSummary, error) {
	entries, err := os.ReadDir(h.projectDir(siteID))
	if err != nil {
		if os.IsNotExist(err) {
			return []versionSummary{}, nil
		}
		return nil, err
	}
	summaries := make([]versionSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(h.projectDir(siteID), entry.Name()))
		if err != nil {
			continue
		}
		var version projectVersion
		if json.Unmarshal(raw, &version) != nil {
			continue
		}
		summaries = append(summaries, versionSummary{
			ID: version.ID, SiteID: version.SiteID, SiteName: version.SiteName,
			CreatedAt: version.CreatedAt, Operator: version.Operator, Note: version.Note,
			Preflight: version.Preflight, Nodes: len(version.Nodes),
		})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].CreatedAt.After(summaries[j].CreatedAt) })
	return summaries, nil
}

func (h *Handler) publishHandler(w http.ResponseWriter, identity Identity, siteID, note string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	version, err := h.snapshotSite(identity, siteID, note)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.saveVersion(version); err != nil {
		writeError(w, http.StatusInternalServerError, "version_save_failed")
		return
	}
	if h.audit != nil {
		_ = h.audit.Append(audit.Record{
			Time: version.CreatedAt, Actor: version.Operator, Action: projectVersionAction,
			Object: version.ID,
			Detail: fmt.Sprintf("%s · %d 个节点 · 预检 %s", version.SiteName, len(version.Nodes), fallback(version.Preflight, "未运行")),
			Result: "success",
		})
	}
	writeJSON(w, versionSummary{
		ID: version.ID, SiteID: version.SiteID, SiteName: version.SiteName,
		CreatedAt: version.CreatedAt, Operator: version.Operator, Note: version.Note,
		Preflight: version.Preflight, Nodes: len(version.Nodes),
	})
}

func (h *Handler) versionsHandler(w http.ResponseWriter, identity Identity, siteID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if !h.allows(identity, siteID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	summaries, err := h.listVersions(siteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "version_list_failed")
		return
	}
	writeJSON(w, map[string]any{"versions": summaries})
}

func (h *Handler) rollbackHandler(w http.ResponseWriter, identity Identity, siteID, versionID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if !h.allows(identity, siteID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	version, err := h.loadVersion(siteID, versionID)
	if err != nil || version.SiteID != siteID {
		writeError(w, http.StatusNotFound, "version_not_found")
		return
	}
	// 回退前先冻结当前配置，回退失败或需要反悔时有据可查。
	if current, err := h.snapshotSite(identity, siteID, "回退前自动快照"); err == nil {
		_ = h.saveVersion(current)
	}
	currentNodes, err := h.loadReuseSource(identity, siteID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing := make(map[string]bool, len(currentNodes))
	for _, node := range currentNodes {
		existing[node.ID] = true
	}
	result := rollbackResult{Version: version.ID}
	restored := make(map[string]bool, len(version.Nodes))
	for _, node := range version.Nodes {
		points := make(data.Points, 0, len(node.Points))
		for _, p := range node.Points {
			if reuseRuntimePoints[p.Type] || isSecretPoint(p.Type) {
				continue
			}
			// 版本里的点是冻结时的旧时间戳；存储会忽略过期写入，回退必须用当前时间。
			p.Time = time.Now()
			points = append(points, p)
		}
		if existing[node.ID] {
			if len(points) > 0 {
				if err := client.SendPoints(h.nc, client.SubjectNodePoints(node.ID), points, false); err != nil {
					writeError(w, http.StatusBadGateway, "restore_failed")
					return
				}
			}
			result.Updated++
		} else {
			if err := client.SendNode(h.nc, data.NodeEdge{ID: node.ID, Type: node.Type, Parent: node.Parent, Points: points}, identity.UserID); err != nil {
				writeError(w, http.StatusBadGateway, "restore_failed")
				return
			}
			result.Created++
		}
		restored[node.ID] = true
	}
	for _, node := range currentNodes {
		if !restored[node.ID] {
			result.Extra++
		}
	}
	if report, err := h.buildPreflight(identity); err == nil {
		result.Preflight = report.Status
	}
	if h.audit != nil {
		actor := identity.Email
		if actor == "" {
			actor = identity.UserID
		}
		_ = h.audit.Append(audit.Record{
			Time: time.Now(), Actor: actor, Action: projectRollbackAction, Object: version.ID,
			Detail: fmt.Sprintf("更新 %d、创建 %d、保留多余 %d；历史不回滚", result.Updated, result.Created, result.Extra),
			Result: "success",
		})
	}
	writeJSON(w, result)
}

func fallback(value, other string) string {
	if value == "" {
		return other
	}
	return value
}
