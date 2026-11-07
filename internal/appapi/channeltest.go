package appapi

import (
	"net/http"
	"time"

	"github.com/simpleiot/simpleiot/data"
)

// LineTestStation is one probed station address on a channel.
type LineTestStation struct {
	ServerID  int    `json:"server_id"`
	Responded bool   `json:"responded"`
	LatencyMS int64  `json:"latency_ms"`
	Probe     string `json:"probe"`
	Error     string `json:"error"`
}

// LineTestResult is the read-only connectivity report for a channel. It never
// changes configuration and never writes a value to the device.
type LineTestResult struct {
	Connected bool              `json:"connected"`
	LatencyMS int64             `json:"latency_ms"`
	Probed    int               `json:"probed"`
	Stations  []LineTestStation `json:"stations"`
}

// modbusConnectionFromNode reconstructs the persisted connection parameters of
// a Modbus channel node so the same validation and transport code used for the
// port form also drives the read-only line test.
func modbusConnectionFromNode(node data.NodeEdge) (ModbusConnectionInput, bool) {
	input := ModbusConnectionInput{
		Parent:      node.Parent,
		Description: textPoint(node.Points, data.PointTypeDescription),
		Protocol:    textPoint(node.Points, data.PointTypeProtocol),
		URI:         textPoint(node.Points, data.PointTypeURI),
		Port:        textPoint(node.Points, data.PointTypePort),
		Baud:        textPoint(node.Points, data.PointTypeBaud),
		DataBits:    int(numberPoint(node.Points, "dataBits")),
		Parity:      textPoint(node.Points, "parity"),
		StopBits:    int(numberPoint(node.Points, "stopBits")),
		PollPeriod:  int(numberPoint(node.Points, data.PointTypePollPeriod)),
		Timeout:     int(numberPoint(node.Points, data.PointTypeTimeout)),
		Disabled:    numberPoint(node.Points, data.PointTypeDisabled) != 0,
	}
	normalizeSerial(&input)
	if validateModbusConnection(input) != nil {
		return ModbusConnectionInput{}, false
	}
	return input, true
}

// lineTestModbus probes an existing channel's connectivity and station
// readability without touching configuration or writing any value.
func (h *Handler) lineTestModbus(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	if node.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	connection, ok := modbusConnectionFromNode(node)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_modbus_connection")
		return
	}
	var input struct {
		ServerID int `json:"server_id"`
		From     int `json:"from"`
		To       int `json:"to"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	from, to := input.From, input.To
	if input.ServerID != 0 {
		if input.ServerID < 1 || input.ServerID > 247 {
			writeError(w, http.StatusBadRequest, "invalid_server")
			return
		}
		from, to = input.ServerID, input.ServerID
	}
	if from == 0 {
		from = 1
	}
	if to == 0 {
		to = 16
	}
	if from < 1 || to > 247 || from > to {
		writeError(w, http.StatusBadRequest, "invalid_range")
		return
	}
	if to-from+1 > 64 {
		writeError(w, http.StatusBadRequest, "range_too_large")
		return
	}
	writeJSON(w, probeLine(connection, from, to))
}

// probeLine opens the channel transport and reads a holding register, falling
// back to an input register, at address 0 for each station in the range.
func probeLine(connection ModbusConnectionInput, from, to int) LineTestResult {
	result := LineTestResult{Probed: to - from + 1, Stations: make([]LineTestStation, 0, to-from+1)}
	clientConnection, closeFn, err := openModbusClient(connection)
	if err != nil {
		for unit := from; unit <= to; unit++ {
			result.Stations = append(result.Stations, LineTestStation{ServerID: unit, Error: err.Error()})
		}
		return result
	}
	defer func() { _ = closeFn() }()
	result.Connected = true
	for unit := from; unit <= to; unit++ {
		start := time.Now()
		probe := "holdingRegister:0"
		_, readErr := clientConnection.ReadHoldingRegs(byte(unit), 0, 1)
		if readErr != nil {
			probe = "inputRegister:0"
			_, readErr = clientConnection.ReadInputRegs(byte(unit), 0, 1)
		}
		station := LineTestStation{ServerID: unit, LatencyMS: time.Since(start).Milliseconds()}
		if readErr == nil {
			station.Responded = true
			station.Probe = probe
			if result.LatencyMS == 0 || station.LatencyMS < result.LatencyMS {
				result.LatencyMS = station.LatencyMS
			}
		} else {
			station.Error = readErr.Error()
		}
		result.Stations = append(result.Stations, station)
	}
	return result
}
