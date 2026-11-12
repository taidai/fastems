package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fastiot/internal/ems/channel"
)

// Load returns the built-in registry merged with operator-maintained profiles
// from dir (*.json, one profile per file). A file profile that reuses a
// built-in ID replaces that entry, so a site can correct a vendor table
// without recompiling. File profiles never carry identity probes: they are
// manual choices only. Invalid files are skipped and reported in LoadErrors.
func Load(dir string) *Registry {
	registry := Default()
	if dir == "" {
		return registry
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			registry.LoadErrors = append(registry.LoadErrors, dir+"："+err.Error())
		}
		return registry
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	index := map[string]int{}
	for i, existing := range registry.profiles {
		index[existing.ID] = i
	}
	seen := map[string]string{}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			registry.LoadErrors = append(registry.LoadErrors, name+"：读取失败："+err.Error())
			continue
		}
		var loaded Profile
		if err := json.Unmarshal(raw, &loaded); err != nil {
			registry.LoadErrors = append(registry.LoadErrors, name+"：不是有效的 JSON Profile："+err.Error())
			continue
		}
		if err := normalize(&loaded); err != nil {
			registry.LoadErrors = append(registry.LoadErrors, name+"："+err.Error())
			continue
		}
		if previous, dup := seen[loaded.ID]; dup {
			registry.LoadErrors = append(registry.LoadErrors, name+"：ID "+loaded.ID+" 与 "+previous+" 重复")
			continue
		}
		seen[loaded.ID] = name
		if at, exists := index[loaded.ID]; exists {
			registry.profiles[at] = loaded
		} else {
			index[loaded.ID] = len(registry.profiles)
			registry.profiles = append(registry.profiles, loaded)
		}
	}
	return registry
}

// normalize validates an operator profile and fills derived fields.
func normalize(p *Profile) error {
	p.ID = strings.TrimSpace(p.ID)
	if p.ID == "" {
		return fmt.Errorf("缺少 id")
	}
	if strings.ContainsAny(p.ID, " /\\") {
		return fmt.Errorf("id %q 不能包含空格或斜杠", p.ID)
	}
	if strings.TrimSpace(p.Vendor) == "" || strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("vendor 和 model 不能为空")
	}
	if p.Protocol == "" {
		p.Protocol = "modbus"
	}
	if p.Protocol != "modbus" {
		return fmt.Errorf("protocol %q 暂不支持，只支持 modbus", p.Protocol)
	}
	pointNames := map[string]bool{}
	for i := range p.PointSpecs {
		spec := &p.PointSpecs[i]
		spec.Name = strings.TrimSpace(spec.Name)
		if spec.Name == "" {
			return fmt.Errorf("第 %d 个点位缺少 name", i+1)
		}
		if pointNames[spec.Name] {
			return fmt.Errorf("点位名称 %q 重复", spec.Name)
		}
		pointNames[spec.Name] = true
		switch spec.Function {
		case "":
			spec.Function = channel.HoldingRegister
		case channel.Coil, channel.DiscreteInput, channel.HoldingRegister, channel.InputRegister:
		default:
			return fmt.Errorf("点位 %q 的 function %q 无效（coil/discreteInput/holdingRegister/inputRegister）", spec.Name, spec.Function)
		}
		switch spec.Format {
		case "":
			spec.Format = "uint16"
		case "uint16", "int16", "uint32", "int32", "float32":
		default:
			return fmt.Errorf("点位 %q 的 format %q 无效（uint16/int16/uint32/int32/float32）", spec.Name, spec.Format)
		}
		if spec.Scale == 0 {
			spec.Scale = 1
		}
		if !ValidRole(spec.Role) {
			return fmt.Errorf("点位 %q 的 role %q 无效（telemetry/status/energy/setpoint/protection）", spec.Name, spec.Role)
		}
	}
	commandIDs := map[string]bool{}
	for i := range p.CommandSpecs {
		spec := &p.CommandSpecs[i]
		spec.ID = strings.TrimSpace(spec.ID)
		if spec.ID == "" || strings.TrimSpace(spec.Name) == "" {
			return fmt.Errorf("第 %d 个命令缺少 id 或 name", i+1)
		}
		if commandIDs[spec.ID] {
			return fmt.Errorf("命令 ID %q 重复", spec.ID)
		}
		commandIDs[spec.ID] = true
		if spec.Point != "" && !pointNames[spec.Point] {
			return fmt.Errorf("命令 %q 引用了未声明的点位 %q", spec.ID, spec.Point)
		}
		for _, parameter := range spec.Parameters {
			if parameter.Name == "" || parameter.Point == "" {
				return fmt.Errorf("命令 %q 的参数缺少 name 或 point", spec.ID)
			}
			if !pointNames[parameter.Point] {
				return fmt.Errorf("命令 %q 的参数 %q 引用了未声明的点位 %q", spec.ID, parameter.Name, parameter.Point)
			}
			switch parameter.Kind {
			case "", "number", "bool":
			default:
				return fmt.Errorf("命令 %q 的参数 %q 的 kind %q 无效（number/bool）", spec.ID, parameter.Name, parameter.Kind)
			}
			if parameter.HasRange && parameter.Min > parameter.Max {
				return fmt.Errorf("命令 %q 的参数 %q 范围无效（min > max）", spec.ID, parameter.Name)
			}
		}
	}
	p.Points = len(p.PointSpecs)
	p.Commands = len(p.CommandSpecs)
	p.Custom = true
	p.Identity = nil
	p.Probes = nil
	return nil
}
