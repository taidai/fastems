import type { CapabilityRule } from "./api"

// 策略模板：把常见储能策略包装成业务参数，用户不用懂能力字段/规则语法。
// 方向语义与 alloc 一致：正值=放电、负值=充电。
export type Template = { key: string; name: string; desc: string; needKW: boolean; needSOC: boolean }

export const templates: Template[] = [
  { key: "discharge", name: "固定放电", desc: "持续按固定功率放电", needKW: true, needSOC: false },
  { key: "charge", name: "固定充电", desc: "持续按固定功率充电", needKW: true, needSOC: false },
  { key: "soc_discharge", name: "SOC 低时禁放", desc: "SOC 低于下限就停止放电，保护电池", needKW: true, needSOC: true },
  { key: "soc_charge", name: "SOC 高时禁充", desc: "SOC 高于上限就停止充电，保护电池", needKW: true, needSOC: true },
]

export function buildFromTemplate(key: string, energyUnitId: string, kw: number, soc: number): CapabilityRule {
  const base = { id: "rule-" + energyUnitId, state: "draft" }
  const always = { ref: { energy_unit_id: energyUnitId, field: "rated_power" }, operator: "gt", value: 0 }
  switch (key) {
    case "discharge":
      return { ...base, conditions: [always], actions: [{ energy_unit_id: energyUnitId, request_kw: Math.abs(kw) }] }
    case "charge":
      return { ...base, conditions: [always], actions: [{ energy_unit_id: energyUnitId, request_kw: -Math.abs(kw) }] }
    case "soc_discharge":
      return { ...base, conditions: [{ ref: { energy_unit_id: energyUnitId, field: "soc" }, operator: "gt", value: soc }], actions: [{ energy_unit_id: energyUnitId, request_kw: Math.abs(kw) }] }
    case "soc_charge":
      return { ...base, conditions: [{ ref: { energy_unit_id: energyUnitId, field: "soc" }, operator: "lt", value: soc }], actions: [{ energy_unit_id: energyUnitId, request_kw: -Math.abs(kw) }] }
    default:
      return { ...base, conditions: [], actions: [] }
  }
}
