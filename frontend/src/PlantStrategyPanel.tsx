import { useEffect, useState } from "react"
import { fetchPlants, fetchPlantPCC, savePlantPCC } from "./api"
import type { Node } from "./api"
import ConfirmDialog from "./ConfirmDialog"

// PlantStrategyPanel 配置电站级策略（防逆流/需量）：选 PCC 主表点位 +
// 反送/需量上限，绑定后 EnergyScheduler 每周期自动闭环。与单柜规则分离。
export default function PlantStrategyPanel({ token, nodes, onError }: {
  token: string
  nodes: Node[]
  onError: (message: string) => void
}) {
  const [plantId, setPlantId] = useState("")
  const [meterId, setMeterId] = useState("")
  const [reverseLimit, setReverseLimit] = useState(0)
  const [demandLimit, setDemandLimit] = useState(0)
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const meters = nodes.filter((node) => node.type === "variable" || node.type === "modbusIo")

  useEffect(() => {
    void fetchPlants(token).then((result) => {
      const first = result.plants?.[0]
      if (first) setPlantId(first.id)
    }).catch(() => onError("电站列表读取失败"))
  }, [token])

  useEffect(() => {
    if (!plantId) return
    void fetchPlantPCC(token, plantId).then((result) => {
      if (result.configured && result.binding) {
        setMeterId(result.binding.MeterPointID)
        setReverseLimit(result.binding.LowerLimitKW)
        setDemandLimit(result.binding.UpperLimitKW)
        setSaved(true)
      }
    }).catch(() => {})
  }, [token, plantId])

  async function save() {
    if (!plantId) { onError("未选择电站"); return }
    if (!meterId) { onError("请选择 PCC 主表点位"); return }
    setBusy(true)
    try {
      await savePlantPCC(token, plantId, { MeterPointID: meterId, Sign: 1, MaxAgeMS: 15000, UpperLimitKW: demandLimit, LowerLimitKW: reverseLimit })
      setSaved(true)
    } catch (error) {
      onError(error instanceof Error ? error.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  return <div className="energy-rule-section">
    <div className="panel-heading"><div><strong>电站策略（防逆流 / 需量）</strong><p className="form-note">选并网点主表，设上限，保存后每秒自动闭环。{saved && "已启用。"}</p></div></div>
    <div className="energy-rule-row">
      <span className="set-label">PCC 主表</span>
      <select aria-label="PCC 主表点位" value={meterId} onChange={(event) => setMeterId(event.target.value)}>
        <option value="">选择关口表点位</option>
        {meters.map((node) => <option key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</option>)}
      </select>
    </div>
    <div className="energy-rule-row">
      <span className="set-label">反送上限 kW（防逆流，0=禁止反送）</span>
      <input aria-label="反送上限" type="number" inputMode="decimal" value={reverseLimit} onChange={(event) => setReverseLimit(Number(event.target.value))} />
      <span className="set-label">需量上限 kW（0=不限）</span>
      <input aria-label="需量上限" type="number" inputMode="decimal" value={demandLimit} onChange={(event) => setDemandLimit(Number(event.target.value))} />
      <button className="primary-button" disabled={busy} onClick={() => {
        if (!plantId) { onError("未选择电站"); return }
        if (!meterId) { onError("请选择 PCC 主表点位"); return }
        setConfirm(true)
      }}>{busy ? "保存中…" : saved ? "更新策略" : "启用策略"}</button>
    </div>

    {confirm && <ConfirmDialog
      title={saved ? "更新电站策略" : "启用电站策略"}
      message={`启用后，该电站将按反送上限 ${reverseLimit} kW、需量上限 ${demandLimit || "不限"} kW 每秒自动调节储能充放。确认${saved ? "更新" : "启用"}？`}
      confirmLabel="确认启用"
      busy={busy}
      onCancel={() => setConfirm(false)}
      onConfirm={() => { setConfirm(false); void save() }}
    />}
  </div>
}
