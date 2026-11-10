import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSectionHeader, EmsSelect, EmsSelectOption, EmptyState, InlineNotice } from "./components/ems"
import { fetchEnergyUnits, createEnergyUnit, saveEnergyRule, publishEnergyRule } from "./api"
import type { EnergyUnitSummary, Node } from "./api"
import { templates, buildFromTemplate } from "./strategyTemplates"
import EnergyRuleEditor from "./EnergyRuleEditor"
import PlantStrategyPanel from "./PlantStrategyPanel"

// EnergyUnitsView 是 EMS 业务层：能源设备与策略。
// 设备与功率点位映射属于接入建模，见 UnitWiringView。
export default function EnergyUnitsView({ token, nodes, onError }: {
  token: string
  nodes: Node[]
  onError: (message: string) => void
}) {
  const [units, setUnits] = useState<EnergyUnitSummary[]>([])
  const [selected, setSelected] = useState<string>("")
  const [showAdd, setShowAdd] = useState(false)
  const [step, setStep] = useState(1)
  const [name, setName] = useState("")
  const [rated, setRated] = useState(100)
  const [energyType, setEnergyType] = useState("storage")
  const [templateKey, setTemplateKey] = useState("discharge")
  const [templateKW, setTemplateKW] = useState(50)
  const [templateSOC, setTemplateSOC] = useState(20)
  const [busy, setBusy] = useState(false)

  function refresh() {
    void fetchEnergyUnits(token)
      .then((result) => setUnits(result.units || []))
      .catch(() => onError("能源设备列表读取失败"))
  }
  useEffect(refresh, [token])

  function openWizard() {
    setStep(1)
    setShowAdd(true)
  }

  function nextStep() {
    if (step === 1) {
      if (!name.trim()) { onError("先填能源设备名称"); return }
      if (rated <= 0) { onError("额定功率必须大于 0"); return }
    }
    setStep(step + 1)
  }

  // 建能源设备并发布策略；设备接线在“接入建模 → 能源设备接线”完成。
  async function add() {
    if (templateKW <= 0) { onError("请填写大于 0 的策略功率"); return }
    setBusy(true)
    try {
      const unit = await createEnergyUnit(token, { description: name.trim(), rated_power_kw: rated, energy_type: energyType })
      const rule = buildFromTemplate(templateKey, unit.id, templateKW, templateSOC)
      await saveEnergyRule(token, unit.id, rule)
      await publishEnergyRule(token, unit.id)
      setName(""); setRated(100); setShowAdd(false)
      refresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "创建失败")
    } finally {
      setBusy(false)
    }
  }

  if (selected) {
    return <section className="grid gap-4">
      <EmsSectionHeader kicker="能源设备" title="运行策略" description={units.find((unit) => unit.id === selected)?.asset_code || selected}
        actions={<EmsButton variant="secondary" onClick={() => setSelected("")}>返回列表</EmsButton>} />
      <EnergyRuleEditor token={token} energyUnitId={selected} onError={onError} />
    </section>
  }

  return <section className="grid gap-4">
    <PlantStrategyPanel token={token} nodes={nodes} onError={onError} />
    <EmsSectionHeader kicker="能源设备" title="能源设备" description="每台能源设备独立按策略自动充放电，可挂设备、配策略。"
      actions={<EmsButton variant="secondary" onClick={() => showAdd ? setShowAdd(false) : openWizard()}>{showAdd ? "收起" : "+ 新建能源设备"}</EmsButton>} />

    {showAdd && <EmsCard variant="subtle"><IonCardContent className="grid gap-4">
      <ol className="wizard-steps">
        {["能源设备信息", "选策略并启动"].map((title, index) => <li key={title} className={step > index + 1 ? "done" : step === index + 1 ? "active" : ""}>{index + 1}. {title}</li>)}
      </ol>

      {step === 1 && <div className="grid gap-3">
        <p className="m-0 text-sm text-ems-muted">给这台能源设备起个名字、填额定功率。这一步只登记信息，不会动设备。</p>
        <div className="grid gap-3 sm:grid-cols-2">
          <EmsInput label="能源设备名称" aria-label="节点名称" placeholder="例如：2 号能源设备" value={name} onIonInput={(event) => setName(event.detail.value || "")} />
          <EmsInput label="额定功率（kW）" aria-label="额定功率" type="number" value={rated} onIonInput={(event) => setRated(Number(event.detail.value))} />
        </div>
        <EmsSelect label="能源类型" aria-label="能源类型" value={energyType} onIonChange={(event) => setEnergyType(event.detail.value)}>
          <EmsSelectOption value="storage">储能（可充可放）</EmsSelectOption>
          <EmsSelectOption value="pv">光伏（只发电）</EmsSelectOption>
          <EmsSelectOption value="ev">充电桩（只耗电）</EmsSelectOption>
        </EmsSelect>
      </div>}

      {step === 2 && <div className="grid gap-3">
        <p className="m-0 text-sm text-ems-muted">选一个运行策略。确认后柜子会按这个策略每秒自动充放电；设备接线在“接入建模”完成。</p>
        <div className="grid gap-3 sm:grid-cols-2">
          <EmsSelect label="策略模板" aria-label="创建策略模板" value={templateKey} onIonChange={(event) => setTemplateKey(event.detail.value)}>
            {templates.map((t) => <EmsSelectOption key={t.key} value={t.key}>{t.name}</EmsSelectOption>)}
          </EmsSelect>
          {templates.find((t) => t.key === templateKey)?.needKW && <EmsInput label="功率（kW）" aria-label="创建策略功率" type="number" value={templateKW} onIonInput={(event) => setTemplateKW(Number(event.detail.value))} />}
          {templates.find((t) => t.key === templateKey)?.needSOC && <EmsInput label="SOC 阈值（%）" aria-label="创建 SOC 阈值" type="number" value={templateSOC} onIonInput={(event) => setTemplateSOC(Number(event.detail.value))} />}
        </div>
        <InlineNotice tone="warning"><strong>确认后果：</strong>能源设备「{name || "未命名"}」（额定 {rated} kW）将按「{templates.find((t) => t.key === templateKey)?.name}」以 {templateKW} kW 自动{templateKey === "charge" ? "充电" : "放电"}。</InlineNotice>
      </div>}

      <div className="flex justify-end gap-2">
        {step > 1 && <EmsButton variant="secondary" onClick={() => setStep(step - 1)}>上一步</EmsButton>}
        {step < 2
          ? <EmsButton onClick={nextStep}>下一步</EmsButton>
          : <EmsButton busy={busy} busyLabel="创建中…" onClick={() => void add()}>确认并启动</EmsButton>}
      </div>
    </IonCardContent></EmsCard>}

    {units.length === 0 ? <EmptyState title="还没有能源设备" description="点击“新建能源设备”，两步建好柜体与策略；设备接线在“接入建模”完成。" /> : <div className="energy-unit-list">
      {units.map((unit) => <button key={unit.id} className="energy-unit-row" onClick={() => setSelected(unit.id)}>
        <strong>{unit.asset_code || unit.description || unit.id}</strong>
        <span>{unit.description || unit.id}</span>
        <span aria-hidden="true">→</span>
      </button>)}
    </div>}
  </section>
}
