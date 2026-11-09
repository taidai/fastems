import { useEffect, useState } from "react"
import { fetchEnergyUnits, createEnergyUnit, fetchEnergyDevices, createEnergyDevice } from "./api"
import type { EnergyUnitSummary, EnergyDevice, Node } from "./api"
import EnergyRuleEditor from "./EnergyRuleEditor"

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
  const [name, setName] = useState("")
  const [rated, setRated] = useState(100)
  const [busy, setBusy] = useState(false)

  function refresh() {
    void fetchEnergyUnits(token)
      .then((result) => setUnits(result.units || []))
      .catch(() => onError("能源节点列表读取失败"))
  }
  useEffect(refresh, [token])

  async function add() {
    if (!name.trim()) { onError("请填写能源节点名称"); return }
    if (rated <= 0) { onError("额定功率必须大于 0"); return }
    setBusy(true)
    try {
      await createEnergyUnit(token, { description: name.trim(), rated_power_kw: rated })
      setName(""); setRated(100); setShowAdd(false)
      refresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "创建失败")
    } finally {
      setBusy(false)
    }
  }

  if (selected) {
    return <section className="panel energy-units-view">
      <div className="panel-heading"><div><span className="section-kicker">能源节点</span><h2>能力级规则</h2><p>{units.find((unit) => unit.id === selected)?.asset_code || selected}</p></div><button className="secondary-button" onClick={() => setSelected("")}>返回列表</button></div>
      <DevicePanel token={token} unitId={selected} nodes={nodes} onError={onError} />
      <EnergyRuleEditor token={token} energyUnitId={selected} onError={onError} />
    </section>
  }

  return <section className="panel energy-units-view">
    <div className="panel-heading"><div><span className="section-kicker">能源节点</span><h2>能源单元</h2><p>一个柜体（能源单元）是一条能力级规则与分配的调度对象。</p></div>
      <button className="secondary-button" onClick={() => setShowAdd(!showAdd)}>{showAdd ? "收起" : "+ 新建能源节点"}</button>
    </div>

    {showAdd && <div className="energy-unit-add">
      <div className="energy-rule-row">
        <input aria-label="节点名称" placeholder="例如：2 号储能柜" value={name} onChange={(event) => setName(event.target.value)} />
        <span className="set-label">额定功率 kW</span>
        <input aria-label="额定功率" type="number" inputMode="decimal" value={rated} onChange={(event) => setRated(Number(event.target.value))} />
        <button className="primary-button" disabled={busy} onClick={() => void add()}>{busy ? "创建中…" : "创建"}</button>
      </div>
      <p className="form-note">创建后自动归属默认站点/电站，进入后可挂设备并配置能力级调度规则。</p>
    </div>}

    {units.length === 0 ? <div className="empty-state"><strong>还没有能源节点</strong><span>点击"新建能源节点"创建一个储能柜，或走接入向导绑定真实设备。</span></div> : <div className="energy-unit-list">
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

  return <div className="energy-rule-section">
    <div className="panel-heading"><div><strong>柜内设备</strong><p className="form-note">设备映射功率设定点位后，调度才能把功率请求写到真实设备。</p></div>
      <button className="secondary-button" onClick={() => setShowAdd(!showAdd)}>{showAdd ? "收起" : "+ 添加设备"}</button>
    </div>

    {devices.length === 0 ? <p className="form-note">还没有设备。添加一台 PCS 并映射功率点位，调度才能下发。</p> : devices.map((device) => (
      <div className="energy-rule-row" key={device.id}>
        <strong>{device.description || device.asset_code}</strong>
        <span className="set-label">额定 {device.rated_power_kw} kW</span>
        <span className="set-label">功率点位 {device.power_setpoint_id || "未映射"}</span>
      </div>
    ))}

    {showAdd && <div className="energy-unit-add">
      <div className="energy-rule-row">
        <input aria-label="设备名称" placeholder="例如：1 号 PCS" value={name} onChange={(event) => setName(event.target.value)} />
        <span className="set-label">额定 kW</span>
        <input aria-label="设备额定功率" type="number" inputMode="decimal" value={rated} onChange={(event) => setRated(Number(event.target.value))} />
      </div>
      <div className="energy-rule-row">
        <span className="set-label">功率设定点位</span>
        <select aria-label="功率设定点位" value={pointId} onChange={(event) => setPointId(event.target.value)}>
          <option value="">选择 modbus 点位</option>
          {powerPoints.map((node) => <option key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</option>)}
        </select>
        <button className="primary-button" disabled={busy} onClick={() => void add()}>{busy ? "添加中…" : "添加"}</button>
      </div>
      {powerPoints.length === 0 && <p className="form-note">还没有 modbus 点位，请先在"通道"页配置功率设定点位。</p>}
    </div>}
  </div>
}
