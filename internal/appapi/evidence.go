package appapi

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// deliveryEvidence exports one zip with the delivery evidence CSVs: device
// ledger, verification records, communication test report and alarm stats.
// Everything is read from existing sources; no passwords or tokens appear in
// any of them.
func (h *Handler) deliveryEvidence(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionExport) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	if h.ems == nil || h.ems.Profiles == nil || h.ems.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "ems_unavailable")
		return
	}
	devices, err := h.ledgerDevices(identity)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	verifications, err := h.verificationList(identity)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "nodes_unavailable")
		return
	}
	stats := h.alarmStatsData(identity, 30)
	channelStats, deviceStats := h.ems.QualityReport()
	names := map[string]string{}
	if nodes, err := h.visibleNodes(identity); err == nil {
		for _, node := range nodes {
			names[node.ID] = node.Desc()
		}
	}
	channelName := func(id string) string {
		if name := names[id]; name != "" {
			return name
		}
		return id
	}

	now := time.Now()
	actor := identity.Email
	if actor == "" {
		actor = identity.UserID
	}
	buffer := new(bytes.Buffer)
	archive := zip.NewWriter(buffer)
	fail := func() {
		writeError(w, http.StatusInternalServerError, "evidence_failed")
	}
	writeCSV := func(name string, rows [][]string) bool {
		file, err := archive.Create(name)
		if err != nil {
			return false
		}
		if _, err := file.Write([]byte("\xef\xbb\xbf")); err != nil {
			return false
		}
		writer := csv.NewWriter(file)
		return writer.WriteAll(rows) == nil
	}

	readme, err := archive.Create("00-说明.txt")
	if err != nil {
		fail()
		return
	}
	fmt.Fprintf(readme, "easyems 交付证据包\n导出时间: %s\n导出人: %s\n\n内容:\n", now.Format("2006-01-02 15:04:05"), actor)
	fmt.Fprint(readme, "  01-设备台账.csv       接入设备、Profile 绑定与验证状态\n")
	fmt.Fprint(readme, "  02-验证记录.csv       每台设备的现场验证人、日期、序列号与备注\n")
	fmt.Fprint(readme, "  03-通信测试报告.csv   通道/设备轮询统计与各设备点位质量汇总\n")
	fmt.Fprint(readme, "  04-报警统计.csv       最近 30 天报警趋势与来源排名\n\n")
	fmt.Fprint(readme, "边界:\n")
	fmt.Fprint(readme, "  - 通信统计为本次运行期间的轮询计数，进程重启后清零。\n")
	fmt.Fprint(readme, "  - 证据包不包含密码、令牌等敏感信息。\n")
	fmt.Fprint(readme, "  - 模拟器或当前实例的检查结果不替代真实设备现场签收。\n")

	percent := func(polls int, rate float64) string {
		if polls <= 0 {
			return ""
		}
		return strconv.FormatFloat(rate, 'f', 1, 64)
	}
	millis := func(value float64) string {
		if value <= 0 {
			return ""
		}
		return strconv.Itoa(int(value + 0.5))
	}
	timeOf := func(value time.Time) string {
		if value.IsZero() {
			return ""
		}
		return value.Format("2006-01-02 15:04:05")
	}

	ledgerRows := [][]string{{"通道", "站号", "设备名称", "业务节点", "Profile", "厂商", "型号", "协议", "地址", "点位数", "命令数", "健康", "成功率%", "平均延迟ms", "已验证", "验证人", "验证日期", "设备序列号"}}
	for _, device := range devices {
		verified := "否"
		if device.Verified {
			verified = "是"
		}
		rate := ""
		if device.SuccessRate > 0 {
			rate = strconv.FormatFloat(device.SuccessRate, 'f', 1, 64)
		}
		ledgerRows = append(ledgerRows, []string{
			device.ChannelName, strconv.Itoa(device.ServerID), device.Name, device.BusinessNodeName,
			device.ProfileID, device.Vendor, device.Model, device.Protocol, device.Address,
			strconv.Itoa(device.PointCount), strconv.Itoa(device.CommandCount), device.Health,
			rate, millis(device.AvgLatencyMS), verified, device.Verifier, device.VerifiedAt, device.DeviceSerial,
		})
	}
	if !writeCSV("01-设备台账.csv", ledgerRows) {
		fail()
		return
	}

	verificationFile, err := archive.Create("02-验证记录.csv")
	if err != nil {
		fail()
		return
	}
	if _, err := verificationFile.Write([]byte("\xef\xbb\xbf")); err != nil {
		fail()
		return
	}
	writeVerificationCSV(csv.NewWriter(verificationFile), verifications)

	qualityRows := [][]string{
		{"通信测试报告"},
		{"导出时间", now.Format("2006-01-02 15:04:05")},
		{"说明", "统计为本次运行期间的轮询计数，进程重启后清零"},
		{},
		{"通道", "轮询次数", "失败次数", "成功率%", "平均延迟ms", "最大延迟ms", "最近成功", "最近错误"},
	}
	for _, item := range channelStats {
		qualityRows = append(qualityRows, []string{
			channelName(item.ChannelID), strconv.Itoa(item.Polls), strconv.Itoa(item.Failures),
			percent(item.Polls, item.SuccessRate), millis(item.AvgLatencyMS), millis(item.MaxLatencyMS),
			timeOf(item.LastSuccess), item.LastError,
		})
	}
	qualityRows = append(qualityRows, []string{}, []string{"设备", "通道", "健康", "轮询次数", "失败次数", "成功率%", "平均延迟ms", "最大延迟ms", "最近成功", "最近错误"})
	for _, item := range deviceStats {
		qualityRows = append(qualityRows, []string{
			item.DeviceID, channelName(item.ChannelID), item.Health,
			strconv.Itoa(item.Polls), strconv.Itoa(item.Failures),
			percent(item.Polls, item.SuccessRate), millis(item.AvgLatencyMS), millis(item.MaxLatencyMS),
			timeOf(item.LastSuccess), item.LastError,
		})
	}
	pointCounts := map[string]map[string]int{}
	for _, value := range h.ems.Service.Snapshot() {
		bucket := pointCounts[value.DeviceID]
		if bucket == nil {
			bucket = map[string]int{}
			pointCounts[value.DeviceID] = bucket
		}
		bucket[value.Quality.String()]++
	}
	deviceIDs := make([]string, 0, len(pointCounts))
	for id := range pointCounts {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)
	qualityRows = append(qualityRows, []string{}, []string{"设备点位质量", "正常", "过期", "无效", "未初始化"})
	for _, id := range deviceIDs {
		bucket := pointCounts[id]
		qualityRows = append(qualityRows, []string{
			id, strconv.Itoa(bucket["good"]), strconv.Itoa(bucket["stale"]),
			strconv.Itoa(bucket["invalid"]), strconv.Itoa(bucket["uninitialized"]),
		})
	}
	if !writeCSV("03-通信测试报告.csv", qualityRows) {
		fail()
		return
	}

	alarmRows := [][]string{
		{"报警统计（最近 30 天）"},
		{"导出时间", now.Format("2006-01-02 15:04:05")},
		{},
		{"日期", "新增报警", "未处理", "已处理"},
	}
	for _, day := range stats.Trend {
		alarmRows = append(alarmRows, []string{day.Date, strconv.Itoa(day.Raised), strconv.Itoa(day.Unhandled), strconv.Itoa(day.Handled)})
	}
	alarmRows = append(alarmRows, []string{}, []string{"报警来源排名", "次数"})
	for _, item := range stats.Ranking {
		alarmRows = append(alarmRows, []string{item.Name, strconv.Itoa(item.Count)})
	}
	if !writeCSV("04-报警统计.csv", alarmRows) {
		fail()
		return
	}

	if err := archive.Close(); err != nil {
		fail()
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="easyems-evidence-%s.zip"`, now.Format("2006-01-02")))
	_, _ = w.Write(buffer.Bytes())
}
