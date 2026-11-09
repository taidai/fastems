import { useEffect, useState } from "react"
import { fetchEnergyUnits, createEnergyUnit } from "./api"
import type { EnergyUnitSummary } from "./api"
import EnergyRuleEditor from "./EnergyRuleEditor"

// EnergyUnitsView 列出能源节点（EnergyUnit），点击进入能力级规则编辑器。
// 支持用户自建能源节点——不再只有 seed 预置的演示站。
export default function EnergyUnitsView({ token, onError }: {
  token: string
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
      <p className="form-note">创建后自动归属默认站点/电站，可在下方配置能力级调度规则。</p>
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
