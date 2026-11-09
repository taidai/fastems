package decision

import "testing"

// T27 资源分组与共用约束：两台单元各放 60kW，总目标 +100kW。
// 仅标签：分配合计 +100，个体不超 60；有共享 70 上限：合计≤70、缺口 30，
// 不重复下发。
func TestAcceptanceT27SharedConstraint(t *testing.T) {
	caps := CapabilityValues{
		"A": {FieldRatedPower: 60, FieldAvailablePower: 60},
		"B": {FieldRatedPower: 60, FieldAvailablePower: 60},
	}
	members := []EnergyMember{{EnergyUnitID: "A"}, {EnergyUnitID: "B"}}

	t.Run("label only no shared limit", func(t *testing.T) {
		group := ResourceGroup{GroupID: "g", Members: members, SharedLimitKW: 0}
		result, err := AllocateWithGroupLimit(100, group, caps)
		if err != nil {
			t.Fatal(err)
		}
		if result.DeliveredKW != 100 {
			t.Fatalf("无共享上限应分满 100，实测 %.1f", result.DeliveredKW)
		}
		for _, a := range result.Allocations {
			if a.SetpointKW > 60 {
				t.Fatalf("个体超 60：%s=%.1f", a.EnergyUnitID, a.SetpointKW)
			}
		}
		if result.ShortfallKW != 0 {
			t.Fatalf("无上限不应有缺口，实测 %.1f", result.ShortfallKW)
		}
	})

	t.Run("shared 70 limit caps total", func(t *testing.T) {
		group := ResourceGroup{GroupID: "g", Members: members, SharedLimitKW: 70}
		result, err := AllocateWithGroupLimit(100, group, caps)
		if err != nil {
			t.Fatal(err)
		}
		if result.DeliveredKW > 70 {
			t.Fatalf("共享 70 上限应封顶合计，实测 %.1f", result.DeliveredKW)
		}
		if result.ShortfallKW != 30 {
			t.Fatalf("缺口应为 30，实测 %.1f", result.ShortfallKW)
		}
		if result.LimitedBy != "group_shared_limit" {
			t.Fatalf("应标注组上限限制来源，实测 %q", result.LimitedBy)
		}
		t.Logf("T27 通过：共享上限 70，合计 %.1f，缺口 %.1f", result.DeliveredKW, result.ShortfallKW)
	})
}
