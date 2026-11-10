import { useCallback, useEffect, useMemo, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { nameOf, Node, request } from "./api"
import { EmsButton, EmsCard, EmsInput, EmsOverlay, EmsRecordList, EmsSectionHeader, EmsSelect, EmsSelectOption, EmptyState, StatusBadge } from "./components/ems"

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
  const closeEditor = useCallback(() => setEditing(null), [])
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

  function renderActions(type: AlarmType) {
    if (!canConfigure) return <span className="text-xs text-ems-muted">只读</span>
    return <div className="flex flex-wrap gap-1">
      <EmsButton variant="quiet" size="small" onClick={() => edit(type)}>编辑</EmsButton>
      <EmsButton variant="danger" size="small" disabled={busy} onClick={() => void remove(type)}>删除</EmsButton>
    </div>
  }

  return <section className="grid min-w-0 gap-4">
    <EmsSectionHeader kicker="可配置阈值与延时" title="告警类型" description="类型只读点位并生成告警；采集、规则与命令的固定告警仍然保留。"
      actions={canConfigure ? <EmsButton size="small" onClick={add}>+ 新增类型</EmsButton> : undefined} />
    <EmsCard><IonCardContent className="min-w-0">
      <EmsRecordList rows={types} keyOf={(type) => type.code}
        empty={<EmptyState title="还没有自定义告警类型" description="新增后按点位阈值或通道离线生成告警；没有配置时只保留固定告警。" />}
        columns={[
          { key: "index", label: "#", render: (type) => types.indexOf(type) + 1 },
          { key: "name", label: "告警类型", render: (type) => <div className="flex flex-wrap items-center gap-2"><strong>{type.name}</strong>{!type.enabled && <StatusBadge tone="neutral" dot={false}>已停用</StatusBadge>}</div> },
          { key: "group", label: "分组", render: (type) => type.group || "—" },
          { key: "severity", label: "级别", render: (type) => <StatusBadge tone={type.severity === "critical" ? "danger" : type.severity === "warning" ? "warning" : "info"}>{severityNames[type.severity]}</StatusBadge> },
          { key: "trigger", label: "触发条件", render: (type) => <span className="break-words">{names.get(type.point_id) || type.point_id} · {triggerSummary(type)}</span> },
          { key: "delay", label: "延时", render: (type) => `${type.delay_seconds} 秒` },
          { key: "confirm", label: "自动确认", render: (type) => type.confirm === "auto" ? "是" : "否" },
          { key: "recovery", label: "记录复归", render: (type) => type.record_recovery ? "是" : "否" },
          { key: "code", label: "编码", render: (type) => <code className="break-all text-xs text-ems-muted">{type.code}</code> },
          { key: "actions", label: "操作", render: renderActions },
        ]}
        mobileCard={(type) => <div className="grid min-w-0 gap-3">
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
            <strong className="min-w-0 break-words">{type.name}</strong>
            <StatusBadge tone={type.severity === "critical" ? "danger" : type.severity === "warning" ? "warning" : "info"}>{severityNames[type.severity]}</StatusBadge>
          </div>
          <div className="flex flex-wrap gap-2">
            {!type.enabled && <StatusBadge tone="neutral" dot={false}>已停用</StatusBadge>}
            {type.group && <StatusBadge tone="neutral" dot={false}>{type.group}</StatusBadge>}
          </div>
          <p className="m-0 break-words text-xs text-ems-ink">{names.get(type.point_id) || type.point_id} · {triggerSummary(type)}</p>
          <p className="m-0 text-xs text-ems-muted">延时 {type.delay_seconds} 秒 · 自动确认 {type.confirm === "auto" ? "是" : "否"} · 记录复归 {type.record_recovery ? "是" : "否"}</p>
          <code className="break-all text-xs text-ems-muted">{type.code}</code>
          {renderActions(type)}
        </div>}
      />
    </IonCardContent></EmsCard>

    <EmsOverlay open={Boolean(editing)} title={originalCode ? "编辑告警类型" : "新增告警类型"} busy={busy} onClose={closeEditor}
      actions={<>
        <EmsButton variant="secondary" disabled={busy} onClick={closeEditor}>取消</EmsButton>
        <EmsButton busy={busy} busyLabel="保存中…" onClick={() => void save()}>保存</EmsButton>
      </>}>
      {editing && <div className="alarm-type-grid !mt-0">
        <EmsInput label="告警类型" aria-label="告警类型" value={editing.name} onIonInput={(event) => setEditing({ ...editing, name: event.detail.value || "" })} placeholder="如 电池温度过高" />
        <EmsInput label="编码" aria-label="编码" value={editing.code} disabled={Boolean(originalCode)} onIonInput={(event) => setEditing({ ...editing, code: (event.detail.value || "").trim() })} placeholder="如 BMS_TEMP_HIGH" />
        <EmsInput label="类型分组" aria-label="类型分组" value={editing.group || ""} onIonInput={(event) => setEditing({ ...editing, group: event.detail.value || "" })} placeholder="如 电池系统" />
        <EmsSelect label="告警级别" aria-label="告警级别" value={editing.severity} onIonChange={(event) => setEditing({ ...editing, severity: event.detail.value as Severity })}>
          <EmsSelectOption value="critical">严重</EmsSelectOption><EmsSelectOption value="warning">重要</EmsSelectOption><EmsSelectOption value="info">提示</EmsSelectOption>
        </EmsSelect>
        <EmsSelect label="提示方式" aria-label="提示方式" value={editing.prompt || "声音+弹窗"} onIonChange={(event) => setEditing({ ...editing, prompt: event.detail.value })}>
          <EmsSelectOption value="声音+弹窗">声音+弹窗</EmsSelectOption><EmsSelectOption value="弹窗">弹窗</EmsSelectOption><EmsSelectOption value="仅记录">仅记录</EmsSelectOption>
        </EmsSelect>
        <EmsSelect label="触发类型" aria-label="触发类型" value={editing.trigger} onIonChange={(event) => {
          const trigger = event.detail.value as Trigger
          setEditing({ ...editing, trigger, point_id: trigger === "offline" ? (channels[0]?.id || "") : (points[0]?.id || "") })
        }}>
          <EmsSelectOption value="gt">大于</EmsSelectOption><EmsSelectOption value="lt">小于</EmsSelectOption><EmsSelectOption value="eq">等于</EmsSelectOption><EmsSelectOption value="ne">不等于</EmsSelectOption><EmsSelectOption value="offline">离线</EmsSelectOption>
        </EmsSelect>
        <EmsSelect label={editing.trigger === "offline" ? "通道" : "触发点位"} aria-label={editing.trigger === "offline" ? "通道" : "触发点位"} value={editing.point_id} onIonChange={(event) => setEditing({ ...editing, point_id: event.detail.value })}>
          {(editing.trigger === "offline" ? channels : points).map((node) => <EmsSelectOption key={node.id} value={node.id}>{nameOf(node)}</EmsSelectOption>)}
        </EmsSelect>
        {editing.trigger !== "offline" && <EmsInput label="阈值" aria-label="阈值" type="number" step="any" value={editing.value ?? 0} onIonInput={(event) => setEditing({ ...editing, value: Number(event.detail.value) })} />}
        <EmsInput label="触发延时（秒）" aria-label="触发延时（秒）" type="number" min={0} max={86400} value={editing.delay_seconds} onIonInput={(event) => setEditing({ ...editing, delay_seconds: Math.max(0, Number(event.detail.value) || 0) })} />
        <EmsSelect label="确认方式" aria-label="确认方式" value={editing.confirm} onIonChange={(event) => setEditing({ ...editing, confirm: event.detail.value as "manual" | "auto" })}>
          <EmsSelectOption value="manual">手动确认</EmsSelectOption><EmsSelectOption value="auto">自动确认</EmsSelectOption>
        </EmsSelect>
        <label className="alarm-type-check min-h-10"><input type="checkbox" checked={editing.record_recovery} onChange={(event) => setEditing({ ...editing, record_recovery: event.target.checked })} /> 记录复归</label>
        <label className="alarm-type-check min-h-10"><input type="checkbox" checked={editing.enabled} onChange={(event) => setEditing({ ...editing, enabled: event.target.checked })} /> 启用</label>
      </div>}
    </EmsOverlay>
  </section>
}
