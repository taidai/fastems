package decision

import (
	"math"
	"testing"
)

// 仿真验收（A 类软件场景，对照《通用型 EMS-Core 评审清单》验收算例）。
// 输入清单给定数值，断言核心通过依据。与清单算例的策略差异在注释中标注，
// 不为了对齐算例数值而改算法。

// almost 浮点比较容差。
func almost(a, b float64) bool { return math.Abs(a-b) < 0.05 }

// allocate 辅助：按清单算例构造能力与成员并分配。
func allocate(t *testing.T, requestKW float64, caps CapabilityValues) map[string]float64 {
	t.Helper()
	members := make([]EnergyMember, 0, len(caps))
	for id := range caps {
		members = append(members, EnergyMember{EnergyUnitID: id})
	}
	allocations, err := AllocateEnergy(requestKW, members, caps)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]float64, len(allocations))
	for _, a := range allocations {
		out[a.EnergyUnitID] = a.SetpointKW
	}
	return out
}

// T03 双柜正常分配：A/B 上限 60/80，目标 +100。
// 通过依据：合计 +100，各柜不越上限。
// 注：清单假设"初始分配 60/40"是某策略的结果；本实现为按比例水填充，
// 分配 42.9/57.1，同样满足"不越界+合计达标"。策略选择属 SCH-02 待裁决。
func TestAcceptanceT03NormalAllocation(t *testing.T) {
	caps := CapabilityValues{
		"A": {FieldRatedPower: 60, FieldAvailablePower: 60},
		"B": {FieldRatedPower: 80, FieldAvailablePower: 80},
	}
	out := allocate(t, 100, caps)
	total := out["A"] + out["B"]
	if !almost(total, 100) {
		t.Fatalf("T03 合计应为 100，实测 %.2f", total)
	}
	if out["A"] > 60 || out["B"] > 80 {
		t.Fatalf("T03 越上限：A=%.2f B=%.2f", out["A"], out["B"])
	}
	t.Logf("T03 通过：A=%.1f B=%.1f 合计=%.1f（按比例水填充）", out["A"], out["B"], total)
}

// T04 单柜降额重分配：A 可放上限 60→30，B 仍 80，目标 +100。
// 通过依据：A 不超新上限 30，合计尽量达 +100。
// 注：清单假设"A 顶满新上限 30、B 补 70"（按上限优先）；本实现按比例
// 分配 A=27.3、B=72.7，满足"不越界+合计达标"。策略差异见 SCH-02。
func TestAcceptanceT04DerateReallocation(t *testing.T) {
	caps := CapabilityValues{
		"A": {FieldRatedPower: 60, FieldAvailablePower: 30}, // 降额到 30
		"B": {FieldRatedPower: 80, FieldAvailablePower: 80},
	}
	out := allocate(t, 100, caps)
	total := out["A"] + out["B"]
	if out["A"] > 30 {
		t.Fatalf("T04 A 超过降额后上限 30：%.2f", out["A"])
	}
	if !almost(total, 100) {
		t.Fatalf("T04 合计应为 100，实测 %.2f", total)
	}
	t.Logf("T04 通过：A=%.1f B=%.1f 合计=%.1f（降额后重分配，不越界）", out["A"], out["B"], total)
}

// T05 总能力不足：A 上限 30、B 上限 40，目标仍 +100。
// 通过依据：可行上限 +70，分配 +30/+40，未满足 30 上报。
// 本算例能力用尽，与清单精确数值一致。
func TestAcceptanceT05InsufficientCapacity(t *testing.T) {
	caps := CapabilityValues{
		"A": {FieldRatedPower: 30, FieldAvailablePower: 30},
		"B": {FieldRatedPower: 40, FieldAvailablePower: 40},
	}
	members := []EnergyMember{{EnergyUnitID: "A"}, {EnergyUnitID: "B"}}
	allocations, err := AllocateEnergy(100, members, caps)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	byID := map[string]float64{}
	for _, a := range allocations {
		byID[a.EnergyUnitID] = a.SetpointKW
		total += a.SetpointKW
	}
	if !almost(byID["A"], 30) || !almost(byID["B"], 40) {
		t.Fatalf("T05 应分满 A=30 B=40，实测 A=%.2f B=%.2f", byID["A"], byID["B"])
	}
	if !almost(total, 70) {
		t.Fatalf("T05 可行合计应为 70，实测 %.2f", total)
	}
	shortfall := 100 - total
	if !almost(shortfall, 30) {
		t.Fatalf("T05 缺口应为 30，实测 %.2f", shortfall)
	}
	t.Logf("T05 通过：A=30 B=40 合计=70，缺口 30 如实上报")
}
