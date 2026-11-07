package appapi

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"github.com/simpleiot/simpleiot/data"
)

type historyReader interface {
	PointHistory(nodeID, typ, key string, start, end time.Time, limit int) ([]data.Point, error)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request, identity Identity, export bool) {
	nodeID := r.URL.Query().Get("node_id")
	node, ok := h.node(w, identity, nodeID)
	if !ok {
		return
	}
	if node.Type != data.NodeTypeVariable && node.Type != data.NodeTypeModbusIO {
		writeError(w, http.StatusBadRequest, "history_not_supported")
		return
	}
	typ := r.URL.Query().Get("type")
	if typ == "" {
		typ = data.PointTypeValue
	}
	if typ != data.PointTypeValue && typ != data.PointTypeValueSet {
		writeError(w, http.StatusForbidden, "history_type_not_allowed")
		return
	}
	var start, end time.Time
	var err error
	if text := r.URL.Query().Get("start"); text != "" {
		start, err = time.Parse(time.RFC3339, text)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_time_range")
			return
		}
	}
	if text := r.URL.Query().Get("end"); text != "" {
		end, err = time.Parse(time.RFC3339, text)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_time_range")
			return
		}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_time_range")
		return
	}
	limit := 500
	if text := r.URL.Query().Get("limit"); text != "" {
		limit, err = strconv.Atoi(text)
	}
	if err != nil || limit < 1 || limit > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_limit")
		return
	}
	reader, ok := h.users.(historyReader)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "history_unavailable")
		return
	}
	points, err := reader.PointHistory(nodeID, typ, r.URL.Query().Get("key"), start, end, limit)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "history_unavailable")
		return
	}
	if export {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="point-history.csv"`)
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"时间", "数值", "文本", "来源"})
		for _, point := range points {
			value, _ := point.ValueFloat()
			_ = writer.Write([]string{point.Time.Format(time.RFC3339), strconv.FormatFloat(value, 'g', -1, 64), point.Txt(), point.Origin})
		}
		writer.Flush()
		return
	}
	writeJSON(w, map[string]any{"node_id": nodeID, "type": typ, "points": points})
}
