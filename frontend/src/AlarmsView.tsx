import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { ackAlarm, closeAlarm, EmsAlarm, maintainAlarm, nameOf, Node, releaseAlarm } from "./api"
import AlarmTypesPanel from "./AlarmTypesPanel"
import AlarmStatsPanel, { AlarmSoundSettings, inSilenceWindow, loadAlarmSound, saveAlarmSound } from "./AlarmStatsPanel"
import AckDialog from "./AckDialog"
import { EmsButton, EmsCard, EmsInput, EmsOverlay, EmsSelect, EmsSelectOption, EmptyState, FilterToolbar, InlineNotice, MetricStrip, SegmentedTabs, StatusBadge, type EmsTone } from "./components/ems"

const severityNames: Record<string, string> = { critical: "严重", warning: "重要", info: "提示" }
const stateNames: Record<string, string> = {
  active: "活动 · 未确认",
  acknowledged: "活动 · 已确认",
  resolved: "已恢复 · 待关闭",
  closed: "已关闭",
}

const severityTone: Record<string, EmsTone> = { critical: "danger", warning: "warning", info: "info" }
const stateTone: Record<string, EmsTone> = { active: "danger", acknowledged: "warning", resolved: "success", closed: "neutral" }

function timeOf(text: string) {
  if (!text) return "—"
  return new Date(text).toLocaleString("zh-CN", { hour12: false })
}

export default function AlarmsView({ token, rootID, nodes, alarms, onRefreshAlarms, canOperate, canConfigure, onError, openHistory }: {
  token: string
  rootID: string
  nodes: Node[]
  alarms: EmsAlarm[]
  onRefreshAlarms: () => Promise<void>
  canOperate: boolean
  canConfigure: boolean
  onError: (message: string) => void
  openHistory: (pointId: string) => void
}) {
  const [tab, setTab] = useState<"records" | "types" | "stats">("records")
  const [filter, setFilter] = useState<"active" | "all">("active")
  const [busyId, setBusyId] = useState("")
  const [sound, setSound] = useState<AlarmSoundSettings>(() => loadAlarmSound())
  const audioRef = useRef<AudioContext | null>(null)
  const soundedRef = useRef<Set<string>>(new Set())
  const [maintaining, setMaintaining] = useState<EmsAlarm | null>(null)
  const [ackTarget, setAckTarget] = useState<EmsAlarm | null>(null)
  const [maintenanceReason, setMaintenanceReason] = useState("")
  const [maintenanceMinutes, setMaintenanceMinutes] = useState(120)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const closeMaintenance = useCallback(() => setMaintaining(null), [])

  const names = new Map(nodes.map((node) => [node.id, nameOf(node)]))
  const nameOfDevice = (id?: string) => (id ? names.get(id) || id : "—")

  // 页面标题与设备树都按站点裁剪，报警列表也必须一致：只展示引用到当前站点节点（或无引用）的报警。
  const scopeIDs = useMemo(() => new Set(nodes.map((node) => node.id)), [nodes])
  const scopedAlarms = useMemo(() => alarms.filter((alarm) => {
    const refs = [alarm.point_id, alarm.device_id, alarm.group_id].filter(Boolean) as string[]
    return refs.length === 0 || refs.some((id) => scopeIDs.has(id))
  }), [alarms, scopeIDs])

  function updateSound(next: AlarmSoundSettings) {
    setSound(next)
    saveAlarmSound(next)
  }

  function playTone() {
    try {
      const Ctor = window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
      if (!Ctor) return
      let context = audioRef.current
      if (!context || context.state === "closed") {
        context = new Ctor()
        audioRef.current = context
      }
      if (context.state === "suspended") void context.resume()
      const start = context.currentTime
      const gainValue = Math.max(0.001, (sound.volume / 100) * 0.35)
      for (const [index, frequency] of [880, 660].entries()) {
        const oscillator = context.createOscillator()
        const gain = context.createGain()
        oscillator.type = "sine"
        oscillator.frequency.value = frequency
        const at = start + index * 0.2
        gain.gain.setValueAtTime(0.0001, at)
        gain.gain.exponentialRampToValueAtTime(gainValue, at + 0.02)
        gain.gain.exponentialRampToValueAtTime(0.0001, at + 0.17)
        oscillator.connect(gain)
        gain.connect(context.destination)
        oscillator.start(at)
        oscillator.stop(at + 0.18)
      }
    } catch {
      // Browsers may block audio until a user gesture; the test button unlocks it.
    }
  }

  useEffect(() => {
    const active = scopedAlarms.filter((alarm) => alarm.active && !(alarm.maintenance_until && new Date(alarm.maintenance_until).getTime() > Date.now()))
    const keys = new Set(active.map((alarm) => alarm.id))
    for (const key of soundedRef.current) if (!keys.has(key)) soundedRef.current.delete(key)
    if (!sound.enabled || inSilenceWindow(sound)) return
    const fresh = active.filter((alarm) => !soundedRef.current.has(alarm.id))
    if (fresh.length === 0) return
    for (const alarm of fresh) soundedRef.current.add(alarm.id)
    playTone()
  }, [scopedAlarms, sound])

  async function act(alarm: EmsAlarm, action: "ack" | "close" | "release") {
    if (action === "ack") { setAckTarget(alarm); return }
    setBusyId(alarm.id)
    try {
      if (action === "close") await closeAlarm(token, alarm.id)
      else await releaseAlarm(token, alarm.id)
      await onRefreshAlarms()
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "操作失败")
    } finally {
      setBusyId("")
    }
  }

  async function submitMaintenance() {
    if (!maintaining) return
    if (!maintenanceReason.trim()) {
      onError("请填写维护原因")
      return
    }
    setBusyId(maintaining.id)
    try {
      const until = new Date(Date.now() + maintenanceMinutes * 60000).toISOString()
      await maintainAlarm(token, maintaining.id, until, maintenanceReason.trim())
      setMaintaining(null)
      setMaintenanceReason("")
      await onRefreshAlarms()
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "维护设置失败")
    } finally {
      setBusyId("")
    }
  }

  function isMaintained(alarm: EmsAlarm) {
    return Boolean(alarm.maintenance_until && new Date(alarm.maintenance_until).getTime() > Date.now())
  }

  const needsAttention = (alarm: EmsAlarm) => alarm.active && !isMaintained(alarm)
  const active = scopedAlarms.filter(needsAttention)
  const unacked = active.filter((alarm) => alarm.state === "active")
  const maintained = scopedAlarms.filter((alarm) => alarm.active && isMaintained(alarm))
  const resolved = scopedAlarms.filter((alarm) => !alarm.active && alarm.state !== "closed")

  const childrenByGroup = new Map<string, EmsAlarm[]>()
  for (const alarm of scopedAlarms) {
    if (!alarm.group_id) continue
    const list = childrenByGroup.get(alarm.group_id) || []
    list.push(alarm)
    childrenByGroup.set(alarm.group_id, list)
  }
  const visibleChildren = (id: string) => (childrenByGroup.get(id) || []).filter((child) => filter === "all" || needsAttention(child))
  const shown = scopedAlarms.filter((alarm) => !alarm.group_id && (filter === "all" || needsAttention(alarm) || visibleChildren(alarm.id).length > 0))

  function toggleGroup(id: string) {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function exportAlarms() {
    const rows = [["时间", "级别", "标题", "详情", "对象", "根因", "次数", "状态", "确认人", "处理说明"]]
    for (const alarm of scopedAlarms) {
      rows.push([timeOf(alarm.created_at), alarm.severity, alarm.title, alarm.detail || "", nameOfDevice(alarm.device_id), nameOfDevice(alarm.root_cause), String(alarm.count || 1), alarm.state, alarm.acked_by || "", alarm.ack_note || ""])
    }
    const csv = rows.map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(",")).join(String.fromCharCode(10))
    const url = URL.createObjectURL(new Blob([String.fromCharCode(0xFEFF) + csv], { type: "text/csv;charset=utf-8" }))
    const link = document.createElement("a")
    link.href = url
    link.download = "fastiot-alarms.csv"
    link.click()
    URL.revokeObjectURL(url)
  }

  function renderCard(alarm: EmsAlarm, isChild: boolean) {
    const children = isChild ? [] : visibleChildren(alarm.id)
    const isOpen = expanded.has(alarm.id)
    return <div className="min-w-0" key={alarm.id}>
      <EmsCard className={`alarm-card ${alarm.severity} ${alarm.state} min-w-0`}>
        <div className="alarm-head">
          <StatusBadge tone={severityTone[alarm.severity] || "neutral"}>{severityNames[alarm.severity] || alarm.severity}</StatusBadge>
          <strong className="min-w-0 break-words">{alarm.title}</strong>
          {(alarm.count || 0) > 1 && <StatusBadge tone="neutral" dot={false}>×{alarm.count}</StatusBadge>}
          <span className="alarm-state"><StatusBadge tone={stateTone[alarm.state] || "neutral"} dot={false}>{stateNames[alarm.state] || alarm.state}</StatusBadge></span>
        </div>
        <p className="min-w-0 break-words">{alarm.detail || "没有更多说明"}</p>
        <div className="alarm-meta min-w-0 gap-x-4 gap-y-1">
          <span className="min-w-0 break-words">对象 {nameOfDevice(alarm.device_id)}</span>
          {alarm.root_cause && <span className="min-w-0 break-words">根因 {nameOfDevice(alarm.root_cause)}</span>}
          <span>发生 {timeOf(alarm.created_at)}</span>
          {alarm.acked_by && <span className="min-w-0 break-words">确认人 {alarm.acked_by}{alarm.ack_note ? ` · ${alarm.ack_note}` : ""}</span>}
        </div>
        {isMaintained(alarm) && <InlineNotice tone="warning">维护中 · 至 {timeOf(alarm.maintenance_until || "")} · {alarm.maintenance_reason || "未填写原因"} · {alarm.maintained_by || "—"}</InlineNotice>}
        {(alarm.point_id || alarm.device_id) && nodes.some((node) => node.id === (alarm.point_id || alarm.device_id)) && <div className="alarm-drilldown"><EmsButton variant="quiet" size="small" onClick={() => openHistory((alarm.point_id || alarm.device_id)!)}>查看点位趋势 →</EmsButton></div>}
        {canOperate && <div className="alarm-actions flex flex-wrap gap-2 border-t border-ems-line pt-3">
          {alarm.active && alarm.state === "active" && <EmsButton size="small" disabled={busyId === alarm.id} onClick={() => void act(alarm, "ack")}>确认接手</EmsButton>}
          {!isMaintained(alarm) && alarm.state !== "closed" && <EmsButton variant="secondary" size="small" disabled={busyId === alarm.id} onClick={() => { setMaintaining(alarm); setMaintenanceReason(""); setMaintenanceMinutes(120) }}>维护</EmsButton>}
          {isMaintained(alarm) && <EmsButton variant="secondary" size="small" disabled={busyId === alarm.id} onClick={() => void act(alarm, "release")}>结束维护</EmsButton>}
          {!alarm.active && alarm.state !== "closed" && <EmsButton variant="secondary" size="small" disabled={busyId === alarm.id} onClick={() => void act(alarm, "close")}>关闭记录</EmsButton>}
        </div>}
      </EmsCard>
      {!isChild && children.length > 0 && <div className="mt-2 grid gap-2">
        <div><EmsButton variant="quiet" size="small" onClick={() => toggleGroup(alarm.id)}>{isOpen ? "收起同根因报警" : `展开同根因 ${children.length} 条`}</EmsButton></div>
        {isOpen && <div className="alarm-children grid min-w-0 gap-3 border-l border-ems-line pl-3 sm:pl-6">{children.map((child) => renderCard(child, true))}</div>}
      </div>}
    </div>
  }

  const tabs: { value: "records" | "types" | "stats"; label: string }[] = [
    { value: "records", label: "报警记录" },
    ...(canConfigure ? [{ value: "types" as const, label: "类型管理" }] : []),
    ...(canConfigure || canOperate ? [{ value: "stats" as const, label: "统计与提醒" }] : []),
  ]

  return <section className="alarm-page min-w-0">
    <div className="alarm-view-tabs min-w-0">
      <SegmentedTabs value={tab} onChange={setTab} ariaLabel="报警与类型" options={tabs} />
    </div>
    {tab === "types" ? <AlarmTypesPanel token={token} rootID={rootID} nodes={nodes} canConfigure={canConfigure} onError={onError} /> : tab === "stats" ? <AlarmStatsPanel token={token} onError={onError} sound={sound} onSound={updateSound} onTest={playTone} /> : <>
      <EmsCard><IonCardContent className="grid min-w-0 gap-4">
        <MetricStrip items={[
          { label: "活动报警", value: active.length, tone: active.length ? "danger" : "success" },
          { label: "未确认", value: unacked.length, tone: unacked.length ? "warning" : "success" },
          { label: "已恢复待关闭", value: resolved.length },
          { label: "维护中", value: maintained.length, tone: maintained.length ? "warning" : "neutral" },
        ]} />
        <FilterToolbar trailing={<EmsButton variant="secondary" size="small" onClick={exportAlarms}>导出 CSV</EmsButton>}>
          <SegmentedTabs value={filter} onChange={setFilter} ariaLabel="报警记录筛选" options={[
            { value: "active", label: "只看需要处理" },
            { value: "all", label: "全部记录" },
          ]} />
        </FilterToolbar>
      </IonCardContent></EmsCard>

      <div className="alarm-list min-w-0">
        {shown.map((alarm) => renderCard(alarm, false))}
        {shown.length === 0 && <EmptyState
          title={filter === "active" ? "没有需要处理的报警，系统正常运行" : "还没有报警记录"}
          description="设备离线、通道心跳过期、控制结果未知或规则异常时，会自动出现在这里。"
        />}
      </div>
    </>}

    {ackTarget && <AckDialog
      alarm={ackTarget}
      busy={busyId === ackTarget.id}
      onCancel={() => setAckTarget(null)}
      onConfirm={(note) => {
        const target = ackTarget
        setAckTarget(null)
        setBusyId(target.id)
        void ackAlarm(token, target.id, note).then(() => onRefreshAlarms()).catch((cause) => onError(cause instanceof Error ? cause.message : "操作失败")).finally(() => setBusyId(""))
      }}
    />}

    <EmsOverlay open={Boolean(maintaining)} title={maintaining?.title || "报警维护"} busy={busyId === maintaining?.id} onClose={closeMaintenance}
      actions={<>
        <EmsButton variant="secondary" disabled={busyId === maintaining?.id} onClick={() => setMaintaining(null)}>取消</EmsButton>
        <EmsButton busy={busyId === maintaining?.id} busyLabel="保存中…" disabled={!maintenanceReason.trim()} onClick={() => void submitMaintenance()}>开始维护</EmsButton>
      </>}>
      <div className="grid min-w-0 gap-4">
        <InlineNotice tone="info">维护期内该报警条件不会重新产生打扰；已有记录保留。维护结束后若条件仍存在，会重新出现。</InlineNotice>
        <EmsSelect label="维护时长" aria-label="维护时长" value={maintenanceMinutes} onIonChange={(event) => setMaintenanceMinutes(Number(event.detail.value))}>
          <EmsSelectOption value={30}>30 分钟</EmsSelectOption><EmsSelectOption value={120}>2 小时</EmsSelectOption><EmsSelectOption value={480}>8 小时</EmsSelectOption><EmsSelectOption value={1440}>24 小时</EmsSelectOption>
        </EmsSelect>
        <EmsInput label="维护原因" aria-label="维护原因" autoFocus value={maintenanceReason} onIonInput={(event) => setMaintenanceReason(event.detail.value || "")} placeholder="如 检修、备件更换" />
      </div>
    </EmsOverlay>
  </section>
}
