import { useEffect, useState } from "react"
import { fetchEnergyRule, saveEnergyRule, publishEnergyRule, simulateEnergyRule } from "./api"
import type { CapabilityRule, CapabilityCondition, CapabilityAction } from "./api"
import { templates, buildFromTemplate } from "./strategyTemplates"
import ConfirmDialog from "./ConfirmDialog"

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
  return { id: "rule-" + energyUnitId, state: "draft", conditions: [], actions: [] }
}

// EnergyRuleEditor 配置一个 EnergyUnit 的能力级规则：条件引用能力字段，
// 动作是对该 EnergyUnit 的功率请求。草稿保存不改变运行，显式发布才生效；
// 模拟只求值，绝不写设备。
export default function EnergyRuleEditor({ token, energyUnitId, onError }: {
  token: string
  energyUnitId: string
  onError: (message: string) => void
}) {
  const [rule, setRule] = useState<CapabilityRule | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [simSoc, setSimSoc] = useState(60)
  const [simPower, setSimPower] = useState(200)
  const [simResult, setSimResult] = useState<string>("")
  const [templateKey, setTemplateKey] = useState("discharge")
  const [templateKW, setTemplateKW] = useState(50)
  const [templateSOC, setTemplateSOC] = useState(20)
  const [publishConfirm, setPublishConfirm] = useState(false)

  function applyTemplate() {
    const template = templates.find((t) => t.key === templateKey)
    if (template?.needKW && templateKW <= 0) { onError("请填写大于 0 的功率"); return }
    setRule(buildFromTemplate(templateKey, energyUnitId, templateKW, templateSOC))
  }

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

  async function saveDraft() {
    if (!rule) return
    setBusy(true)
    try {
      await saveEnergyRule(token, energyUnitId, { ...rule, state: "draft" })
      setRule({ ...rule, state: "draft" })
    } catch (error) {
      onError(error instanceof Error ? error.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  async function publish() {
    if (!rule) return
    setBusy(true)
    try {
      await saveEnergyRule(token, energyUnitId, { ...rule, state: "draft" })
      await publishEnergyRule(token, energyUnitId)
      const refreshed = await fetchEnergyRule(token, energyUnitId)
      setRule(refreshed.id ? refreshed : rule)
    } catch (error) {
      onError(error instanceof Error ? error.message : "发布失败")
    } finally {
      setBusy(false)
    }
  }

  async function unpublish() {
    if (!rule) return
    setBusy(true)
    try {
      await saveEnergyRule(token, energyUnitId, { ...rule, state: "draft" })
      setRule({ ...rule, state: "draft" })
    } catch (error) {
      onError(error instanceof Error ? error.message : "停用失败")
    } finally {
      setBusy(false)
    }
  }

  async function simulate() {
    if (!rule) return
    setBusy(true)
    setSimResult("")
    try {
      const values = { [energyUnitId]: { soc: simSoc, available_power: simPower, status: 1 } }
      const result = await simulateEnergyRule(token, energyUnitId, rule, values)
      if (!result.actions || result.actions.length === 0) {
        setSimResult("无命中动作：没有条件同时满足，不请求功率。")
      } else {
        setSimResult(result.actions.map((action) => `${action.request_kw > 0 ? "充电" : "放电"} ${Math.abs(action.request_kw)} kW`).join("；"))
      }
    } catch (error) {
      onError(error instanceof Error ? error.message : "模拟失败")
    } finally {
      setBusy(false)
    }
  }

  if (!loaded) return <div className="empty-state"><span>正在读取能力级规则…</span></div>

  const published = rule!.state === "published"

  return <section className="energy-rule-editor">
    <div className="panel-heading"><div><span className="section-kicker">能力级调度</span><h3>调度规则</h3><p>条件引用能源节点的能力字段，动作是对它请求功率；不认具体设备型号。</p></div>
      <span className={"energy-rule-state" + (published ? " published" : "")}>{published ? "已发布" : "草稿"}</span>
    </div>
    {published && rule!.published_at && <p className="form-note">发布于 {rule!.published_at}。修改后需重新发布才会生效。</p>}

    <div className="energy-rule-section energy-template-section">
      <strong>从模板创建（推荐，无需懂规则语法）</strong>
      <div className="energy-rule-row">
        <select aria-label="策略模板" value={templateKey} onChange={(event) => setTemplateKey(event.target.value)}>
          {templates.map((t) => <option key={t.key} value={t.key}>{t.name}</option>)}
        </select>
        {templates.find((t) => t.key === templateKey)?.needKW && <>
          <span className="set-label">功率 kW</span>
          <input aria-label="模板功率" type="number" inputMode="decimal" value={templateKW} onChange={(event) => setTemplateKW(Number(event.target.value))} />
        </>}
        {templates.find((t) => t.key === templateKey)?.needSOC && <>
          <span className="set-label">SOC 阈值 %</span>
          <input aria-label="模板 SOC 阈值" type="number" inputMode="decimal" value={templateSOC} onChange={(event) => setTemplateSOC(Number(event.target.value))} />
        </>}
        <button className="secondary-button" onClick={applyTemplate}>应用模板</button>
      </div>
      <p className="form-note">{templates.find((t) => t.key === templateKey)?.desc}。应用后可微调，再发布生效。</p>
    </div>

    <details className="energy-advanced">
      <summary>高级：手动编辑条件与动作</summary>
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
    </details>

    <div className="energy-rule-section">
      <strong>模拟（只求值，不下发）</strong>
      <div className="energy-rule-row">
        <span className="set-label">SOC %</span>
        <input aria-label="模拟 SOC" type="number" inputMode="decimal" value={simSoc} onChange={(event) => setSimSoc(Number(event.target.value))} />
        <span className="set-label">可用功率 kW</span>
        <input aria-label="模拟可用功率" type="number" inputMode="decimal" value={simPower} onChange={(event) => setSimPower(Number(event.target.value))} />
        <button className="secondary-button" disabled={busy} onClick={() => void simulate()}>模拟</button>
      </div>
      {simResult && <p className="form-note">结果：{simResult}</p>}
    </div>

    <div className="energy-rule-actions">
      <button className="secondary-button" disabled={busy} onClick={() => void saveDraft()}>{busy ? "保存中…" : "保存草稿"}</button>
      {published
        ? <button className="secondary-button" disabled={busy} onClick={() => void unpublish()}>{busy ? "停用中…" : "停用"}</button>
        : <button className="primary-button" disabled={busy || rule!.conditions.length === 0 || rule!.actions.length === 0} onClick={() => setPublishConfirm(true)}>{busy ? "发布中…" : "发布"}</button>}
    </div>

    {publishConfirm && <ConfirmDialog
      title="发布调度规则"
      message={`发布后，该能源节点将按此规则每秒自动调节功率${rule!.actions[0] ? `（${rule!.actions[0].request_kw >= 0 ? "放电" : "充电"} ${Math.abs(rule!.actions[0].request_kw)} kW）` : ""}。确认发布？`}
      confirmLabel="确认发布"
      busy={busy}
      onCancel={() => setPublishConfirm(false)}
      onConfirm={() => { setPublishConfirm(false); void publish() }}
    />}
  </section>
}
