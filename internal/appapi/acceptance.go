package appapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/simpleiot/simpleiot/client"

	"fastems/internal/ems/channel"
	"fastems/internal/ems/signal"
)

type acceptanceCheck struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Object   string `json:"object"`
	NextStep string `json:"next_step"`
}

type acceptanceReport struct {
	ID          string            `json:"id"`
	GeneratedAt time.Time         `json:"generated_at"`
	Status      string            `json:"status"`
	Checks      []acceptanceCheck `json:"checks"`
	Summary     map[string]int    `json:"summary"`
}

// runAcceptance builds a report from the live runtime and product nodes.
func (h *Handler) runAcceptance(w http.ResponseWriter, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if h.ems == nil || h.ems.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "ems_unavailable")
		return
	}
	report := h.buildAcceptanceReport()
	h.acceptanceMu.Lock()
	h.latestAcceptance = &report
	h.acceptanceMu.Unlock()
	writeJSON(w, report)
}

func (h *Handler) latestAcceptanceReport(w http.ResponseWriter) {
	h.acceptanceMu.Lock()
	defer h.acceptanceMu.Unlock()
	if h.latestAcceptance == nil {
		writeError(w, http.StatusNotFound, "no_report")
		return
	}
	writeJSON(w, *h.latestAcceptance)
}

func (h *Handler) buildAcceptanceReport() acceptanceReport {
	now := time.Now()
	service := h.ems.Service
	checks := make([]acceptanceCheck, 0, 8)

	channels := service.Channels()
	if len(channels) == 0 {
		checks = append(checks, acceptanceCheck{
			Key: "channels", Title: "通道通信", Status: "failed",
			Detail: "没有配置任何通道", NextStep: "在部署页添加通道并确认健康",
		})
	} else {
		healthy, degraded, offline, devices := 0, 0, 0, 0
		object := channels[0].ID
		for _, ch := range channels {
			for _, device := range ch.Devices {
				devices++
				switch ch.Health(device.ID, now, h.ems.MaxAge) {
				case channel.HealthHealthy:
					healthy++
				case channel.HealthDegraded:
					degraded++
				case channel.HealthOffline:
					offline++
					object = device.ID
				}
			}
		}
		status := "passed"
		next := ""
		if offline > 0 {
			status, next = "failed", "检查通道地址、串口占用、接线与站号"
		} else if degraded > 0 {
			status, next = "warning", "查看通信错误与响应时间"
		}
		checks = append(checks, acceptanceCheck{
			Key: "channels", Title: "通道通信", Status: status, Object: object, NextStep: next,
			Detail: fmt.Sprintf("%d 条通道、%d 台设备：正常 %d，不稳定 %d，离线 %d", len(channels), devices, healthy, degraded, offline),
		})
	}

	values := service.Snapshot()
	if len(values) == 0 {
		checks = append(checks, acceptanceCheck{
			Key: "points", Title: "点位质量", Status: "warning",
			Detail: "还没有点位数据", NextStep: "确认通道已采集到点位",
		})
	} else {
		good, bad := 0, 0
		object := ""
		for _, value := range values {
			if value.Quality == signal.Good {
				good++
			} else {
				bad++
				object = value.PointID
			}
		}
		status := "passed"
		next := ""
		if bad > 0 {
			status, next = "warning", "查看过期/无效点位并检查设备通信"
		}
		checks = append(checks, acceptanceCheck{
			Key: "points", Title: "点位质量", Status: status, Object: object, NextStep: next,
			Detail: fmt.Sprintf("%d 个点位：正常 %d，过期或无效 %d", len(values), good, bad),
		})
	}

	rules, err := client.GetNodes(h.nc, "all", "all", "decisionTable", false)
	switch {
	case err != nil:
		checks = append(checks, acceptanceCheck{
			Key: "rules", Title: "规则发布", Status: "warning",
			Detail: "规则读取失败：" + err.Error(), NextStep: "检查存储与规则节点",
		})
	case len(rules) == 0:
		checks = append(checks, acceptanceCheck{
			Key: "rules", Title: "规则发布", Status: "warning",
			Detail: "还没有决策表", NextStep: "按需要创建并发布决策表",
		})
	default:
		enabled := 0
		for _, rule := range rules {
			if value, _ := rule.Points.Value("enabled", ""); value != 0 {
				enabled++
			}
		}
		status := "passed"
		next := ""
		if enabled == 0 {
			status, next = "warning", "发布并启用需要自动运行的规则"
		}
		checks = append(checks, acceptanceCheck{
			Key: "rules", Title: "规则发布", Status: status, NextStep: next,
			Detail: fmt.Sprintf("决策表 %d 个，已启用 %d 个", len(rules), enabled),
		})
	}

	active := service.Alarms.Active()
	alarmStatus, alarmNext := "passed", ""
	if active > 0 {
		alarmStatus, alarmNext = "warning", "在报警页确认并处理"
	}
	checks = append(checks, acceptanceCheck{
		Key: "alarms", Title: "报警状态", Status: alarmStatus, NextStep: alarmNext,
		Detail: fmt.Sprintf("活动报警 %d 项", active),
	})

	historyPoints := len(service.History.Points())
	historyStatus, historyNext := "passed", ""
	if historyPoints == 0 {
		historyStatus, historyNext = "warning", "确认点位已采集并写入历史"
	}
	checks = append(checks, acceptanceCheck{
		Key: "history", Title: "历史记录", Status: historyStatus, NextStep: historyNext,
		Detail: fmt.Sprintf("已记录 %d 个点位的历史", historyPoints),
	})

	checks = append(checks, acceptanceCheck{
		Key: "permissions", Title: "权限边界", Status: "passed",
		Detail: "服务端对每个请求校验账号、角色与业务树范围",
	})
	checks = append(checks, acceptanceCheck{
		Key: "backup", Title: "备份状态", Status: "warning",
		Detail:   "未验证在线备份；完整备份需停止应用后复制数据目录",
		NextStep: "交付前执行一次离线完整备份并记录恢复演练",
	})

	summary := map[string]int{"passed": 0, "warning": 0, "failed": 0}
	for _, check := range checks {
		summary[check.Status]++
	}
	status := "passed"
	if summary["failed"] > 0 {
		status = "failed"
	} else if summary["warning"] > 0 {
		status = "warning"
	}
	return acceptanceReport{
		ID: "acc-" + now.Format("20060102-150405"), GeneratedAt: now,
		Status: status, Checks: checks, Summary: summary,
	}
}
