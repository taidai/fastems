import { useEffect, useState } from "react"
import { IonButton, IonCard, IonCardContent, IonCardHeader, IonCardSubtitle, IonCardTitle, IonInput, IonSelect, IonSelectOption } from "@ionic/react"
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
    return <section className="panel energy-units-view">
      <div className="panel-heading"><div><span className="section-kicker">能源设备</span><h2>运行策略</h2><p>{units.find((unit) => unit.id === selected)?.asset_code || selected}</p></div><button className="secondary-button" onClick={() => setSelected("")}>返回列表</button></div>
      <DevicePanel token={token} unitId={selected} nodes={nodes} onError={onError} />
      <EnergyRuleEditor token={token} energyUnitId={selected} onError={onError} />
    </section>
  }

  return <section className="panel energy-units-view">
    <PlantStrategyPanel token={token} nodes={nodes} onError={onError} />
    <div className="panel-heading"><div><span className="section-kicker">能源设备</span><h2>能源设备</h2><p>每台能源设备独立按策略自动充放电，可挂设备、配策略。</p></div>
      <button className="secondary-button" onClick={() => showAdd ? setShowAdd(false) : openWizard()}>{showAdd ? "收起" : "+ 新建能源设备"}</button>
    </div>

    {showAdd && <div className="energy-unit-add wizard">
      <ol className="wizard-steps">
        {["能源设备信息", "挂设备", "选策略并启动"].map((title, index) => <li key={title} className={step > index + 1 ? "done" : step === index + 1 ? "active" : ""}>{index + 1}. {title}</li>)}
      </ol>

      {step === 1 && <div className="wizard-body">
        <p className="wizard-consequence">给这台能源设备起个名字、填额定功率。这一步只登记信息，不会动设备。</p>
        <div className="energy-rule-row">
          <input aria-label="节点名称" placeholder="能源设备名称，例如：2 号能源设备" value={name} onChange={(event) => setName(event.target.value)} />
          <span className="set-label">额定功率 kW</span>
          <input aria-label="额定功率" type="number" inputMode="decimal" value={rated} onChange={(event) => setRated(Number(event.target.value))} />
          <select aria-label="能源类型" value={energyType} onChange={(event) => setEnergyType(event.target.value)}>
            <option value="storage">储能（可充可放）</option>
            <option value="pv">光伏（只发电）</option>
            <option value="ev">充电桩（只耗电）</option>
          </select>
        </div>
      </div>}

      {step === 2 && <div className="wizard-body">
        <p className="wizard-consequence">把柜子的功率设定映射到一个实际点位。选了之后，调度才会真正写这台设备。</p>
        <div className="energy-rule-row">
          <span className="set-label">功率设定点位</span>
          <select aria-label="创建功率点位" value={pointId} onChange={(event) => setPointId(event.target.value)}>
            <option value="">选择 modbus 点位</option>
            {powerPoints.map((node) => <option key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</option>)}
          </select>
        </div>
        {powerPoints.length === 0 && <p className="form-note">还没有 modbus 点位，请先在"通道"页配置功率设定点位。</p>}
      </div>}

      {step === 3 && <div className="wizard-body">
        <p className="wizard-consequence">选一个运行策略。确认后柜子会按这个策略每秒自动充放电，请核对功率是否正确。</p>
        <div className="energy-rule-row">
          <select aria-label="创建策略模板" value={templateKey} onChange={(event) => setTemplateKey(event.target.value)}>
            {templates.map((t) => <option key={t.key} value={t.key}>{t.name}</option>)}
          </select>
          {templates.find((t) => t.key === templateKey)?.needKW && <>
            <span className="set-label">功率 kW</span>
            <input aria-label="创建策略功率" type="number" inputMode="decimal" value={templateKW} onChange={(event) => setTemplateKW(Number(event.target.value))} />
          </>}
          {templates.find((t) => t.key === templateKey)?.needSOC && <>
            <span className="set-label">SOC 阈值 %</span>
            <input aria-label="创建 SOC 阈值" type="number" inputMode="decimal" value={templateSOC} onChange={(event) => setTemplateSOC(Number(event.target.value))} />
          </>}
        </div>
        <div className="wizard-preview">
          <strong>确认后果</strong>
          <span>能源设备「{name || "未命名"}」（额定 {rated} kW）将按「{templates.find((t) => t.key === templateKey)?.name}」以 {templateKW} kW 自动{templateKey === "charge" ? "充电" : "放电"}。</span>
        </div>
      </div>}

      <div className="wizard-actions">
        {step > 1 && <button className="secondary-button" onClick={() => setStep(step - 1)}>上一步</button>}
        {step < 3
          ? <button className="primary-button" onClick={nextStep}>下一步</button>
          : <button className="primary-button" disabled={busy} onClick={() => void add()}>{busy ? "创建中…" : "确认并启动"}</button>}
      </div>
    </div>}

    {units.length === 0 ? <div className="empty-state"><strong>还没有能源设备</strong><span>点击"新建能源设备"，三步让一台柜子按策略自动跑起来。</span></div> : <div className="energy-unit-list">
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

  const inputClass = "h-10 rounded-lg border border-ems-line bg-white/95 px-3 text-sm text-ems-ink"
  const fieldClass = "grid min-w-36 flex-1 gap-1 text-xs text-ems-muted"

  return <IonCard className="m-0 rounded-2xl border border-ems-line bg-white/85 shadow-sm backdrop-blur-xl">
    <IonCardHeader className="pb-2">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><IonCardTitle className="text-base">柜内设备</IonCardTitle><IonCardSubtitle>映射功率点位后，调度才能写到真实设备</IonCardSubtitle></div>
        <IonButton size="small" fill="outline" onClick={() => setShowAdd(!showAdd)}>{showAdd ? "收起" : "+ 添加设备"}</IonButton>
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
          <label className={fieldClass}>设备名称
            <IonInput className={inputClass} aria-label="设备名称" placeholder="例如：1 号 PCS" value={name} onIonInput={(event) => setName(event.detail.value || "")} />
          </label>
          <label className={fieldClass}>额定功率（kW）
            <IonInput className={inputClass} aria-label="设备额定功率" type="number" value={rated} onIonInput={(event) => setRated(Number(event.detail.value))} />
          </label>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <label className={fieldClass}>功率设定点位
            <IonSelect className={inputClass} aria-label="功率设定点位" value={pointId} onIonChange={(event) => setPointId(event.detail.value)}>
              <IonSelectOption value="">选择 modbus 点位</IonSelectOption>
              {powerPoints.map((node) => <IonSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</IonSelectOption>)}
            </IonSelect>
          </label>
          <IonButton disabled={busy} onClick={() => void add()}>{busy ? "添加中…" : "添加"}</IonButton>
        </div>
        {powerPoints.length === 0 && <p className="m-0 text-xs text-ems-amber">还没有 modbus 点位，请先在"通道"页配置功率设定点位。</p>}
      </div>}
    </IonCardContent>
  </IonCard>
}
