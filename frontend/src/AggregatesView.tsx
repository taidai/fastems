import { useCallback, useEffect, useMemo, useState } from "react"
import { nameOf, Node, point, request } from "./api"

type Op = "sum" | "avg" | "min" | "max" | "count"

type Member = {
  node_id: string
  name: string
  value: number | null
  quality: string
  time?: string
}

type Result = {
  configured: boolean
  config: { group: string; op: Op; members: string[]; units?: string }
  value: number | null
  quality: string
  partial: boolean
  valid: number
  total: number
  time?: string
  members: Member[]
}

const opNames: Record<Op, string> = { sum: "求和", avg: "平均", min: "最小", max: "最大", count: "计数" }
const qualityNames: Record<string, string> = { good: "正常", partial: "部分有效", offline: "全部无效/过期", unconfigured: "未配置" }

function qualityClass(quality: string) {
  if (quality === "good") return "good"
  if (quality === "partial") return "warn"
  return "bad"
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
    } catch {
      setOp("sum")
      setUnits("")
      setMembers([])
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
    setBusy(true)
    setError("")
    try {
      const next = await request<Result>(token, "/aggregate", "PUT", { group: group.id, op, members, units })
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
          {result?.configured && <span className={`aggregate-quality ${qualityClass(result.quality)}`}>{qualityNames[result.quality] || result.quality}</span>}</div>
        {result?.configured ? <>
          <div className="aggregate-value">
            <strong>{result.value === null ? "—" : result.value.toFixed(2)}</strong><span>{units || result.config.units || ""}</span>
          </div>
          <div className="aggregate-meta">
            <span>{opNames[result.config.op]} · 有效 {result.valid} / {result.total}</span>
            <span>{result.time ? `最新 ${new Date(result.time).toLocaleTimeString("zh-CN", { hour12: false })}` : "无有效采样"}</span>
            {result.partial && <em>部分成员无效或过期，结果仅覆盖有效成员。</em>}
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
