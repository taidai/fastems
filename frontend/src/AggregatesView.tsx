import { useCallback, useEffect, useMemo, useState } from "react"
import { nameOf, Node, point, request } from "./api"

type Op = "sum" | "avg" | "min" | "max" | "count"
type QualityPolicy = "ignore" | "count" | "degrade"

type Member = {
  node_id: string
  name: string
  value: number | null
  quality: string
  time?: string
}

type AlarmConfig = {
  enabled: boolean
  min?: number | null
  max?: number | null
  severity?: string
}

type Result = {
  configured: boolean
  config: { group: string; op: Op; members: string[]; units?: string; window?: string; quality?: string; alarm?: AlarmConfig }
  value: number | null
  quality: string
  partial: boolean
  valid: number
  total: number
  time?: string
  members: Member[]
  window?: { start: string; end: string; members: number }
  alarm?: { active: boolean; severity: string; detail?: string }
}

const opNames: Record<Op, string> = { sum: "求和", avg: "平均", min: "最小", max: "最大", count: "计数" }
const qualityNames: Record<string, string> = { good: "正常", partial: "部分有效", degraded: "质量降级", offline: "全部无效/过期", unconfigured: "未配置" }
const severityNames: Record<string, string> = { info: "提示", warning: "警告", critical: "严重" }

function qualityClass(quality: string) {
  if (quality === "good") return "good"
  if (quality === "partial" || quality === "degraded") return "warn"
  return "bad"
}

function numOrUndef(text: string): number | undefined {
  if (text.trim() === "") return undefined
  const value = Number(text)
  return Number.isFinite(value) ? value : undefined
}

export default function AggregatesView({ token, nodes, groups, canConfigure }: {
  token: string
  nodes: Node[]
  groups: Node[]
  canConfigure: boolean
}) {
  const points = useMemo(() => nodes.filter((node) => node.type === "variable" || node.type === "modbusIo"), [nodes])
  const [groupID, setGroupID] = useState(groups[0]?.id || "")
  const [op, setOp] = useState<Op>("sum")
  const [units, setUnits] = useState("")
  const [aggWindow, setAggWindow] = useState("")
  const [quality, setQuality] = useState<QualityPolicy>("count")
  const [alarmEnabled, setAlarmEnabled] = useState(false)
  const [alarmMin, setAlarmMin] = useState("")
  const [alarmMax, setAlarmMax] = useState("")
  const [alarmSeverity, setAlarmSeverity] = useState("warning")
  const [members, setMembers] = useState<string[]>([])
  const [result, setResult] = useState<Result | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  const group = groups.find((item) => item.id === groupID)

  const load = useCallback(async () => {
    if (!groupID) return
    try {
      const next = await request<Result>(token, `/aggregate?group=${encodeURIComponent(groupID)}`)
      setResult(next)
      setError("")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "聚合读取失败")
    }
  }, [token, groupID])

  useEffect(() => {
    const selected = groups.find((item) => item.id === groupID)
    if (!selected) return
    setResult(null)
    try {
      const saved = JSON.parse(point(selected, "easyemsAggregate")?.text || "{}")
      setOp(saved.op || "sum")
      setUnits(saved.units || "")
      setMembers(Array.isArray(saved.members) ? saved.members : [])
      setAggWindow(saved.window || "")
      setQuality(saved.quality === "ignore" || saved.quality === "degrade" ? saved.quality : "count")
      setAlarmEnabled(!!saved.alarm?.enabled)
      setAlarmMin(saved.alarm?.min != null ? String(saved.alarm.min) : "")
      setAlarmMax(saved.alarm?.max != null ? String(saved.alarm.max) : "")
      setAlarmSeverity(saved.alarm?.severity || "warning")
    } catch {
      setOp("sum")
      setUnits("")
      setMembers([])
      setAggWindow("")
      setQuality("count")
      setAlarmEnabled(false)
      setAlarmMin("")
      setAlarmMax("")
      setAlarmSeverity("warning")
    }
    void load()
    // Reload the form only when the selected group changes, not on every snapshot refresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [groupID, load])

  useEffect(() => {
    const timer = window.setInterval(() => void load(), 2000)
    return () => window.clearInterval(timer)
  }, [load])

  async function save() {
    if (!group) return
    if (alarmEnabled && alarmMin.trim() === "" && alarmMax.trim() === "") {
      setError("阈值告警需至少设置上限或下限")
      return
    }
    const alarm = alarmEnabled
      ? { enabled: true, min: numOrUndef(alarmMin), max: numOrUndef(alarmMax), severity: alarmSeverity }
      : undefined
    setBusy(true)
    setError("")
    try {
      const next = await request<Result>(token, "/aggregate", "PUT", { group: group.id, op, members, units, window: aggWindow, quality, alarm })
      setResult(next)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "聚合保存失败")
    } finally {
      setBusy(false)
    }
  }

  function toggleMember(id: string) {
    setMembers((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id])
  }

  return <section className="aggregate-page">
    <div className="aggregate-grid">
      <div className="panel aggregate-config">
        <div className="panel-heading"><div><span className="section-kicker">只读虚拟设备</span><h2>聚合配置</h2></div></div>
        <label>所属业务节点<select value={groupID} onChange={(event) => setGroupID(event.target.value)}>
          {groups.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}
        </select></label>
        <label>计算方式<select value={op} disabled={!canConfigure} onChange={(event) => setOp(event.target.value as Op)}>
          {(Object.keys(opNames) as Op[]).map((key) => <option key={key} value={key}>{opNames[key]}</option>)}
        </select></label>
        <label>单位<input value={units} disabled={!canConfigure} placeholder="如 kW / kWh" onChange={(event) => setUnits(event.target.value)} /></label>
        <label>时间窗口<select value={aggWindow} disabled={!canConfigure} onChange={(event) => setAggWindow(event.target.value)}>
          <option value="">瞬时值</option>
          <option value="5m">最近 5 分钟</option>
          <option value="15m">最近 15 分钟</option>
          <option value="1h">最近 1 小时</option>
        </select></label>
        <label>质量策略<select value={quality} disabled={!canConfigure} onChange={(event) => setQuality(event.target.value as QualityPolicy)}>
          <option value="count">按坏值计（部分有效）</option>
          <option value="ignore">忽略坏成员</option>
          <option value="degrade">整体降级</option>
        </select></label>
        <div className="aggregate-alarm-cfg">
          <label className="aggregate-alarm-switch">
            <input type="checkbox" disabled={!canConfigure} checked={alarmEnabled} onChange={(event) => setAlarmEnabled(event.target.checked)} />
            <span>阈值告警</span>
          </label>
          {alarmEnabled && <div className="aggregate-alarm-fields">
            <label>下限<input type="number" disabled={!canConfigure} value={alarmMin} placeholder="可选" onChange={(event) => setAlarmMin(event.target.value)} /></label>
            <label>上限<input type="number" disabled={!canConfigure} value={alarmMax} placeholder="可选" onChange={(event) => setAlarmMax(event.target.value)} /></label>
            <label>级别<select value={alarmSeverity} disabled={!canConfigure} onChange={(event) => setAlarmSeverity(event.target.value)}>
              <option value="info">提示</option><option value="warning">警告</option><option value="critical">严重</option>
            </select></label>
          </div>}
        </div>
        <div className="aggregate-pick-head"><span>成员点位</span><small>已选 {members.length} / {points.length}</small></div>
        <div className="aggregate-picker">
          {points.map((node) => <label key={node.id} className="aggregate-check">
            <input type="checkbox" disabled={!canConfigure} checked={members.includes(node.id)} onChange={() => toggleMember(node.id)} />
            <span>{nameOf(node)}</span><small>{point(node, "units")?.text || "—"} · {point(node, "value")?.value ?? "—"}</small>
          </label>)}
          {points.length === 0 && <div className="empty-state"><span>当前没有可聚合的点位。</span></div>}
        </div>
        {canConfigure && <div className="aggregate-actions"><button className="primary-button" disabled={busy || members.length === 0} onClick={() => void save()}>{busy ? "保存中…" : "保存聚合"}</button></div>}
        {!canConfigure && <p className="inline-note">当前账号只能查看聚合结果。</p>}
        {error && <div className="inline-error">{error}</div>}
      </div>

      <div className="panel aggregate-result">
        <div className="panel-heading"><div><span className="section-kicker">质量与时效随成员传播</span><h2>{group ? nameOf(group) : "聚合结果"}</h2></div>
          <div className="aggregate-badges">
            {result?.alarm?.active && <span className="aggregate-quality bad">告警 {severityNames[result.alarm.severity] || result.alarm.severity}</span>}
            {result?.configured && <span className={`aggregate-quality ${qualityClass(result.quality)}`}>{qualityNames[result.quality] || result.quality}</span>}
          </div></div>
        {result?.configured ? <>
          <div className="aggregate-value">
            <strong>{result.value === null ? "—" : result.value.toFixed(2)}</strong><span>{units || result.config.units || ""}</span>
          </div>
          <div className="aggregate-meta">
            <span>{opNames[result.config.op]} · 有效 {result.valid} / {result.total}</span>
            {result.time ? <span>{`最新 ${new Date(result.time).toLocaleTimeString("zh-CN", { hour12: false })}`}</span> : <span>无有效采样</span>}
            {result.window && <span className="aggregate-window">窗口 {new Date(result.window.start).toLocaleTimeString("zh-CN", { hour12: false })}–{new Date(result.window.end).toLocaleTimeString("zh-CN", { hour12: false })} · 参与 {result.window.members} 成员</span>}
            {result.partial && <em>部分成员无效或过期，结果仅覆盖有效成员。</em>}
            {result.quality === "degraded" && <em>存在坏成员，整体结果已降级。</em>}
            {result.alarm?.active && <em>聚合值已触发阈值告警：{result.alarm.detail || ""}</em>}
          </div>
          <div className="aggregate-members">
            {result.members.map((member) => <div key={member.node_id} className={`aggregate-member ${member.quality}`}>
              <span>{member.name}</span>
              <strong>{member.value === null ? "—" : member.value}</strong>
              <small>{member.quality === "good" ? "正常" : member.quality === "stale" ? "过期" : "无值"}</small>
            </div>)}
          </div>
        </> : <div className="empty-state large">
          <strong>{result?.configured === false ? "该节点还没有聚合配置" : "正在读取聚合…"}</strong>
          <span>选择成员点位并保存后，这里显示只读聚合值、质量与部分有效标记。</span>
        </div>}
      </div>
    </div>
  </section>
}
