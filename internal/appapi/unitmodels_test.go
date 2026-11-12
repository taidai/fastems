package appapi

import (
	"testing"

	"fastems/internal/ems/profile"
)

func TestUnitModelResponseMapsCommands(t *testing.T) {
	registry := profile.Default()
	model := unitModelConfig{
		Name: "PCS", ProfileID: "enjo-pcs",
		PointMap: map[string]string{"开机/关机指令": "point-1"},
	}
	points := []unitModelPointView{{ID: "point-1", Name: "开机/关机指令", ServerID: 1}}
	view := unitModelResponse("channel-1", 1, model, true, points, registry)
	if view.Model == nil || view.Model.ProfileID != "enjo-pcs" {
		t.Fatalf("model = %+v", view.Model)
	}
	if len(view.Commands) != 2 {
		t.Fatalf("commands = %+v", view.Commands)
	}
	for _, command := range view.Commands {
		if !command.Available || command.PointID != "point-1" {
			t.Fatalf("command = %+v", command)
		}
	}
	if len(view.Missing) != 0 {
		t.Fatalf("missing = %v", view.Missing)
	}
}

func TestUnitModelResponseRejectsReadOnlyCommandPoint(t *testing.T) {
	registry := profile.Default()
	model := unitModelConfig{ProfileID: "enjo-pcs", PointMap: map[string]string{"开机/关机指令": "point-1"}}
	points := []unitModelPointView{{ID: "point-1", Name: "开机/关机指令", ServerID: 1, ReadOnly: true}}
	view := unitModelResponse("channel-1", 1, model, true, points, registry)
	if len(view.Commands) == 0 || view.Commands[0].Available {
		t.Fatalf("read-only command point should be unavailable: %+v", view.Commands)
	}
	if view.Commands[0].Reason != "点位为只读" {
		t.Fatalf("reason = %q", view.Commands[0].Reason)
	}
}

func TestUnitModelResponseReportsMissingCommandPoint(t *testing.T) {
	registry := profile.Default()
	model := unitModelConfig{ProfileID: "enjo-pcs", PointMap: map[string]string{}}
	view := unitModelResponse("channel-1", 1, model, true, nil, registry)
	if len(view.Missing) != 1 || view.Missing[0] != "开机/关机指令" {
		t.Fatalf("missing = %v", view.Missing)
	}
	if len(view.Commands) != 2 || view.Commands[0].Available {
		t.Fatalf("commands = %+v", view.Commands)
	}
}

func TestUnitModelResponseExposesCommandParameters(t *testing.T) {
	registry := profile.Default()
	model := unitModelConfig{ProfileID: "huawei-inverter", PointMap: map[string]string{"有功功率百分比控制": "percent-point"}}
	points := []unitModelPointView{{ID: "percent-point", Name: "有功功率百分比控制", ServerID: 1}}
	view := unitModelResponse("channel-1", 1, model, true, points, registry)
	var command *unitModelCommand
	for i := range view.Commands {
		if view.Commands[i].ID == "set-active-power-percent" {
			command = &view.Commands[i]
			break
		}
	}
	if command == nil {
		t.Fatalf("command missing: %+v", view.Commands)
	}
	if !command.Available || len(command.Parameters) != 1 {
		t.Fatalf("command = %+v", command)
	}
	parameter := command.Parameters[0]
	if parameter.Name != "percent" || !parameter.Available || !parameter.HasRange || parameter.Max != 100 {
		t.Fatalf("parameter = %+v", parameter)
	}
}

func TestUnitModelResponseReportsMissingParameterPoint(t *testing.T) {
	registry := profile.Default()
	model := unitModelConfig{ProfileID: "huawei-inverter", PointMap: map[string]string{}}
	view := unitModelResponse("channel-1", 1, model, true, nil, registry)
	var command *unitModelCommand
	for i := range view.Commands {
		if view.Commands[i].ID == "set-active-power-percent" {
			command = &view.Commands[i]
			break
		}
	}
	if command == nil || command.Available || len(command.Parameters) != 1 || command.Parameters[0].Available {
		t.Fatalf("command = %+v", command)
	}
	found := false
	for _, missing := range view.Missing {
		if missing == "有功功率百分比控制" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing = %v", view.Missing)
	}
}
