package appapi

import (
	"encoding/json"
	"net/http"

	"github.com/nats-io/nats.go"
	siotclient "github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"

	"fastiot/internal/decision"
)

// 调度追溯记录的持久化（OBS-01）：把每次调度的 DispatchRecord 追加到
// EnergyUnit 的 easyemsDispatchLog 点，保留最近 50 条，供值班员按
// dispatchID 复盘（T18）。

const dispatchLogKeep = 50

// appendDispatchRecord 把一条调度记录追加到 EnergyUnit 的追溯日志。
func appendDispatchRecord(nc *nats.Conn, energyUnitID string, record decision.DispatchRecord) error {
	records := listDispatchRecords(nc, energyUnitID)
	records = append(records, record)
	if len(records) > dispatchLogKeep {
		records = records[len(records)-dispatchLogKeep:]
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		return err
	}
	point := data.NewPointString(PointTypeDispatchLog, "", string(encoded))
	point.Origin = "capability-dispatch"
	return siotclient.SendNodePoint(nc, energyUnitID, point, true)
}

// listDispatchRecords 读 EnergyUnit 的调度追溯记录（按时间升序）。
func listDispatchRecords(nc *nats.Conn, energyUnitID string) []decision.DispatchRecord {
	nodes, err := siotclient.GetNodes(nc, "all", energyUnitID, "", false)
	if err != nil || len(nodes) == 0 {
		return nil
	}
	point, found := nodes[0].Points.Find(PointTypeDispatchLog, "")
	if !found || point.Txt() == "" {
		return nil
	}
	var records []decision.DispatchRecord
	if json.Unmarshal([]byte(point.Txt()), &records) != nil {
		return nil
	}
	return records
}

// GetDispatchRecord 按 dispatchID 查一条记录（复盘入口）。
func GetDispatchRecord(nc *nats.Conn, energyUnitID, dispatchID string) (decision.DispatchRecord, bool) {
	for _, record := range listDispatchRecords(nc, energyUnitID) {
		if record.DispatchID == dispatchID {
			return record, true
		}
	}
	return decision.DispatchRecord{}, false
}

// dispatchRecords 列出某个能源设备的调度追溯记录（最新在前）。
func (h *Handler) dispatchRecords(w http.ResponseWriter, identity Identity, energyUnitID string) {
	node, ok := h.node(w, identity, energyUnitID)
	if !ok {
		return
	}
	if node.Type != NodeTypeEnergyUnit {
		writeError(w, http.StatusBadRequest, "requires_energy_unit")
		return
	}
	records := listDispatchRecords(h.nc, energyUnitID)
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	writeJSON(w, map[string]any{"records": records})
}
