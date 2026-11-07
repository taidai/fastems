package appapi

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
	siotmodbus "github.com/simpleiot/simpleiot/modbus"
	"github.com/simpleiot/simpleiot/respreader"
	"go.bug.st/serial"
)

type ModbusConnectionInput struct {
	Parent      string `json:"parent"`
	Description string `json:"description"`
	Protocol    string `json:"protocol"`
	URI         string `json:"uri"`
	Port        string `json:"port"`
	Baud        string `json:"baud"`
	DataBits    int    `json:"data_bits"`
	Parity      string `json:"parity"`
	StopBits    int    `json:"stop_bits"`
	PollPeriod  int    `json:"poll_period"`
	Timeout     int    `json:"timeout"`
	Disabled    bool   `json:"disabled"`
}

func normalizeSerial(input *ModbusConnectionInput) {
	if input.Protocol != data.PointValueRTU {
		return
	}
	if input.DataBits == 0 {
		input.DataBits = 8
	}
	if input.Parity == "" {
		input.Parity = "N"
	}
	input.Parity = strings.ToUpper(input.Parity)
	if input.StopBits == 0 {
		input.StopBits = 1
	}
}

func serialPoints(input ModbusConnectionInput) data.Points {
	return data.Points{
		data.NewPointInt("dataBits", "", int64(input.DataBits)),
		data.NewPointString("parity", "", input.Parity),
		data.NewPointInt("stopBits", "", int64(input.StopBits)),
	}
}

type ModbusIOInput struct {
	Parent      string  `json:"parent"`
	Description string  `json:"description"`
	ServerID    int     `json:"server_id"`
	Address     int     `json:"address"`
	IOType      string  `json:"io_type"`
	DataFormat  string  `json:"data_format"`
	ReadOnly    bool    `json:"read_only"`
	Scale       float64 `json:"scale"`
	Offset      float64 `json:"offset"`
	Units       string  `json:"units"`
	Disabled    bool    `json:"disabled"`
}

func (h *Handler) createModbus(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input ModbusConnectionInput
	if !decodeJSON(w, r, &input) {
		return
	}
	normalizeSerial(&input)
	if input.Description == "" || !h.allows(identity, input.Parent) || validateModbusConnection(input) != nil {
		writeError(w, http.StatusBadRequest, "invalid_modbus_connection")
		return
	}
	if input.PollPeriod == 0 {
		input.PollPeriod = 1000
	}
	if input.Timeout == 0 {
		input.Timeout = 1000
	}
	id := data.NewID()
	err := client.SendNodeType(h.nc, client.Modbus{
		ID: id, Parent: input.Parent, Description: input.Description,
		ClientServer: data.PointValueClient, Protocol: input.Protocol,
		URI: input.URI, Port: input.Port, Baud: input.Baud,
		PollPeriod: input.PollPeriod, Timeout: input.Timeout, Disabled: input.Disabled,
	}, identity.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create_failed")
		return
	}
	if err := client.SendNodePoints(h.nc, id, serialPoints(input), true); err != nil {
		writeError(w, http.StatusBadRequest, "create_failed")
		return
	}
	writeJSON(w, map[string]string{"id": id})
}

func (h *Handler) createModbusIO(w http.ResponseWriter, r *http.Request, identity Identity, busID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	bus, ok := h.node(w, identity, busID)
	if !ok {
		return
	}
	if bus.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	var input ModbusIOInput
	input.Parent = busID
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Parent = busID
	if err := validateModbusIO(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	encoded, err := newModbusIONode(busID, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_modbus_point")
		return
	}
	if err := client.SendNode(h.nc, encoded, identity.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "create_failed")
		return
	}
	writeJSON(w, map[string]string{"id": encoded.ID})
}

func newModbusIONode(busID string, input ModbusIOInput) (data.NodeEdge, error) {
	if input.Scale == 0 {
		input.Scale = 1
	}
	encoded, err := data.Encode(client.ModbusIo{
		ID: data.NewID(), Parent: busID, Description: input.Description,
		ServerID: input.ServerID, Address: input.Address,
		ModbusIOType: input.IOType, DataFormat: input.DataFormat,
		ReadOnly: input.ReadOnly, Scale: input.Scale, Offset: input.Offset, Disabled: input.Disabled,
	})
	if err != nil {
		return data.NodeEdge{}, err
	}
	encoded.Points = append(encoded.Points,
		data.NewPointString("businessId", "", data.NewID()),
		data.NewPointString(data.PointTypeUnits, "", input.Units))
	return encoded, nil
}

func (h *Handler) importModbusCSV(w http.ResponseWriter, r *http.Request, identity Identity, busID string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	bus, ok := h.node(w, identity, busID)
	if !ok {
		return
	}
	if bus.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil || !utf8.Valid(content) {
		writeError(w, http.StatusBadRequest, "invalid_utf8_csv")
		return
	}
	reader := csv.NewReader(bytes.NewReader(content))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		writeError(w, http.StatusBadRequest, "empty_or_invalid_csv")
		return
	}
	columns := map[string]int{}
	for i, name := range records[0] {
		name = strings.TrimSpace(strings.TrimPrefix(name, "\xef\xbb\xbf"))
		if _, exists := columns[name]; exists {
			writeError(w, http.StatusBadRequest, "duplicate_csv_column")
			return
		}
		columns[name] = i
	}
	for _, required := range []string{"名称", "站号", "地址", "类型"} {
		if _, ok := columns[required]; !ok {
			writeError(w, http.StatusBadRequest, "missing_csv_column: "+required)
			return
		}
	}
	query := r.URL.Query()
	diffMode := query.Get("diff") == "true" || query.Get("apply") == "diff"
	existing := map[string]bool{}
	if !diffMode {
		children, _ := client.GetNodes(h.nc, busID, "all", "", false)
		for _, child := range children {
			name, _ := child.Points.Text(data.PointTypeDescription, "")
			existing[name] = true
		}
	}

	inputs := []ModbusIOInput{}
	for rowIndex, record := range records[1:] {
		blank := true
		for _, cell := range record {
			if strings.TrimSpace(cell) != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		if len(record) != len(records[0]) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行字段数量不正确", rowIndex+2))
			return
		}
		cell := func(name string) string {
			index, ok := columns[name]
			if !ok {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		serverID, err1 := strconv.Atoi(cell("站号"))
		address, err2 := strconv.Atoi(cell("地址"))
		if err1 != nil || err2 != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行站号或地址无效", rowIndex+2))
			return
		}
		typeNames := map[string]string{
			"保持寄存器": data.PointValueModbusHoldingRegister,
			"输入寄存器": data.PointValueModbusInputRegister,
			"线圈":    data.PointValueModbusCoil,
			"离散输入":  data.PointValueModbusDiscreteInput,
		}
		ioType := cell("类型")
		if mapped, ok := typeNames[ioType]; ok {
			ioType = mapped
		}
		readOnly := ioType == data.PointValueModbusDiscreteInput || ioType == data.PointValueModbusInputRegister
		if text := cell("只读"); text != "" {
			readOnly, err = parseCSVBool(text)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行只读字段无效", rowIndex+2))
				return
			}
		}
		scale := 1.0
		offset := 0.0
		if text := cell("缩放"); text != "" {
			scale, err = strconv.ParseFloat(text, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行缩放无效", rowIndex+2))
				return
			}
		}
		if text := cell("偏移"); text != "" {
			offset, err = strconv.ParseFloat(text, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行偏移无效", rowIndex+2))
				return
			}
		}
		input := ModbusIOInput{
			Parent: busID, Description: cell("名称"), ServerID: serverID, Address: address,
			IOType: ioType, DataFormat: cell("数据格式"), ReadOnly: readOnly,
			Scale: scale, Offset: offset, Units: cell("单位"),
		}
		if input.DataFormat == "" {
			input.DataFormat = data.PointValueUINT16
		}
		if err := validateModbusIO(input); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("第 %d 行: %s", rowIndex+2, err.Error()))
			return
		}
		if existing[input.Description] {
			writeError(w, http.StatusConflict, "duplicate_point_name: "+input.Description)
			return
		}
		existing[input.Description] = true
		inputs = append(inputs, input)
	}
	if len(inputs) == 0 {
		writeError(w, http.StatusBadRequest, "empty_or_invalid_csv")
		return
	}
	if diffMode {
		h.modbusCSVDiff(w, r, identity, busID, inputs)
		return
	}
	if r.URL.Query().Get("preview") == "true" {
		writeJSON(w, map[string]any{"preview": true, "count": len(inputs), "points": inputs})
		return
	}

	if err := h.createModbusBatch(busID, inputs, identity.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "import_rolled_back")
		return
	}
	writeJSON(w, map[string]int{"imported": len(inputs)})
}

func parseCSVBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "是", "true", "1", "yes":
		return true, nil
	case "否", "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}

func (h *Handler) createModbusBatch(busID string, inputs []ModbusIOInput, origin string) error {
	created := []data.NodeEdge{}
	for _, input := range inputs {
		node, err := newModbusIONode(busID, input)
		if err == nil {
			err = client.SendNode(h.nc, node, origin)
		}
		if err != nil {
			for _, rollback := range created {
				_ = client.DeleteNode(h.nc, rollback.ID, busID, origin)
			}
			return err
		}
		created = append(created, node)
	}
	return nil
}

type modbusDiffChange struct {
	Name   string        `json:"name"`
	Before ModbusIOInput `json:"before"`
	After  ModbusIOInput `json:"after"`
	Fields []string      `json:"fields"`
}

type existingModbusIO struct {
	NodeID string
	Input  ModbusIOInput
}

func (h *Handler) existingModbusIOs(busID string) ([]existingModbusIO, error) {
	children, err := client.GetNodes(h.nc, busID, "all", "", false)
	if err != nil {
		return nil, err
	}
	items := []existingModbusIO{}
	for _, node := range children {
		if node.Type != data.NodeTypeModbusIO {
			continue
		}
		serverID, _ := node.Points.Value(data.PointTypeID, "")
		address, _ := node.Points.Value(data.PointTypeAddress, "")
		scale, _ := node.Points.Value(data.PointTypeScale, "")
		if scale == 0 {
			scale = 1
		}
		offset, _ := node.Points.Value(data.PointTypeOffset, "")
		readOnly, _ := node.Points.Value(data.PointTypeReadOnly, "")
		items = append(items, existingModbusIO{NodeID: node.ID, Input: ModbusIOInput{
			Parent:      busID,
			Description: textPoint(node.Points, data.PointTypeDescription),
			ServerID:    int(serverID), Address: int(address),
			IOType:     textPoint(node.Points, data.PointTypeModbusIOType),
			DataFormat: textPoint(node.Points, data.PointTypeDataFormat),
			ReadOnly:   readOnly != 0, Scale: scale, Offset: offset,
			Units: textPoint(node.Points, data.PointTypeUnits),
		}})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Input.Address != items[j].Input.Address {
			return items[i].Input.Address < items[j].Input.Address
		}
		return items[i].Input.Description < items[j].Input.Description
	})
	return items, nil
}

func modbusDiffFields(before, after ModbusIOInput) []string {
	fields := []string{}
	if before.ServerID != after.ServerID {
		fields = append(fields, "站号")
	}
	if before.Address != after.Address {
		fields = append(fields, "地址")
	}
	if before.IOType != after.IOType {
		fields = append(fields, "类型")
	}
	register := after.IOType == data.PointValueModbusHoldingRegister || after.IOType == data.PointValueModbusInputRegister
	if register && before.DataFormat != after.DataFormat {
		fields = append(fields, "数据格式")
	}
	if before.ReadOnly != after.ReadOnly {
		fields = append(fields, "只读")
	}
	if before.Scale != after.Scale {
		fields = append(fields, "缩放")
	}
	if before.Offset != after.Offset {
		fields = append(fields, "偏移")
	}
	if strings.TrimSpace(before.Units) != strings.TrimSpace(after.Units) {
		fields = append(fields, "单位")
	}
	return fields
}

// modbusCSVDiff compares a validated CSV against the channel's current point
// table. Names identify points; apply=diff creates/updates the differences and
// only deletes missing points when remove=true is passed explicitly.
func (h *Handler) modbusCSVDiff(w http.ResponseWriter, r *http.Request, identity Identity, busID string, inputs []ModbusIOInput) {
	existing, err := h.existingModbusIOs(busID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "diff_unavailable")
		return
	}
	byName := map[string]existingModbusIO{}
	for _, item := range existing {
		if _, duplicate := byName[item.Input.Description]; duplicate {
			writeError(w, http.StatusConflict, "duplicate_existing_point_name: "+item.Input.Description)
			return
		}
		byName[item.Input.Description] = item
	}
	added := []ModbusIOInput{}
	changed := []modbusDiffChange{}
	removedItems := []existingModbusIO{}
	unchanged := 0
	inCSV := map[string]bool{}
	for i := range inputs {
		if inputs[i].Scale == 0 {
			inputs[i].Scale = 1
		}
		input := inputs[i]
		inCSV[input.Description] = true
		current, ok := byName[input.Description]
		if !ok {
			added = append(added, input)
			continue
		}
		fields := modbusDiffFields(current.Input, input)
		if len(fields) == 0 {
			unchanged++
			continue
		}
		changed = append(changed, modbusDiffChange{Name: input.Description, Before: current.Input, After: input, Fields: fields})
	}
	for _, item := range existing {
		if !inCSV[item.Input.Description] {
			removedItems = append(removedItems, item)
		}
	}
	removed := make([]ModbusIOInput, 0, len(removedItems))
	for _, item := range removedItems {
		removed = append(removed, item.Input)
	}
	if r.URL.Query().Get("apply") != "diff" {
		writeJSON(w, map[string]any{
			"diff": true, "added": added, "changed": changed,
			"removed": removed, "unchanged": unchanged,
		})
		return
	}
	if err := h.createModbusBatch(busID, added, identity.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "import_rolled_back")
		return
	}
	for _, change := range changed {
		if err := h.updateModbusIOFields(byName[change.Name].NodeID, busID, change.After, identity.UserID); err != nil {
			writeError(w, http.StatusInternalServerError, "sync_update_failed: "+change.Name)
			return
		}
	}
	removedCount := 0
	if r.URL.Query().Get("remove") == "true" {
		for _, item := range removedItems {
			if err := client.DeleteNode(h.nc, item.NodeID, busID, identity.UserID); err != nil {
				writeError(w, http.StatusInternalServerError, "sync_delete_failed: "+item.Input.Description)
				return
			}
			removedCount++
		}
	}
	writeJSON(w, map[string]any{"applied": true, "added": len(added), "updated": len(changed), "removed": removedCount})
}

func (h *Handler) testModbus(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		ModbusConnectionInput
		ServerID int    `json:"server_id"`
		Address  int    `json:"address"`
		IOType   string `json:"io_type"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	normalizeSerial(&input.ModbusConnectionInput)
	if err := validateModbusConnection(input.ModbusConnectionInput); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.ServerID < 1 || input.ServerID > 247 || input.Address < 0 || input.Address > 65535 {
		writeError(w, http.StatusBadRequest, "invalid_modbus_address")
		return
	}
	value, err := readModbus(input.ModbusConnectionInput, input.ServerID, input.Address, input.IOType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "read_failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"success": true, "value": value})
}

func (h *Handler) updateModbus(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
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
	var input ModbusConnectionInput
	if !decodeJSON(w, r, &input) {
		return
	}
	normalizeSerial(&input)
	if input.Description == "" || validateModbusConnection(input) != nil {
		writeError(w, http.StatusBadRequest, "invalid_modbus_connection")
		return
	}
	if input.PollPeriod == 0 {
		input.PollPeriod = 1000
	}
	if input.Timeout == 0 {
		input.Timeout = 1000
	}
	encoded, err := data.Encode(client.Modbus{
		ID: id, Parent: node.Parent, Description: input.Description,
		ClientServer: data.PointValueClient, Protocol: input.Protocol,
		URI: input.URI, Port: input.Port, Baud: input.Baud,
		PollPeriod: input.PollPeriod, Timeout: input.Timeout, Disabled: input.Disabled,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_modbus_connection")
		return
	}
	encoded.Points = append(encoded.Points, serialPoints(input)...)
	for i := range encoded.Points {
		encoded.Points[i].Origin = identity.UserID
	}
	if err := client.SendNodePoints(h.nc, id, encoded.Points, true); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (h *Handler) updateModbusIO(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canAction(identity, ActionConfigure) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	node, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	if node.Type != data.NodeTypeModbusIO {
		writeError(w, http.StatusBadRequest, "invalid_modbus_point")
		return
	}
	var input ModbusIOInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := validateModbusIO(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.updateModbusIOFields(id, node.Parent, input, identity.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "save_failed")
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

func (h *Handler) updateModbusIOFields(id, parent string, input ModbusIOInput, origin string) error {
	if input.Scale == 0 {
		input.Scale = 1
	}
	encoded, err := data.Encode(client.ModbusIo{
		ID: id, Parent: parent, Description: input.Description,
		ServerID: input.ServerID, Address: input.Address,
		ModbusIOType: input.IOType, DataFormat: input.DataFormat,
		ReadOnly: input.ReadOnly, Scale: input.Scale, Offset: input.Offset, Disabled: input.Disabled,
	})
	if err != nil {
		return err
	}
	encoded.Points = append(encoded.Points, data.NewPointString(data.PointTypeUnits, "", input.Units))
	for i := range encoded.Points {
		encoded.Points[i].Origin = origin
	}
	return client.SendNodePoints(h.nc, id, encoded.Points, true)
}

func (h *Handler) exportModbusCSV(w http.ResponseWriter, identity Identity, busID string) {
	if !canAction(identity, ActionExport) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	bus, ok := h.node(w, identity, busID)
	if !ok {
		return
	}
	if bus.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	children, err := client.GetNodes(h.nc, busID, "all", "", false)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "export_failed")
		return
	}
	sort.Slice(children, func(i, j int) bool {
		left, _ := children[i].Points.Value(data.PointTypeAddress, "")
		right, _ := children[j].Points.Value(data.PointTypeAddress, "")
		if left != right {
			return left < right
		}
		return children[i].ID < children[j].ID
	})
	var buffer bytes.Buffer
	buffer.WriteString("\xef\xbb\xbf")
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"名称", "站号", "地址", "类型", "数据格式", "只读", "缩放", "偏移", "单位"})
	typeNames := map[string]string{
		data.PointValueModbusHoldingRegister: "保持寄存器", data.PointValueModbusInputRegister: "输入寄存器",
		data.PointValueModbusCoil: "线圈", data.PointValueModbusDiscreteInput: "离散输入",
	}
	for _, node := range children {
		name, _ := node.Points.Text(data.PointTypeDescription, "")
		serverID, _ := node.Points.Value(data.PointTypeID, "")
		address, _ := node.Points.Value(data.PointTypeAddress, "")
		ioType, _ := node.Points.Text(data.PointTypeModbusIOType, "")
		format, _ := node.Points.Text(data.PointTypeDataFormat, "")
		readOnly, _ := node.Points.Value(data.PointTypeReadOnly, "")
		scale, _ := node.Points.Value(data.PointTypeScale, "")
		offset, _ := node.Points.Value(data.PointTypeOffset, "")
		units, _ := node.Points.Text(data.PointTypeUnits, "")
		_ = writer.Write([]string{
			name, strconv.Itoa(int(serverID)), strconv.Itoa(int(address)), typeNames[ioType], format,
			map[bool]string{true: "是", false: "否"}[readOnly != 0],
			strconv.FormatFloat(scale, 'g', -1, 64), strconv.FormatFloat(offset, 'g', -1, 64), units,
		})
	}
	writer.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="modbus-points.csv"`)
	_, _ = w.Write(buffer.Bytes())
}

func validateModbusConnection(input ModbusConnectionInput) error {
	switch input.Protocol {
	case data.PointValueTCP:
		host, port, err := net.SplitHostPort(input.URI)
		if err != nil || host == "" {
			return fmt.Errorf("TCP address must be host:port")
		}
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return fmt.Errorf("invalid TCP port")
		}
	case data.PointValueRTU:
		baud, err := strconv.Atoi(input.Baud)
		if input.Port == "" || err != nil || baud < 1 || baud > 4_000_000 {
			return fmt.Errorf("invalid RTU port or baud")
		}
		if input.DataBits != 0 && (input.DataBits < 5 || input.DataBits > 8) {
			return fmt.Errorf("invalid data bits")
		}
		switch strings.ToUpper(input.Parity) {
		case "", "N", "E", "O":
		default:
			return fmt.Errorf("invalid parity")
		}
		if input.StopBits != 0 && input.StopBits != 1 && input.StopBits != 2 {
			return fmt.Errorf("invalid stop bits")
		}
	default:
		return fmt.Errorf("protocol must be TCP or RTU")
	}
	if input.PollPeriod < 0 || input.PollPeriod > 60_000 || input.Timeout < 0 || input.Timeout > 10_000 {
		return fmt.Errorf("invalid timing")
	}
	return nil
}

func validateModbusIO(input ModbusIOInput) error {
	if input.Description == "" || input.ServerID < 1 || input.ServerID > 247 || input.Address < 0 || input.Address > 65535 {
		return fmt.Errorf("invalid_modbus_address")
	}
	if !finite(input.Scale) || !finite(input.Offset) {
		return fmt.Errorf("invalid_scaling")
	}
	switch input.IOType {
	case data.PointValueModbusDiscreteInput, data.PointValueModbusInputRegister:
		if !input.ReadOnly {
			return fmt.Errorf("input_points_must_be_read_only")
		}
	case data.PointValueModbusCoil, data.PointValueModbusHoldingRegister:
	default:
		return fmt.Errorf("invalid_modbus_io_type")
	}
	if input.IOType == data.PointValueModbusCoil || input.IOType == data.PointValueModbusDiscreteInput {
		return nil
	}
	switch input.DataFormat {
	case data.PointValueUINT16, data.PointValueINT16:
	case data.PointValueUINT32, data.PointValueINT32, data.PointValueFLOAT32:
		if input.Address > 65534 {
			return fmt.Errorf("register_pair_out_of_range")
		}
	default:
		return fmt.Errorf("invalid_modbus_data_format")
	}
	return nil
}

func openModbusClient(connection ModbusConnectionInput) (*siotmodbus.Client, func() error, error) {
	if connection.Protocol == data.PointValueTCP {
		socket, err := net.DialTimeout("tcp", connection.URI, 3*time.Second)
		if err != nil {
			return nil, nil, err
		}
		return siotmodbus.NewClient(siotmodbus.NewTCP(socket, 2*time.Second, siotmodbus.TransportClient), 0), socket.Close, nil
	}
	baud, _ := strconv.Atoi(connection.Baud)
	mode := &serial.Mode{BaudRate: baud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}
	if connection.DataBits >= 5 && connection.DataBits <= 8 {
		mode.DataBits = connection.DataBits
	}
	switch strings.ToUpper(connection.Parity) {
	case "E":
		mode.Parity = serial.EvenParity
	case "O":
		mode.Parity = serial.OddParity
	}
	if connection.StopBits == 2 {
		mode.StopBits = serial.TwoStopBits
	}
	port, err := serial.Open(connection.Port, mode)
	if err != nil {
		return nil, nil, err
	}
	reader := respreader.NewReadWriteCloser(port, 2*time.Second, 20*time.Millisecond)
	return siotmodbus.NewClient(siotmodbus.NewRTU(reader), 0), port.Close, nil
}

func readModbus(connection ModbusConnectionInput, serverID, address int, ioType string) (any, error) {
	clientConnection, close, err := openModbusClient(connection)
	if err != nil {
		return nil, err
	}
	defer close()
	switch ioType {
	case data.PointValueModbusCoil:
		return clientConnection.ReadCoils(byte(serverID), uint16(address), 1)
	case data.PointValueModbusDiscreteInput:
		return clientConnection.ReadDiscreteInputs(byte(serverID), uint16(address), 1)
	case data.PointValueModbusHoldingRegister:
		return clientConnection.ReadHoldingRegs(byte(serverID), uint16(address), 1)
	case data.PointValueModbusInputRegister:
		return clientConnection.ReadInputRegs(byte(serverID), uint16(address), 1)
	default:
		return nil, fmt.Errorf("unsupported read type")
	}
}
