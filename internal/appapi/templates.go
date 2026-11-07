package appapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/simpleiot/simpleiot/client"
	"github.com/simpleiot/simpleiot/data"
)

type DeviceTemplate struct {
	ID          string `node:"id"`
	Parent      string `node:"parent"`
	Description string `point:"description"`
	Definition  string `point:"definition"`
}

type TemplateDefinition struct {
	Points []ModbusIOInput `json:"points"`
}

func (h *Handler) saveTemplate(w http.ResponseWriter, r *http.Request, identity Identity) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	var input struct {
		Parent      string `json:"parent"`
		Description string `json:"description"`
		SourceBusID string `json:"source_bus_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Description) == "" || !h.allows(identity, input.Parent) {
		writeError(w, http.StatusBadRequest, "invalid_template")
		return
	}
	source, ok := h.node(w, identity, input.SourceBusID)
	if !ok {
		return
	}
	if source.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_template_source")
		return
	}
	children, err := client.GetNodes(h.nc, source.ID, "all", "", false)
	if err != nil || len(children) == 0 {
		writeError(w, http.StatusBadRequest, "template_has_no_points")
		return
	}
	definition := TemplateDefinition{Points: []ModbusIOInput{}}
	for _, child := range children {
		if child.Type != data.NodeTypeModbusIO {
			continue
		}
		name, _ := child.Points.Text(data.PointTypeDescription, "")
		serverID, _ := child.Points.Value(data.PointTypeID, "")
		address, _ := child.Points.Value(data.PointTypeAddress, "")
		ioType, _ := child.Points.Text(data.PointTypeModbusIOType, "")
		format, _ := child.Points.Text(data.PointTypeDataFormat, "")
		readOnly, _ := child.Points.Value(data.PointTypeReadOnly, "")
		scale, _ := child.Points.Value(data.PointTypeScale, "")
		offset, _ := child.Points.Value(data.PointTypeOffset, "")
		units, _ := child.Points.Text(data.PointTypeUnits, "")
		definition.Points = append(definition.Points, ModbusIOInput{
			Description: name, ServerID: int(serverID), Address: int(address),
			IOType: ioType, DataFormat: format, ReadOnly: readOnly != 0,
			Scale: scale, Offset: offset, Units: units,
		})
	}
	if len(definition.Points) == 0 {
		writeError(w, http.StatusBadRequest, "template_has_no_points")
		return
	}
	encoded, _ := json.Marshal(definition)
	id := data.NewID()
	if err := client.SendNodeType(h.nc, DeviceTemplate{
		ID: id, Parent: input.Parent, Description: strings.TrimSpace(input.Description), Definition: string(encoded),
	}, identity.UserID); err != nil {
		writeError(w, http.StatusBadRequest, "create_failed")
		return
	}
	writeJSON(w, map[string]string{"id": id})
}

func (h *Handler) applyTemplate(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if !canConfigure(identity) {
		writeError(w, http.StatusForbidden, "admin_required")
		return
	}
	template, ok := h.node(w, identity, id)
	if !ok {
		return
	}
	if template.Type != "deviceTemplate" {
		writeError(w, http.StatusBadRequest, "invalid_template")
		return
	}
	var input struct {
		BusID string `json:"bus_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	bus, ok := h.node(w, identity, input.BusID)
	if !ok {
		return
	}
	if bus.Type != data.NodeTypeModbus {
		writeError(w, http.StatusBadRequest, "invalid_modbus_bus")
		return
	}
	definitionText, _ := template.Points.Text("definition", "")
	var definition TemplateDefinition
	if json.Unmarshal([]byte(definitionText), &definition) != nil || len(definition.Points) == 0 {
		writeError(w, http.StatusConflict, "invalid_template_definition")
		return
	}
	children, _ := client.GetNodes(h.nc, bus.ID, "all", "", false)
	existing := map[string]bool{}
	for _, child := range children {
		name, _ := child.Points.Text(data.PointTypeDescription, "")
		existing[name] = true
	}
	for i := range definition.Points {
		definition.Points[i].Parent = bus.ID
		if err := validateModbusIO(definition.Points[i]); err != nil || existing[definition.Points[i].Description] {
			writeError(w, http.StatusConflict, "template_conflict: "+definition.Points[i].Description)
			return
		}
		existing[definition.Points[i].Description] = true
	}
	if err := h.createModbusBatch(bus.ID, definition.Points, identity.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "apply_rolled_back")
		return
	}
	writeJSON(w, map[string]int{"created": len(definition.Points)})
}
