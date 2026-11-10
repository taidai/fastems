import { useState } from "react"
import { EmsButton, EmsInput, EmsOverlay, EmsSelect, EmsSelectOption, InlineNotice } from "./components/ems"
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

  return <EmsOverlay open variant="drawer" title={`${nameOf(node)} 安全约束`} busy={busy} onClose={onClose}
    actions={<>
      <EmsButton variant="secondary" disabled={busy} onClick={onClose}>取消</EmsButton>
      <EmsButton busy={busy} busyLabel="保存中…" onClick={() => void save()}>保存安全约束</EmsButton>
    </>}>
    <div className="grid gap-4">
      <InlineNotice tone="info">配置后，规则请求先经过这里的范围、充放电上限和允许条件；条件不满足会阻止下发，超出范围会夹到安全值，原因写入最近一次求值。</InlineNotice>

      <div className="grid gap-3 rounded-xl border border-ems-line p-3">
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-ems-accent" checked={rangeOn} onChange={(event) => setRangeOn(event.target.checked)} /> 数值范围</label>
        {rangeOn && <div className="grid gap-3 sm:grid-cols-2"><EmsInput label="最小值" type="number" value={min} onIonInput={(event) => setMin(event.detail.value || "")} /><EmsInput label="最大值" type="number" value={max} onIonInput={(event) => setMax(event.detail.value || "")} /></div>}
      </div>

      <div className="grid gap-3 rounded-xl border border-ems-line p-3">
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-ems-accent" checked={dischargeOn} onChange={(event) => setDischargeOn(event.target.checked)} /> 放电功率上限</label>
        {dischargeOn && <EmsInput label="上限（正值）" type="number" value={discharge} onIonInput={(event) => setDischarge(event.detail.value || "")} />}
      </div>

      <div className="grid gap-3 rounded-xl border border-ems-line p-3">
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-ems-accent" checked={chargeOn} onChange={(event) => setChargeOn(event.target.checked)} /> 充电功率上限</label>
        {chargeOn && <EmsInput label="上限（正值，内部按负功率处理）" type="number" value={charge} onIonInput={(event) => setCharge(event.detail.value || "")} />}
      </div>

      <div className="grid gap-3 rounded-xl border border-ems-line p-3">
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-ems-accent" checked={rampOn} onChange={(event) => setRampOn(event.target.checked)} /> 爬坡限制</label>
        {rampOn && <EmsInput label="每秒最大变化（单位/秒）" type="number" value={ramp} onIonInput={(event) => setRamp(event.detail.value || "")} hint="按每秒最多变化该值逐步接近目标；仅对数值输出生效，0 或未启用表示直接下发。" />}
      </div>

      <div className="grid gap-3 rounded-xl border border-ems-line p-3">
        <div className="flex flex-wrap items-center justify-between gap-2"><strong className="text-sm">允许条件</strong><EmsButton variant="secondary" size="small" disabled={!points.length} onClick={addCondition}>+ 添加条件</EmsButton></div>
        <p className="m-0 text-xs text-ems-muted">例如 SOC ≥ 10、温度 ≤ 45、PCS 状态 = 运行、BMS 允许功率 &gt; 0。点位无值或过期即视为不满足。</p>
        {conditions.map((condition, index) => {
          const kind = condition.kind || "number"
          return <div className="flex flex-wrap items-end gap-2" key={index}>
            <EmsSelect label="点位" aria-label="安全条件点位" value={condition.point_id} onIonChange={(event) => updateCondition(index, { point_id: event.detail.value })}>
              {points.map((item) => <EmsSelectOption key={item.id} value={item.id}>{nameOf(item)}</EmsSelectOption>)}
            </EmsSelect>
            <EmsSelect label="类型" aria-label="安全条件类型" value={kind} onIonChange={(event) => {
              const next = event.detail.value as Kind
              updateCondition(index, { kind: next, operator: next === "number" ? condition.operator : "eq", value: next === "bool" ? 0 : condition.value, text: next === "enum" ? (condition.text || "") : "" })
            }}><EmsSelectOption value="number">数值</EmsSelectOption><EmsSelectOption value="bool">开关</EmsSelectOption><EmsSelectOption value="enum">枚举</EmsSelectOption></EmsSelect>
            <EmsSelect label="比较" aria-label="安全条件比较" value={condition.operator} onIonChange={(event) => updateCondition(index, { operator: event.detail.value })}>
              {operatorsFor(kind).map(([value, label]) => <EmsSelectOption key={value} value={value}>{label}</EmsSelectOption>)}
            </EmsSelect>
            {kind === "bool"
              ? <EmsSelect label="值" aria-label="安全条件值" value={String(condition.value ? 1 : 0)} onIonChange={(event) => updateCondition(index, { value: Number(event.detail.value) })}><EmsSelectOption value="1">是</EmsSelectOption><EmsSelectOption value="0">否</EmsSelectOption></EmsSelect>
              : kind === "enum"
                ? <EmsInput label="枚举值" aria-label="安全枚举值" value={condition.text || ""} placeholder="如 running" onIonInput={(event) => updateCondition(index, { text: event.detail.value || "" })} />
                : <EmsInput label="值" aria-label="安全条件值" type="number" value={String(condition.value)} onIonInput={(event) => updateCondition(index, { value: Number(event.detail.value) })} />}
            <EmsButton variant="quiet" aria-label="删除安全条件" onClick={() => setConditions((current) => current.filter((_, i) => i !== index))}>×</EmsButton>
          </div>
        })}
        {conditions.length === 0 && <p className="m-0 text-xs text-ems-muted">未配置允许条件；只应用范围/功率上限。</p>}
      </div>

      {error && <InlineNotice tone="danger">{error}</InlineNotice>}
    </div>
  </EmsOverlay>
}
