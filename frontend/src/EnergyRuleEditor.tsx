import { useEffect, useState } from "react"
import { fetchEnergyRule, saveEnergyRule } from "./api"
import type { CapabilityRule, CapabilityCondition, CapabilityAction } from "./api"

const capabilityFields = [
  { value: "soc", label: "SOC %" },
  { value: "available_power", label: "可用功率 kW" },
  { value: "rated_power", label: "额定功率 kW" },
  { value: "status", label: "状态" },
]

const operators = [
  { value: "gt", label: ">" },
  { value: "ge", label: "≥" },
  { value: "lt", label: "<" },
  { value: "le", label: "≤" },
  { value: "eq", label: "=" },
  { value: "ne", label: "≠" },
]

function emptyRule(energyUnitId: string): CapabilityRule {
  return { id: "rule-" + energyUnitId, conditions: [], actions: [] }
}

// EnergyRuleEditor 配置一个 EnergyUnit 的能力级规则：条件引用能力字段，
// 动作是对该 EnergyUnit 的功率请求。这是能力级调度对用户可见的编辑入口。
export default function EnergyRuleEditor({ token, energyUnitId, onError }: {
  token: string
  energyUnitId: string
  onError: (message: string) => void
}) {
  const [rule, setRule] = useState<CapabilityRule | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let cancelled = false
    void fetchEnergyRule(token, energyUnitId)
      .then((result) => { if (!cancelled) setRule(result.id ? result : emptyRule(energyUnitId)) })
      .catch(() => { if (!cancelled) setRule(emptyRule(energyUnitId)) })
      .finally(() => { if (!cancelled) setLoaded(true) })
    return () => { cancelled = true }
  }, [token, energyUnitId])

  function setCondition(index: number, patch: Partial<CapabilityCondition>) {
    if (!rule) return
    const conditions = rule.conditions.map((condition, i) => i === index ? { ...condition, ...patch } : condition)
    setRule({ ...rule, conditions })
  }

  function setAction(index: number, patch: Partial<CapabilityAction>) {
    if (!rule) return
    const actions = rule.actions.map((action, i) => i === index ? { ...action, ...patch } : action)
    setRule({ ...rule, actions })
  }

  async function save() {
    if (!rule) return
    setBusy(true)
    try {
      await saveEnergyRule(token, energyUnitId, rule)
    } catch (error) {
      onError(error instanceof Error ? error.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  if (!loaded) return <div className="empty-state"><span>正在读取能力级规则…</span></div>

  return <section className="energy-rule-editor">
    <div className="panel-heading"><div><span className="section-kicker">能力级调度</span><h3>调度规则</h3><p>条件引用能源节点的能力字段，动作是对它请求功率；不认具体设备型号。</p></div></div>

    <div className="energy-rule-section">
      <strong>当以下条件同时满足</strong>
      {rule!.conditions.map((condition, index) => (
        <div className="energy-rule-row" key={index}>
          <select aria-label="能力字段" value={condition.ref.field} onChange={(event) => setCondition(index, { ref: { ...condition.ref, field: event.target.value } })}>
            {capabilityFields.map((field) => <option key={field.value} value={field.value}>{field.label}</option>)}
          </select>
          <select aria-label="比较逻辑" value={condition.operator} onChange={(event) => setCondition(index, { operator: event.target.value })}>
            {operators.map((operator) => <option key={operator.value} value={operator.value}>{operator.label}</option>)}
          </select>
          <input aria-label="条件值" type="number" inputMode="decimal" value={condition.value} onChange={(event) => setCondition(index, { value: Number(event.target.value) })} />
          <button className="icon-button" aria-label="删除条件" onClick={() => setRule({ ...rule!, conditions: rule!.conditions.filter((_, i) => i !== index) })}>×</button>
        </div>
      ))}
      <button className="secondary-button" onClick={() => setRule({ ...rule!, conditions: [...rule!.conditions, { ref: { energy_unit_id: energyUnitId, field: "soc" }, operator: "gt", value: 0 }] })}>+ 条件</button>
    </div>

    <div className="energy-rule-section">
      <strong>执行动作</strong>
      {rule!.actions.map((action, index) => (
        <div className="energy-rule-row" key={index}>
          <span className="set-label">请求功率</span>
          <input aria-label="请求功率" type="number" inputMode="decimal" value={action.request_kw} onChange={(event) => setAction(index, { request_kw: Number(event.target.value) })} />
          <span className="set-label">kW</span>
          <button className="icon-button" aria-label="删除动作" onClick={() => setRule({ ...rule!, actions: rule!.actions.filter((_, i) => i !== index) })}>×</button>
        </div>
      ))}
      <button className="secondary-button" onClick={() => setRule({ ...rule!, actions: [...rule!.actions, { energy_unit_id: energyUnitId, request_kw: 0 }] })}>+ 动作</button>
    </div>

    <div className="energy-rule-actions">
      <button className="primary-button" disabled={busy || rule!.conditions.length === 0 || rule!.actions.length === 0} onClick={() => void save()}>{busy ? "保存中…" : "保存规则"}</button>
    </div>
  </section>
}
