import { useState } from "react"
import { createPortal } from "react-dom"
import { nameOf, Node, point, request } from "./api"

type Kind = "number" | "bool" | "enum"
type SafetyCondition = { point_id: string; operator: string; kind?: Kind; value: number; text?: string }
type SafetyConfig = {
  min?: number
  max?: number
  max_discharge?: number
  max_charge?: number
  ramp_per_second?: number
  conditions?: SafetyCondition[]
}

const numericOperators = [
  ["gt", "大于"], ["ge", "大于或等于"], ["eq", "等于"],
  ["ne", "不等于"], ["le", "小于或等于"], ["lt", "小于"],
]
const switchOperators = [["eq", "等于"], ["ne", "不等于"]]

function operatorsFor(kind: Kind) {
  return kind === "number" ? numericOperators : switchOperators
}

function readConfig(node: Node): SafetyConfig {
  try {
    return JSON.parse(point(node, "easyemsSafety")?.text || "{}")
  } catch {
    return {}
  }
}

export default function SafetyModal({ token, node, nodes, onClose, onSaved }: {
  token: string
  node: Node
  nodes: Node[]
  onClose: () => void
  onSaved: () => void
}) {
  const saved = readConfig(node)
  const points = nodes.filter((item) => item.type === "variable" || item.type === "modbusIo")
  const [rangeOn, setRangeOn] = useState(saved.min !== undefined || saved.max !== undefined)
  const [min, setMin] = useState(saved.min === undefined ? "" : String(saved.min))
  const [max, setMax] = useState(saved.max === undefined ? "" : String(saved.max))
  const [dischargeOn, setDischargeOn] = useState(saved.max_discharge !== undefined)
  const [discharge, setDischarge] = useState(saved.max_discharge === undefined ? "" : String(saved.max_discharge))
  const [chargeOn, setChargeOn] = useState(saved.max_charge !== undefined)
  const [charge, setCharge] = useState(saved.max_charge === undefined ? "" : String(saved.max_charge))
  const [rampOn, setRampOn] = useState(saved.ramp_per_second !== undefined)
  const [ramp, setRamp] = useState(saved.ramp_per_second === undefined ? "" : String(saved.ramp_per_second))
  const [conditions, setConditions] = useState<SafetyCondition[]>(saved.conditions || [])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  function updateCondition(index: number, patch: Partial<SafetyCondition>) {
    setConditions((current) => current.map((item, i) => i === index ? { ...item, ...patch } : item))
  }

  function addCondition() {
    const first = points[0]
    setConditions((current) => [...current, { point_id: first?.id || "", operator: "gt", kind: "number", value: 0 }])
  }

  async function save() {
    setError("")
    const config: SafetyConfig = { conditions }
    if (rangeOn) {
      const lo = Number(min), hi = Number(max)
      if (min.trim() === "" || max.trim() === "" || !Number.isFinite(lo) || !Number.isFinite(hi) || lo >= hi) {
        setError("范围需要同时填写最小值和最大值，且最小值必须小于最大值。")
        return
      }
      config.min = lo
      config.max = hi
    }
    if (dischargeOn) {
      const value = Number(discharge)
      if (!Number.isFinite(value) || value < 0) { setError("放电上限必须是非负数字。"); return }
      config.max_discharge = value
    }
    if (chargeOn) {
      const value = Number(charge)
      if (!Number.isFinite(value) || value < 0) { setError("充电上限必须是非负数字。"); return }
      config.max_charge = value
    }
    if (rampOn) {
      const value = Number(ramp)
      if (!Number.isFinite(value) || value < 0) { setError("爬坡速度必须是非负数字。"); return }
      config.ramp_per_second = value
    }
    for (const condition of conditions) {
      if (!condition.point_id) { setError("每条允许条件都需要选择点位。"); return }
      if (condition.kind === "enum" && !condition.text?.trim()) { setError("枚举条件需要填写标签。"); return }
    }
    setBusy(true)
    try {
      await request(token, "/safety", "PUT", { node: node.id, safety: config })
      onSaved()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "安全约束保存失败")
    } finally {
      setBusy(false)
    }
  }

  return createPortal(<>
    <div className="safety-backdrop" onClick={onClose} />
    <section className="safety-modal" role="dialog" aria-label="安全约束">
      <div className="safety-head"><div><span className="section-kicker">普通规则输出统一拦截</span><h2>{nameOf(node)} 安全约束</h2></div><button className="icon-button" aria-label="关闭安全约束" onClick={onClose}>×</button></div>
      <p className="safety-intro">配置后，规则请求先经过这里的范围、充放电上限和允许条件；条件不满足会阻止下发，超出范围会夹到安全值，原因写入最近一次求值。</p>

      <div className="safety-section">
        <label className="safety-toggle"><input type="checkbox" checked={rangeOn} onChange={(event) => setRangeOn(event.target.checked)} /> 数值范围</label>
        {rangeOn && <div className="safety-row"><label>最小值<input type="number" step="any" value={min} onChange={(event) => setMin(event.target.value)} /></label><label>最大值<input type="number" step="any" value={max} onChange={(event) => setMax(event.target.value)} /></label></div>}
      </div>

      <div className="safety-section">
        <label className="safety-toggle"><input type="checkbox" checked={dischargeOn} onChange={(event) => setDischargeOn(event.target.checked)} /> 放电功率上限</label>
        {dischargeOn && <div className="safety-row"><label>上限（正值）<input type="number" min="0" step="any" value={discharge} onChange={(event) => setDischarge(event.target.value)} /></label></div>}
      </div>

      <div className="safety-section">
        <label className="safety-toggle"><input type="checkbox" checked={chargeOn} onChange={(event) => setChargeOn(event.target.checked)} /> 充电功率上限</label>
        {chargeOn && <div className="safety-row"><label>上限（正值，内部按负功率处理）<input type="number" min="0" step="any" value={charge} onChange={(event) => setCharge(event.target.value)} /></label></div>}
      </div>

      <div className="safety-section">
        <label className="safety-toggle"><input type="checkbox" checked={rampOn} onChange={(event) => setRampOn(event.target.checked)} /> 爬坡限制</label>
        {rampOn && <div className="safety-row"><label>每秒最大变化（单位/秒）<input type="number" min="0" step="any" value={ramp} onChange={(event) => setRamp(event.target.value)} /></label></div>}
        <small>按每秒最多变化该值逐步接近目标；仅对数值输出生效，0 或未启用表示直接下发。</small>
      </div>

      <div className="safety-section">
        <div className="safety-conditions-head"><strong>允许条件</strong><button className="secondary-button" disabled={!points.length} onClick={addCondition}>+ 添加条件</button></div>
        <small>例如 SOC ≥ 10、温度 ≤ 45、PCS 状态 = 运行、BMS 允许功率 &gt; 0。点位无值或过期即视为不满足。</small>
        {conditions.map((condition, index) => {
          const kind = condition.kind || "number"
          return <div className="safety-condition" key={index}>
            <select aria-label="安全条件点位" value={condition.point_id} onChange={(event) => updateCondition(index, { point_id: event.target.value })}>
              {points.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}
            </select>
            <select aria-label="安全条件类型" value={kind} onChange={(event) => {
              const next = event.target.value as Kind
              updateCondition(index, { kind: next, operator: next === "number" ? condition.operator : "eq", value: next === "bool" ? 0 : condition.value, text: next === "enum" ? (condition.text || "") : "" })
            }}><option value="number">数值</option><option value="bool">开关</option><option value="enum">枚举</option></select>
            <select aria-label="安全条件比较" value={condition.operator} onChange={(event) => updateCondition(index, { operator: event.target.value })}>
              {operatorsFor(kind).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
            {kind === "bool"
              ? <select aria-label="安全条件值" value={condition.value ? 1 : 0} onChange={(event) => updateCondition(index, { value: Number(event.target.value) })}><option value={1}>是</option><option value={0}>否</option></select>
              : kind === "enum"
                ? <input aria-label="安全枚举值" value={condition.text || ""} placeholder="如 running" onChange={(event) => updateCondition(index, { text: event.target.value })} />
                : <input aria-label="安全条件值" type="number" step="any" value={condition.value} onChange={(event) => updateCondition(index, { value: Number(event.target.value) })} />}
            <button className="icon-button" aria-label="删除安全条件" onClick={() => setConditions((current) => current.filter((_, i) => i !== index))}>×</button>
          </div>
        })}
        {conditions.length === 0 && <div className="safety-empty">未配置允许条件；只应用范围/功率上限。</div>}
      </div>

      {error && <div className="inline-error">{error}</div>}
      <div className="safety-actions"><button className="secondary-button" onClick={onClose}>取消</button><button className="primary-button" disabled={busy} onClick={() => void save()}>{busy ? "保存中…" : "保存安全约束"}</button></div>
    </section>
  </>, document.body)
}
