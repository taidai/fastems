import { useCallback, useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { nameOf, Node, request } from "./api"

type Severity = "critical" | "warning" | "info"
type Trigger = "gt" | "lt" | "eq" | "ne" | "offline"

type AlarmType = {
  code: string
  name: string
  group?: string
  severity: Severity
  prompt?: string
  trigger: Trigger
  point_id: string
  value?: number
  delay_seconds: number
  confirm: "manual" | "auto"
  record_recovery: boolean
  enabled: boolean
}

const emptyType: AlarmType = {
  code: "",
  name: "",
  group: "",
  severity: "warning",
  prompt: "声音+弹窗",
  trigger: "gt",
  point_id: "",
  value: 0,
  delay_seconds: 0,
  confirm: "manual",
  record_recovery: true,
  enabled: true,
}

const severityNames: Record<Severity, string> = { critical: "严重", warning: "重要", info: "提示" }
const triggerNames: Record<Trigger, string> = { gt: "大于", lt: "小于", eq: "等于", ne: "不等于", offline: "离线" }

function triggerSummary(type: AlarmType) {
  if (type.trigger === "offline") return "离线"
  return `${triggerNames[type.trigger]} ${type.value ?? 0}`
}

export default function AlarmTypesPanel({ token, rootID, nodes, canConfigure, onError }: {
  token: string
  rootID: string
  nodes: Node[]
  canConfigure: boolean
  onError: (message: string) => void
}) {
  const [types, setTypes] = useState<AlarmType[]>([])
  const [editing, setEditing] = useState<AlarmType | null>(null)
  const [originalCode, setOriginalCode] = useState("")
  const [busy, setBusy] = useState(false)
  const points = useMemo(() => nodes.filter((node) => node.type === "variable" || node.type === "modbusIo"), [nodes])
  const channels = useMemo(() => nodes.filter((node) => node.type === "modbus"), [nodes])
  const names = useMemo(() => new Map(nodes.map((node) => [node.id, nameOf(node)])), [nodes])

  const load = useCallback(async () => {
    try {
      const result = await request<{ types: AlarmType[] }>(token, `/alarm-types?root=${encodeURIComponent(rootID)}`)
      setTypes(result.types || [])
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "告警类型读取失败")
    }
  }, [token, rootID, onError])

  useEffect(() => { void load() }, [load])

  function add() {
    setOriginalCode("")
    setEditing({ ...emptyType, point_id: points[0]?.id || "" })
  }

  function edit(type: AlarmType) {
    setOriginalCode(type.code)
    setEditing({ ...type })
  }

  async function save() {
    if (!editing) return
    if (!editing.name.trim()) { onError("告警类型名称不能为空"); return }
    if (!/^[A-Za-z0-9_-]{2,32}$/.test(editing.code)) { onError("编码需为 2–32 位字母、数字、下划线或短横线"); return }
    if (!editing.point_id) { onError("请选择触发点位"); return }
    setBusy(true)
    try {
      await request(token, `/alarm-types?root=${encodeURIComponent(rootID)}`, "PUT", { root: rootID, type: editing })
      setEditing(null)
      await load()
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "告警类型保存失败")
    } finally {
      setBusy(false)
    }
  }

  async function remove(type: AlarmType) {
    if (!window.confirm(`确定删除“${type.name}”吗？活动告警会被恢复并关闭。`)) return
    setBusy(true)
    try {
      await request(token, `/alarm-types?root=${encodeURIComponent(rootID)}&code=${encodeURIComponent(type.code)}`, "DELETE")
      await load()
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "告警类型删除失败")
    } finally {
      setBusy(false)
    }
  }

  return <section className="alarm-types-panel">
    <div className="alarm-types-head">
      <div><span className="section-kicker">可配置阈值与延时</span><h2>告警类型</h2><p>类型只读点位并生成告警；采集、规则与命令的固定告警仍然保留。</p></div>
      {canConfigure && <button className="primary-button" onClick={add}>+ 新增类型</button>}
    </div>
    <div className="alarm-type-table-wrap">
      <table className="alarm-type-table">
        <thead><tr><th>#</th><th>告警类型</th><th>分组</th><th>级别</th><th>触发条件</th><th>延时</th><th>自动确认</th><th>记录复归</th><th>编码</th><th>操作</th></tr></thead>
        <tbody>
          {types.map((type, index) => <tr key={type.code} className={!type.enabled ? "disabled" : ""}>
            <td>{index + 1}</td>
            <td><strong>{type.name}</strong></td>
            <td>{type.group || "—"}</td>
            <td><span className={`alarm-type-severity ${type.severity}`}>{severityNames[type.severity]}</span></td>
            <td>{names.get(type.point_id) || type.point_id} · {triggerSummary(type)}</td>
            <td>{type.delay_seconds} 秒</td>
            <td>{type.confirm === "auto" ? "是" : "否"}</td>
            <td>{type.record_recovery ? "是" : "否"}</td>
            <td><code>{type.code}</code></td>
            <td className="alarm-type-actions">
              {canConfigure && <button onClick={() => edit(type)}>编辑</button>}
              {canConfigure && <button className="danger-text" disabled={busy} onClick={() => void remove(type)}>删除</button>}
            </td>
          </tr>)}
          {types.length === 0 && <tr><td colSpan={10}><div className="empty-state"><strong>还没有自定义告警类型</strong><span>新增后按点位阈值或通道离线生成告警；没有配置时只保留固定告警。</span></div></td></tr>}
        </tbody>
      </table>
    </div>

    {editing && createPortal(<>
      <div className="alarm-type-backdrop" onClick={() => setEditing(null)} />
      <section className="alarm-type-modal" role="dialog" aria-label="告警类型">
        <div className="alarm-type-modal-head"><div><span className="section-kicker">告警类型</span><h2>{originalCode ? "编辑告警类型" : "新增告警类型"}</h2></div><button className="icon-button" aria-label="关闭" onClick={() => setEditing(null)}>×</button></div>
        <div className="alarm-type-grid">
          <label>告警类型<input value={editing.name} onChange={(event) => setEditing({ ...editing, name: event.target.value })} placeholder="如 电池温度过高" /></label>
          <label>编码<input value={editing.code} disabled={Boolean(originalCode)} onChange={(event) => setEditing({ ...editing, code: event.target.value.trim() })} placeholder="如 BMS_TEMP_HIGH" /></label>
          <label>类型分组<input value={editing.group || ""} onChange={(event) => setEditing({ ...editing, group: event.target.value })} placeholder="如 电池系统" /></label>
          <label>告警级别<select value={editing.severity} onChange={(event) => setEditing({ ...editing, severity: event.target.value as Severity })}><option value="critical">严重</option><option value="warning">重要</option><option value="info">提示</option></select></label>
          <label>提示方式<select value={editing.prompt || "声音+弹窗"} onChange={(event) => setEditing({ ...editing, prompt: event.target.value })}><option>声音+弹窗</option><option>弹窗</option><option>仅记录</option></select></label>
          <label>触发类型<select value={editing.trigger} onChange={(event) => {
            const trigger = event.target.value as Trigger
            setEditing({ ...editing, trigger, point_id: trigger === "offline" ? (channels[0]?.id || "") : (points[0]?.id || "") })
          }}><option value="gt">大于</option><option value="lt">小于</option><option value="eq">等于</option><option value="ne">不等于</option><option value="offline">离线</option></select></label>
          <label>{editing.trigger === "offline" ? "通道" : "触发点位"}<select value={editing.point_id} onChange={(event) => setEditing({ ...editing, point_id: event.target.value })}>
            {(editing.trigger === "offline" ? channels : points).map((node) => <option key={node.id} value={node.id}>{nameOf(node)}</option>)}
          </select></label>
          {editing.trigger !== "offline" && <label>阈值<input type="number" step="any" value={editing.value ?? 0} onChange={(event) => setEditing({ ...editing, value: Number(event.target.value) })} /></label>}
          <label>触发延时（秒）<input type="number" min="0" max="86400" value={editing.delay_seconds} onChange={(event) => setEditing({ ...editing, delay_seconds: Math.max(0, Number(event.target.value) || 0) })} /></label>
          <label>确认方式<select value={editing.confirm} onChange={(event) => setEditing({ ...editing, confirm: event.target.value as "manual" | "auto" })}><option value="manual">手动确认</option><option value="auto">自动确认</option></select></label>
          <label className="alarm-type-check"><input type="checkbox" checked={editing.record_recovery} onChange={(event) => setEditing({ ...editing, record_recovery: event.target.checked })} /> 记录复归</label>
          <label className="alarm-type-check"><input type="checkbox" checked={editing.enabled} onChange={(event) => setEditing({ ...editing, enabled: event.target.checked })} /> 启用</label>
        </div>
        <div className="alarm-type-modal-actions"><button className="secondary-button" onClick={() => setEditing(null)}>取消</button><button className="primary-button" disabled={busy} onClick={() => void save()}>{busy ? "保存中…" : "保存"}</button></div>
      </section>
    </>, document.body)}
  </section>
}
