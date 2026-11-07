package profile

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"easyems/internal/ems/channel"
)

func TestDefaultProfilesAreSelectable(t *testing.T) {
	registry := Default()
	list := registry.List()
	if len(list) == 0 {
		t.Fatal("default registry is empty")
	}
	profile, ok := registry.Get("modbus-generic")
	if !ok || profile.Vendor != "通用" {
		t.Fatalf("generic profile = %+v ok=%v", profile, ok)
	}
	if _, ok := registry.Get("missing"); ok {
		t.Fatal("unknown profile returned")
	}
}

func TestMatchSkipsProfilesWithoutIdentity(t *testing.T) {
	registry := Default()
	if _, ok := registry.Match(func(Identity) ([]uint16, error) {
		t.Fatal("reader must not be called without identity")
		return nil, nil
	}); ok {
		t.Fatal("generic profile must not auto-match")
	}
}

func TestMatchComparesIdentityWords(t *testing.T) {
	registry := &Registry{profiles: []Profile{
		{ID: "vendor-a", Identity: &Identity{Function: channel.HoldingRegister, Address: 100, Count: 2, Expect: []uint16{0x1234, 0x0001}}},
	}}
	matched, ok := registry.Match(func(identity Identity) ([]uint16, error) {
		if identity.Address != 100 || identity.Count != 2 {
			t.Fatalf("unexpected identity %+v", identity)
		}
		return []uint16{0x1234, 0x0001}, nil
	})
	if !ok || matched.ID != "vendor-a" {
		t.Fatalf("matched = %+v ok=%v", matched, ok)
	}
	if _, ok := registry.Match(func(Identity) ([]uint16, error) {
		return []uint16{0x1234, 0x0002}, nil
	}); ok {
		t.Fatal("wrong identity words matched")
	}
}

func TestCatalogProfilesCarryPointsAndVerification(t *testing.T) {
	registry := Default()
	list := registry.List()
	if len(list) < 7 {
		t.Fatalf("expected generic + at least 6 vendor profiles, got %d", len(list))
	}
	seen := map[string]bool{}
	for _, profile := range list {
		if seen[profile.ID] {
			t.Fatalf("duplicate profile id %s", profile.ID)
		}
		seen[profile.ID] = true
		if profile.ID == "modbus-generic" {
			continue
		}
		if len(profile.PointSpecs) == 0 {
			t.Fatalf("profile %s has no point specs", profile.ID)
		}
		if profile.Points != len(profile.PointSpecs) || profile.Commands != len(profile.CommandSpecs) {
			t.Fatalf("profile %s counts do not match specs", profile.ID)
		}
		if profile.Verified {
			t.Fatalf("profile %s must stay unverified until on-site sign-off", profile.ID)
		}
		if profile.Verification == "" || profile.Source == "" || profile.IdentityHint == "" {
			t.Fatalf("profile %s is missing provenance", profile.ID)
		}
		for _, point := range profile.PointSpecs {
			if point.Name == "" || point.Address == 0 || point.Format == "" {
				t.Fatalf("profile %s has an incomplete point: %+v", profile.ID, point)
			}
			if point.Function != channel.HoldingRegister && point.Function != channel.InputRegister {
				t.Fatalf("profile %s point %s has unsupported function %s", profile.ID, point.Name, point.Function)
			}
		}
		byName := map[string]PointSpec{}
		for _, point := range profile.PointSpecs {
			byName[point.Name] = point
		}
		for _, command := range profile.CommandSpecs {
			if command.Point == "" && len(command.Parameters) == 0 {
				t.Fatalf("profile %s command %s has no write target", profile.ID, command.ID)
			}
			if command.Point != "" {
				point, ok := byName[command.Point]
				if !ok || point.ReadOnly {
					t.Fatalf("profile %s command %s must target a writable point", profile.ID, command.ID)
				}
			}
			for _, parameter := range command.Parameters {
				point, ok := byName[parameter.Point]
				if !ok || point.ReadOnly {
					t.Fatalf("profile %s command %s parameter %s must target a writable point", profile.ID, command.ID, parameter.Name)
				}
				if parameter.Name == "" || parameter.Label == "" {
					t.Fatalf("profile %s command %s has an incomplete parameter", profile.ID, command.ID)
				}
			}
		}
	}
}

func TestProfileCSVMatchesPointTableFormat(t *testing.T) {
	registry := Default()
	profile, ok := registry.Get("enjo-pcs")
	if !ok {
		t.Fatal("enjo-pcs profile missing")
	}
	data := profile.CSV(42)
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatalf("CSV parse: %v", err)
	}
	if len(records) != len(profile.PointSpecs)+1 {
		t.Fatalf("CSV rows = %d, points = %d", len(records), len(profile.PointSpecs))
	}
	header := strings.Join(records[0], ",")
	if header != "名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位" {
		t.Fatalf("unexpected header %q", header)
	}
	if records[1][1] != "42" {
		t.Fatalf("station id = %q", records[1][1])
	}
	if records[1][3] != "保持寄存器" {
		t.Fatalf("function = %q", records[1][3])
	}
	if records[1][5] != "否" {
		t.Fatalf("first point should be writable, readOnly = %q", records[1][5])
	}
}

func wordsFor(text string) []uint16 {
	if len(text)%2 != 0 {
		text += " "
	}
	words := make([]uint16, 0, len(text)/2)
	for i := 0; i < len(text); i += 2 {
		words = append(words, uint16(text[i])<<8|uint16(text[i+1]))
	}
	return words
}

func TestIdentifyRanksStrongAndWeakCandidates(t *testing.T) {
	registry := &Registry{profiles: []Profile{
		{ID: "yd", Probes: []Identity{{Function: channel.HoldingRegister, Address: 43904, Count: 8, TextContains: []string{"YD2037"}, Weight: 3, Label: "model"}}},
		{ID: "weak", Probes: []Identity{{Function: channel.HoldingRegister, Address: 100, Count: 2, Weight: 1, Label: "feature"}}},
		{ID: "missing", Probes: []Identity{{Function: channel.InputRegister, Address: 200, Count: 2, Weight: 1}}},
	}}
	candidates := registry.Identify(func(identity Identity) ([]uint16, error) {
		switch identity.Address {
		case 43904:
			return wordsFor("YD2037Y-F      "), nil
		case 100:
			return []uint16{1, 2}, nil
		default:
			return nil, errors.New("no response")
		}
	})
	if len(candidates) != 2 {
		t.Fatalf("expected two candidates, got %+v", candidates)
	}
	if candidates[0].Profile.ID != "yd" || !candidates[0].Strong {
		t.Fatalf("strong text candidate should rank first: %+v", candidates[0])
	}
	if candidates[1].Profile.ID != "weak" || candidates[1].Strong {
		t.Fatalf("structural candidate should stay weak: %+v", candidates[1])
	}
}
