import { useEffect, useState } from "react"
import { fetchEnergyUnits } from "./api"
import type { EnergyUnitSummary } from "./api"
import EnergyRuleEditor from "./EnergyRuleEditor"

// EnergyUnitsView 列出能源节点（EnergyUnit），点击进入能力级规则编辑器。
// 这是能力级模型对用户可见的入口，挂在设备台账/调度页下。
export default function EnergyUnitsView({ token, onError }: {
  token: string
  onError: (message: string) => void
}) {
  const [units, setUnits] = useState<EnergyUnitSummary[]>([])
  const [selected, setSelected] = useState<string>("")

  useEffect(() => {
    void fetchEnergyUnits(token)
      .then((result) => setUnits(result.units || []))
      .catch(() => onError("能源节点列表读取失败"))
  }, [token])

  if (selected) {
    return <section className="panel energy-units-view">
      <div className="panel-heading"><div><span className="section-kicker">能源节点</span><h2>能力级规则</h2><p>{units.find((unit) => unit.id === selected)?.asset_code || selected}</p></div><button className="secondary-button" onClick={() => setSelected("")}>返回列表</button></div>
      <EnergyRuleEditor token={token} energyUnitId={selected} onError={onError} />
    </section>
  }

  return <section className="panel energy-units-view">
    <div className="panel-heading"><div><span className="section-kicker">能源节点</span><h2>能源单元</h2><p>一个柜体（能源单元）是一条能力级规则与分配的调度对象。</p></div></div>
    {units.length === 0 ? <div className="empty-state"><strong>还没有能源节点</strong><span>能源节点（EnergyUnit）由接入向导创建；绑定 Profile 后即可配置能力级规则。</span></div> : <div className="energy-unit-list">
      {units.map((unit) => <button key={unit.id} className="energy-unit-row" onClick={() => setSelected(unit.id)}>
        <strong>{unit.asset_code || unit.description || unit.id}</strong>
        <span>{unit.description || unit.id}</span>
        <span aria-hidden="true">→</span>
      </button>)}
    </div>}
  </section>
}
