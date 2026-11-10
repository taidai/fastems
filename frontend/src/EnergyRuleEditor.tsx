import { useEffect, useState } from "react"
import { IonBadge, IonButton, IonCard, IonCardContent, IonCardHeader, IonCardSubtitle, IonCardTitle, IonInput, IonSelect, IonSelectOption } from "@ionic/react"
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

  const fieldClass = "grid min-w-36 flex-1 gap-1 text-xs text-ems-muted"
  const inputClass = "h-10 rounded-lg border border-ems-line bg-white/95 px-3 text-sm text-ems-ink"

  return <section className="grid gap-4">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div><span className="section-kicker">能力级调度</span><h3 className="mb-1 text-xl font-semibold text-ems-ink">调度规则</h3><p className="m-0 text-sm text-ems-muted">按能源设备的实时能力自动调节，不认具体设备型号。</p></div>
      <IonBadge color={published ? "success" : "medium"}>{published ? "已发布" : "草稿"}</IonBadge>
    </div>
    {published && rule!.published_at && <p className="m-0 text-xs text-ems-amber">发布于 {rule!.published_at}，修改后需重新发布才会生效。</p>}

    <IonCard className="m-0 rounded-2xl border border-ems-mint/40 bg-white/85 shadow-sm backdrop-blur-xl">
      <IonCardHeader className="pb-2">
        <IonCardTitle className="text-base">从模板创建</IonCardTitle>
        <IonCardSubtitle>选策略、填参数即可，不需要懂规则语法</IonCardSubtitle>
      </IonCardHeader>
      <IonCardContent>
        <div className="flex flex-wrap items-end gap-3">
          <label className={fieldClass}>策略模板
            <IonSelect className={inputClass} aria-label="策略模板" value={templateKey} onIonChange={(event) => setTemplateKey(event.detail.value)}>
              {templates.map((t) => <IonSelectOption key={t.key} value={t.key}>{t.name}</IonSelectOption>)}
            </IonSelect>
          </label>
          {templates.find((t) => t.key === templateKey)?.needKW && <label className={fieldClass}>功率（kW）
            <IonInput className={inputClass} aria-label="模板功率" type="number" value={templateKW} onIonInput={(event) => setTemplateKW(Number(event.detail.value))} />
          </label>}
          {templates.find((t) => t.key === templateKey)?.needSOC && <label className={fieldClass}>SOC 阈值（%）
            <IonInput className={inputClass} aria-label="模板 SOC 阈值" type="number" value={templateSOC} onIonInput={(event) => setTemplateSOC(Number(event.detail.value))} />
          </label>}
          <IonButton fill="outline" onClick={applyTemplate}>应用模板</IonButton>
        </div>
        <p className="mb-0 mt-3 text-xs text-ems-muted">{templates.find((t) => t.key === templateKey)?.desc}。应用后可微调，再发布生效。</p>
      </IonCardContent>
    </IonCard>

    <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(280px,1fr)]">
      <details className="energy-advanced rounded-2xl border border-ems-line bg-white/80 p-4 backdrop-blur-xl">
        <summary className="cursor-pointer text-sm font-medium text-ems-muted">高级：手动编辑条件与动作</summary>
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <section className="grid content-start gap-3 rounded-xl border border-ems-line p-3">
            <strong className="text-sm text-ems-ink">条件（同时满足）</strong>
            {rule!.conditions.map((condition, index) => (
              <div className="flex flex-wrap items-end gap-2" key={index}>
                <label className={fieldClass}>能力字段
                  <IonSelect className={inputClass} aria-label="能力字段" value={condition.ref.field} onIonChange={(event) => setCondition(index, { ref: { ...condition.ref, field: event.detail.value } })}>
                    {capabilityFields.map((field) => <IonSelectOption key={field.value} value={field.value}>{field.label}</IonSelectOption>)}
                  </IonSelect>
                </label>
                <label className="grid w-20 gap-1 text-xs text-ems-muted">比较
                  <IonSelect className={inputClass} aria-label="比较逻辑" value={condition.operator} onIonChange={(event) => setCondition(index, { operator: event.detail.value })}>
                    {operators.map((operator) => <IonSelectOption key={operator.value} value={operator.value}>{operator.label}</IonSelectOption>)}
                  </IonSelect>
                </label>
                <label className="grid w-24 gap-1 text-xs text-ems-muted">条件值
                  <IonInput className={inputClass} aria-label="条件值" type="number" value={condition.value} onIonInput={(event) => setCondition(index, { value: Number(event.detail.value) })} />
                </label>
                <IonButton fill="clear" color="danger" aria-label="删除条件" onClick={() => setRule({ ...rule!, conditions: rule!.conditions.filter((_, i) => i !== index) })}>×</IonButton>
              </div>
            ))}
            <IonButton size="small" fill="outline" onClick={() => setRule({ ...rule!, conditions: [...rule!.conditions, { ref: { energy_unit_id: energyUnitId, field: "soc" }, operator: "gt", value: 0 }] })}>+ 条件</IonButton>
          </section>

          <section className="grid content-start gap-3 rounded-xl border border-ems-line p-3">
            <strong className="text-sm text-ems-ink">执行动作</strong>
            {rule!.actions.map((action, index) => (
              <div className="flex flex-wrap items-end gap-2" key={index}>
                <label className={fieldClass}>请求功率（kW）
                  <IonInput className={inputClass} aria-label="请求功率" type="number" value={action.request_kw} onIonInput={(event) => setAction(index, { request_kw: Number(event.detail.value) })} />
                </label>
                <IonButton fill="clear" color="danger" aria-label="删除动作" onClick={() => setRule({ ...rule!, actions: rule!.actions.filter((_, i) => i !== index) })}>×</IonButton>
              </div>
            ))}
            <IonButton size="small" fill="outline" onClick={() => setRule({ ...rule!, actions: [...rule!.actions, { energy_unit_id: energyUnitId, request_kw: 0 }] })}>+ 动作</IonButton>
          </section>
        </div>
      </details>

      <IonCard className="m-0 rounded-2xl border border-ems-line bg-white/80 shadow-sm backdrop-blur-xl">
        <IonCardHeader className="pb-2"><IonCardTitle className="text-base">模拟</IonCardTitle><IonCardSubtitle>只求值，绝不下发设备</IonCardSubtitle></IonCardHeader>
        <IonCardContent>
          <div className="grid gap-3 sm:grid-cols-2">
            <label className={fieldClass}>SOC（%）
              <IonInput className={inputClass} aria-label="模拟 SOC" type="number" value={simSoc} onIonInput={(event) => setSimSoc(Number(event.detail.value))} />
            </label>
            <label className={fieldClass}>可用功率（kW）
              <IonInput className={inputClass} aria-label="模拟可用功率" type="number" value={simPower} onIonInput={(event) => setSimPower(Number(event.detail.value))} />
            </label>
          </div>
          <IonButton className="mt-3" fill="outline" disabled={busy} onClick={() => void simulate()}>运行模拟</IonButton>
          {simResult && <p className="mb-0 mt-3 rounded-lg bg-black/5 p-3 text-xs text-ems-ink">结果：{simResult}</p>}
        </IonCardContent>
      </IonCard>
    </div>

    <div className="flex flex-wrap justify-end gap-2 border-t border-ems-line pt-4">
      <IonButton fill="outline" disabled={busy} onClick={() => void saveDraft()}>{busy ? "保存中…" : "保存草稿"}</IonButton>
      {published
        ? <IonButton fill="outline" disabled={busy} onClick={() => void unpublish()}>{busy ? "停用中…" : "停用"}</IonButton>
        : <IonButton disabled={busy || rule!.conditions.length === 0 || rule!.actions.length === 0} onClick={() => setPublishConfirm(true)}>{busy ? "发布中…" : "发布"}</IonButton>}
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
