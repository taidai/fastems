import { useEffect, useState } from "react"
import { IonCardContent, IonCardHeader, IonCardSubtitle, IonCardTitle } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSectionHeader, EmsSelect, EmsSelectOption, EmptyState, InlineNotice } from "./components/ems"
import { fetchEnergyUnits, createEnergyUnit, fetchEnergyDevices, createEnergyDevice, saveEnergyRule, publishEnergyRule } from "./api"
import type { EnergyUnitSummary, EnergyDevice, Node } from "./api"
import { templates, buildFromTemplate } from "./strategyTemplates"
import EnergyRuleEditor from "./EnergyRuleEditor"
import PlantStrategyPanel from "./PlantStrategyPanel"

// EnergyUnitsView 列出能源节点（EnergyUnit），点击进入能力级规则编辑器。
// 支持用户自建能源节点并挂设备（Device + 功率点位映射），真正可控。
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
  const [pointId, setPointId] = useState("")
  const [templateKey, setTemplateKey] = useState("discharge")
  const [templateKW, setTemplateKW] = useState(50)
  const [templateSOC, setTemplateSOC] = useState(20)
  const [busy, setBusy] = useState(false)
  const powerPoints = nodes.filter((node) => node.type === "modbusIo")

  function refresh() {
    void fetchEnergyUnits(token)
      .then((result) => setUnits(result.units || []))
      .catch(() => onError("能源节点列表读取失败"))
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
    if (step === 2 && !pointId) { onError("选择功率设定点位，调度才能写到设备"); return }
    setStep(step + 1)
  }

  // 引导式创建：一个表单同时建能源节点、挂设备、配策略并发布，
  // 让柜子直接跑起来，不用分步懂技术概念。
  async function add() {
    if (templateKW <= 0) { onError("请填写大于 0 的策略功率"); return }
    setBusy(true)
    try {
      const unit = await createEnergyUnit(token, { description: name.trim(), rated_power_kw: rated, energy_type: energyType })
      await createEnergyDevice(token, unit.id, { description: name.trim() + "-PCS", rated_power_kw: rated, power_setpoint_id: pointId })
      const rule = buildFromTemplate(templateKey, unit.id, templateKW, templateSOC)
      await saveEnergyRule(token, unit.id, rule)
      await publishEnergyRule(token, unit.id)
      setName(""); setRated(100); setPointId(""); setShowAdd(false)
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
      <DevicePanel token={token} unitId={selected} nodes={nodes} onError={onError} />
      <EnergyRuleEditor token={token} energyUnitId={selected} onError={onError} />
    </section>
  }

  return <section className="grid gap-4">
    <PlantStrategyPanel token={token} nodes={nodes} onError={onError} />
    <EmsSectionHeader kicker="能源设备" title="能源设备" description="每台能源设备独立按策略自动充放电，可挂设备、配策略。"
      actions={<EmsButton variant="secondary" onClick={() => showAdd ? setShowAdd(false) : openWizard()}>{showAdd ? "收起" : "+ 新建能源设备"}</EmsButton>} />

    {showAdd && <EmsCard variant="subtle"><IonCardContent className="grid gap-4">
      <ol className="wizard-steps">
        {["能源设备信息", "挂设备", "选策略并启动"].map((title, index) => <li key={title} className={step > index + 1 ? "done" : step === index + 1 ? "active" : ""}>{index + 1}. {title}</li>)}
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
        <p className="m-0 text-sm text-ems-muted">把柜子的功率设定映射到一个实际点位。选了之后，调度才会真正写这台设备。</p>
        <EmsSelect label="功率设定点位" aria-label="创建功率点位" value={pointId} onIonChange={(event) => setPointId(event.detail.value)}>
          <EmsSelectOption value="">选择 modbus 点位</EmsSelectOption>
          {powerPoints.map((node) => <EmsSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</EmsSelectOption>)}
        </EmsSelect>
        {powerPoints.length === 0 && <InlineNotice tone="warning">还没有 modbus 点位，请先在"通道"页配置功率设定点位。</InlineNotice>}
      </div>}

      {step === 3 && <div className="grid gap-3">
        <p className="m-0 text-sm text-ems-muted">选一个运行策略。确认后柜子会按这个策略每秒自动充放电，请核对功率是否正确。</p>
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
        {step < 3
          ? <EmsButton onClick={nextStep}>下一步</EmsButton>
          : <EmsButton busy={busy} busyLabel="创建中…" onClick={() => void add()}>确认并启动</EmsButton>}
      </div>
    </IonCardContent></EmsCard>}

    {units.length === 0 ? <EmptyState title="还没有能源设备" description="点击“新建能源设备”，三步让一台柜子按策略自动跑起来。" /> : <div className="energy-unit-list">
      {units.map((unit) => <button key={unit.id} className="energy-unit-row" onClick={() => setSelected(unit.id)}>
        <strong>{unit.asset_code || unit.description || unit.id}</strong>
        <span>{unit.description || unit.id}</span>
        <span aria-hidden="true">→</span>
      </button>)}
    </div>}
  </section>
}

// DevicePanel 管理能源节点下的设备：列表 + 添加设备（映射功率设定点位）。
function DevicePanel({ token, unitId, nodes, onError }: {
  token: string
  unitId: string
  nodes: Node[]
  onError: (message: string) => void
}) {
  const [devices, setDevices] = useState<EnergyDevice[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [name, setName] = useState("")
  const [rated, setRated] = useState(100)
  const [pointId, setPointId] = useState("")
  const [busy, setBusy] = useState(false)
  const powerPoints = nodes.filter((node) => node.type === "modbusIo")

  function refresh() {
    void fetchEnergyDevices(token, unitId)
      .then((result) => setDevices(result.devices || []))
      .catch(() => onError("设备列表读取失败"))
  }
  useEffect(refresh, [token, unitId])

  async function add() {
    if (!name.trim()) { onError("请填写设备名称"); return }
    if (rated <= 0) { onError("额定功率必须大于 0"); return }
    if (!pointId) { onError("请选择功率设定点位"); return }
    setBusy(true)
    try {
      await createEnergyDevice(token, unitId, { description: name.trim(), rated_power_kw: rated, power_setpoint_id: pointId })
      setName(""); setRated(100); setPointId(""); setShowAdd(false)
      refresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "添加设备失败")
    } finally {
      setBusy(false)
    }
  }

  return <EmsCard>
    <IonCardHeader className="pb-2">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><IonCardTitle className="text-base">柜内设备</IonCardTitle><IonCardSubtitle>映射功率点位后，调度才能写到真实设备</IonCardSubtitle></div>
        <EmsButton variant="secondary" size="small" onClick={() => setShowAdd(!showAdd)}>{showAdd ? "收起" : "+ 添加设备"}</EmsButton>
      </div>
    </IonCardHeader>
    <IonCardContent>
      {devices.length === 0 ? <p className="m-0 text-sm text-ems-muted">还没有设备。添加一台 PCS 并映射功率点位，调度才能下发。</p> : <div className="grid gap-2">
        {devices.map((device) => <div key={device.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-ems-line px-3 py-2 text-sm">
          <strong className="text-ems-ink">{device.description || device.asset_code}</strong>
          <span className="text-ems-muted">额定 {device.rated_power_kw} kW</span>
          <span className="break-all text-xs text-ems-muted">功率点位 {device.power_setpoint_id || "未映射"}</span>
        </div>)}
      </div>}

      {showAdd && <div className="mt-4 grid gap-3 border-t border-ems-line pt-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <EmsInput label="设备名称" aria-label="设备名称" placeholder="例如：1 号 PCS" value={name} onIonInput={(event) => setName(event.detail.value || "")} />
          <EmsInput label="额定功率（kW）" aria-label="设备额定功率" type="number" value={rated} onIonInput={(event) => setRated(Number(event.detail.value))} />
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <EmsSelect label="功率设定点位" aria-label="功率设定点位" value={pointId} onIonChange={(event) => setPointId(event.detail.value)}>
            <EmsSelectOption value="">选择 modbus 点位</EmsSelectOption>
            {powerPoints.map((node) => <EmsSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</EmsSelectOption>)}
          </EmsSelect>
          <EmsButton busy={busy} busyLabel="添加中…" onClick={() => void add()}>添加</EmsButton>
        </div>
        {powerPoints.length === 0 && <p className="m-0 text-xs text-ems-amber">还没有 modbus 点位，请先在"通道"页配置功率设定点位。</p>}
      </div>}
    </IonCardContent>
  </EmsCard>
}
