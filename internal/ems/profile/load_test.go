package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogDeclaresRoles(t *testing.T) {
	for _, entry := range catalog() {
		for _, spec := range entry.PointSpecs {
			if spec.Role == "" || !ValidRole(spec.Role) {
				t.Fatalf("profile %s point %q has no valid role (%q)", entry.ID, spec.Name, spec.Role)
			}
		}
	}
}

func writeProfile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCustomProfile(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "pump.json", `{
		"id": "acme-pump",
		"vendor": "ACME",
		"model": "泵控制器",
		"point_specs": [
			{"name": "转速", "address": 10, "unit": "rpm", "read_only": true, "role": "telemetry"},
			{"name": "启停", "address": 1, "role": "setpoint"}
		],
		"command_specs": [
			{"id": "start", "name": "启动", "point": "启停", "value": 1}
		]
	}`)
	registry := Load(dir)
	if len(registry.LoadErrors) != 0 {
		t.Fatalf("unexpected load errors: %v", registry.LoadErrors)
	}
	loaded, ok := registry.Get("acme-pump")
	if !ok {
		t.Fatal("custom profile missing")
	}
	if !loaded.Custom {
		t.Fatal("custom profile must be marked custom")
	}
	if loaded.Points != 2 || loaded.Commands != 1 {
		t.Fatalf("counts not derived: %d/%d", loaded.Points, loaded.Commands)
	}
	spec := loaded.PointSpecs[0]
	if spec.Function != "holdingRegister" || spec.Format != "uint16" || spec.Scale != 1 {
		t.Fatalf("defaults not applied: %+v", spec)
	}
}

func TestLoadOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "fix.json", `{
		"id": "jiwa-mppt",
		"vendor": "吉瓦时代",
		"model": "MPPT（现场修正）",
		"point_specs": [{"name": "输入侧总功率", "address": 45217, "format": "int16", "scale": 0.1, "unit": "kW", "read_only": true, "role": "telemetry"}]
	}`)
	registry := Load(dir)
	loaded, ok := registry.Get("jiwa-mppt")
	if !ok || !loaded.Custom || loaded.Model != "MPPT（现场修正）" {
		t.Fatalf("builtin not overridden: %+v", loaded)
	}
	builtinCount := len(Default().List())
	if len(registry.List()) != builtinCount {
		t.Fatalf("override must not grow the registry: %d != %d", len(registry.List()), builtinCount)
	}
}

func TestLoadReportsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "broken.json", `{not json`)
	writeProfile(t, dir, "badrole.json", `{"id":"x","vendor":"v","model":"m","point_specs":[{"name":"a","role":"wat"}]}`)
	writeProfile(t, dir, "badcmd.json", `{"id":"y","vendor":"v","model":"m","command_specs":[{"id":"go","name":"走","point":"不存在"}]}`)
	registry := Load(dir)
	if len(registry.LoadErrors) != 3 {
		t.Fatalf("expected 3 load errors, got %v", registry.LoadErrors)
	}
	for _, id := range []string{"x", "y"} {
		if _, ok := registry.Get(id); ok {
			t.Fatalf("invalid profile %s must not load", id)
		}
	}
	if !strings.Contains(registry.LoadErrors[1], "role") {
		t.Fatalf("role error not explained: %v", registry.LoadErrors[1])
	}
}

func TestLoadMissingDirIsClean(t *testing.T) {
	registry := Load(filepath.Join(t.TempDir(), "absent"))
	if len(registry.LoadErrors) != 0 {
		t.Fatalf("missing dir is not an error: %v", registry.LoadErrors)
	}
	if len(registry.List()) != len(Default().List()) {
		t.Fatal("builtin profiles missing")
	}
}
