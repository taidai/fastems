// Package profile holds device profiles used to identify a slave and propose
// points and commands. Profiles are added from verified protocol documents; an
// empty identity block means the profile can only be chosen manually.
package profile

import (
	"bytes"
	"encoding/csv"
	"sort"
	"strconv"
	"strings"

	"easyems/internal/ems/channel"
)

// Identity describes one register probe used to recognise a device. A probe
// may match exact words, an ASCII model/version string, or merely prove that a
// characteristic register is readable (candidate evidence, not a match).
type Identity struct {
	Function     channel.Function
	Address      uint16
	Count        uint16
	Expect       []uint16
	TextContains []string
	Weight       int
	Label        string
}

// PointSpec is one point proposed by a vendor profile. It uses the same
// function/address/format vocabulary as the Modbus point-table CSV.
type PointSpec struct {
	Name     string           `json:"name"`
	Function channel.Function `json:"function"`
	Address  uint16           `json:"address"`
	Format   string           `json:"format"`
	Scale    float64          `json:"scale"`
	Offset   float64          `json:"offset"`
	Unit     string           `json:"unit"`
	ReadOnly bool             `json:"read_only"`
	Note     string           `json:"note,omitempty"`
}

// CommandParameter is one operator-supplied value of a named command.
type CommandParameter struct {
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	Point    string  `json:"point"`
	Unit     string  `json:"unit,omitempty"`
	Kind     string  `json:"kind,omitempty"` // number | bool
	HasRange bool    `json:"has_range,omitempty"`
	Min      float64 `json:"min,omitempty"`
	Max      float64 `json:"max,omitempty"`
	Default  float64 `json:"default,omitempty"`
}

// CommandSpec is one writable action proposed by a vendor profile. Value is
// the default test value for a fixed command; Parameters add operator input.
type CommandSpec struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Point      string             `json:"point,omitempty"`
	Value      float64            `json:"value"`
	Parameters []CommandParameter `json:"parameters,omitempty"`
	Note       string             `json:"note,omitempty"`
}

// Profile is one vendor/model description. Points/Commands are counts kept for
// the existing API; PointSpecs/CommandSpecs carry the curated key points.
type Profile struct {
	ID           string        `json:"id"`
	Vendor       string        `json:"vendor"`
	Model        string        `json:"model"`
	Protocol     string        `json:"protocol"`
	Points       int           `json:"points"`
	Commands     int           `json:"commands"`
	Verified     bool          `json:"verified"`
	Verification string        `json:"verification,omitempty"`
	Source       string        `json:"source,omitempty"`
	IdentityHint string        `json:"identity_hint,omitempty"`
	Notes        []string      `json:"notes,omitempty"`
	PointSpecs   []PointSpec   `json:"point_specs,omitempty"`
	CommandSpecs []CommandSpec `json:"command_specs,omitempty"`
	Identity     *Identity     `json:"-"`
	Probes       []Identity    `json:"-"`
}

// Registry is a read-only set of profiles.
type Registry struct {
	profiles []Profile
}

// Default returns the built-in profiles. The generic entry is a manual choice,
// not a claim of vendor support; it has no identity block and never auto-matches.
func Default() *Registry {
	generic := Profile{ID: "modbus-generic", Vendor: "通用", Model: "Modbus 设备", Protocol: "modbus", Points: 0, Commands: 0}
	return &Registry{profiles: append([]Profile{generic}, catalog()...)}
}

// List returns all profiles.
func (r *Registry) List() []Profile {
	return append([]Profile(nil), r.profiles...)
}

// Get returns one profile by id.
func (r *Registry) Get(id string) (Profile, bool) {
	for _, profile := range r.profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}

// Candidate is one ranked identification suggestion. Strong candidates matched
// an exact word or model string; weak candidates only proved a characteristic
// register is readable and still require an operator decision.
type Candidate struct {
	Profile  Profile
	Score    int
	Evidence string
	Strong   bool
}

// Identify probes every profile and returns ranked candidates. Reads are
// cached so several profiles can share one register block.
func (r *Registry) Identify(read func(Identity) ([]uint16, error)) []Candidate {
	type cacheEntry struct {
		words []uint16
		err   error
	}
	cache := map[string]cacheEntry{}
	readCached := func(identity Identity) ([]uint16, error) {
		key := string(identity.Function) + ":" + strconv.Itoa(int(identity.Address)) + ":" + strconv.Itoa(int(identity.Count))
		if entry, ok := cache[key]; ok {
			return entry.words, entry.err
		}
		words, err := read(identity)
		cache[key] = cacheEntry{words, err}
		return words, err
	}
	var candidates []Candidate
	for _, profile := range r.profiles {
		probes := profile.Probes
		if profile.Identity != nil {
			probes = append([]Identity{*profile.Identity}, probes...)
		}
		if len(probes) == 0 {
			continue
		}
		score := 0
		strong := false
		evidence := []string{}
		for _, probe := range probes {
			words, err := readCached(probe)
			if err != nil || len(words) < int(probe.Count) {
				continue
			}
			weight := probe.Weight
			if weight <= 0 {
				weight = 1
			}
			label := probe.Label
			if label == "" {
				label = "寄存器可读"
			}
			if len(probe.Expect) > 0 {
				matched := true
				for i, expected := range probe.Expect {
					if i >= len(words) || words[i] != expected {
						matched = false
						break
					}
				}
				if matched {
					score += weight * 3
					strong = true
					evidence = append(evidence, label+"：特征字匹配")
				}
				continue
			}
			if len(probe.TextContains) > 0 {
				text := decodeWords(words)
				hit := ""
				for _, needle := range probe.TextContains {
					if strings.Contains(strings.ToUpper(text), strings.ToUpper(needle)) {
						hit = needle
						break
					}
				}
				if hit != "" {
					score += weight * 3
					strong = true
					evidence = append(evidence, label+"：读到 \""+strings.TrimSpace(text)+"\"")
				} else {
					score += weight
					evidence = append(evidence, label+"：可读，未识别到型号文本")
				}
				continue
			}
			score += weight
			evidence = append(evidence, label+"：特征寄存器可读")
		}
		if score > 0 {
			candidates = append(candidates, Candidate{Profile: profile, Score: score, Evidence: strings.Join(evidence, "；"), Strong: strong})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Strong != candidates[j].Strong {
			return candidates[i].Strong
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	return candidates
}

func decodeWords(words []uint16) string {
	raw := make([]byte, 0, len(words)*2)
	for _, word := range words {
		raw = append(raw, byte(word>>8), byte(word))
	}
	return strings.TrimFunc(string(raw), func(r rune) bool { return r == 0 || r == ' ' })
}

// Match compares read identity words against the profiles that define one.
// read returns the words for an identity block.
func (r *Registry) Match(read func(Identity) ([]uint16, error)) (Profile, bool) {
	for _, profile := range r.profiles {
		if profile.Identity == nil || len(profile.Identity.Expect) == 0 {
			continue
		}
		words, err := read(*profile.Identity)
		if err != nil || len(words) < len(profile.Identity.Expect) {
			continue
		}
		match := true
		for i, expected := range profile.Identity.Expect {
			if words[i] != expected {
				match = false
				break
			}
		}
		if match {
			return profile, true
		}
	}
	return Profile{}, false
}

// CSV renders the curated points in the same shape as the Modbus point-table
// import (名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位). The station id
// is a placeholder chosen by the caller; addresses and scaling come from the
// vendor document and remain unverified until a site read confirms them.
func (p Profile) CSV(serverID int) []byte {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"名称", "站号", "地址", "类型", "数据格式", "只读", "缩放", "偏移", "单位"})
	for _, point := range p.PointSpecs {
		readOnly := "否"
		if point.ReadOnly {
			readOnly = "是"
		}
		format := point.Format
		if format == "" {
			format = "uint16"
		}
		_ = writer.Write([]string{
			point.Name,
			strconv.Itoa(serverID),
			strconv.Itoa(int(point.Address)),
			csvFunction(point.Function),
			format,
			readOnly,
			strconv.FormatFloat(point.Scale, 'f', -1, 64),
			strconv.FormatFloat(point.Offset, 'f', -1, 64),
			point.Unit,
		})
	}
	writer.Flush()
	return buf.Bytes()
}

func csvFunction(function channel.Function) string {
	switch function {
	case channel.InputRegister:
		return "输入寄存器"
	case channel.Coil:
		return "线圈"
	case channel.DiscreteInput:
		return "离散输入"
	default:
		return "保持寄存器"
	}
}
