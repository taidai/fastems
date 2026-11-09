package appapi

import (
	"fmt"
	"net/http"

	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

// Prometheus 监控：/metrics 暴露系统运行指标（能源节点数、命令状态统计、
// 通道在线数等），供 Prometheus 抓取。这是基础支撑"监控与指标"的落地。

// serveMetrics 输出 Prometheus 文本格式的关键指标。
func (h *Handler) serveMetrics(w http.ResponseWriter, identity Identity) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// 能源节点数。
	units, _ := siotclient.GetNodes(h.nc, "all", "all", NodeTypeEnergyUnit, false)
	fmt.Fprintf(w, "# HELP easyems_energy_units 能源节点数量\n# TYPE easyems_energy_units gauge\neasyems_energy_units %d\n", len(units))

	// 各运行状态的能源节点数。
	stateCount := map[string]int{}
	for _, node := range units {
		stateCount[string(LoadCabinetState(node))]++
	}
	for state, count := range stateCount {
		fmt.Fprintf(w, "easyems_energy_unit_state{state=%q} %d\n", state, count)
	}

	// 通道总数与通信错误通道数。
	channels, _ := siotclient.GetNodes(h.nc, "all", "all", data.NodeTypeModbus, false)
	withErrors := 0
	for _, ch := range channels {
		if point, ok := ch.Points.Find("errorCount", ""); ok && pointFloat(point) > 0 {
			withErrors++
		}
	}
	fmt.Fprintf(w, "# HELP easyems_channels 通道总数与含通信错误的通道数\n# TYPE easyems_channels gauge\neasyems_channels_total %d\neasyems_channels_with_errors %d\n", len(channels), withErrors)

	// 调度追溯记录总数（反映调度活动）。
	totalDispatches := 0
	for _, node := range units {
		totalDispatches += len(listDispatchRecords(h.nc, node.ID))
	}
	fmt.Fprintf(w, "# HELP easyems_dispatch_records 调度追溯记录总数\n# TYPE easyems_dispatch_records gauge\neasyems_dispatch_records %d\n", totalDispatches)
}
