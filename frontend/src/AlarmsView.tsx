import { useCallback, useEffect, useRef, useState } from "react"
import { createPortal } from "react-dom"
import { ackAlarm, closeAlarm, EmsAlarm, fetchAlarms, maintainAlarm, nameOf, Node, releaseAlarm } from "./api"
import AlarmTypesPanel from "./AlarmTypesPanel"
import AlarmStatsPanel, { AlarmSoundSettings, inSilenceWindow, loadAlarmSound, saveAlarmSound } from "./AlarmStatsPanel"
import AckDialog from "./AckDialog"

const severityNames: Record<string, string> = { critical: "严重", warning: "重要", info: "提示" }
const stateNames: Record<string, string> = {
  active: "活动 · 未确认",
  acknowledged: "活动 · 已确认",
  resolved: "已恢复 · 待关闭",
  closed: "已关闭",
}

function timeOf(text: string) {
  if (!text) return "—"
  return new Date(text).toLocaleString("zh-CN", { hour12: false })
}

export default function AlarmsView({ token, rootID, nodes, canOperate, canConfigure, onError, openHistory }: {
  token: string
  rootID: string
  nodes: Node[]
  canOperate: boolean
  canConfigure: boolean
  onError: (message: string) => void
  openHistory: (pointId: string) => void
}) {
  const [alarms, setAlarms] = useState<EmsAlarm[]>([])
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

  const names = new Map(nodes.map((node) => [node.id, nameOf(node)]))
  const nameOfDevice = (id?: string) => (id ? names.get(id) || id : "—")

  const load = useCallback(async () => {
    try {
      const result = await fetchAlarms(token)
      setAlarms(result.alarms || [])
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "报警读取失败")
    }
  }, [token, onError])

  useEffect(() => {
    void load()
    const timer = window.setInterval(() => void load(), 2000)
    return () => window.clearInterval(timer)
  }, [load])

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
    const active = alarms.filter((alarm) => alarm.active && !(alarm.maintenance_until && new Date(alarm.maintenance_until).getTime() > Date.now()))
    const keys = new Set(active.map((alarm) => alarm.id))
    for (const key of soundedRef.current) if (!keys.has(key)) soundedRef.current.delete(key)
    if (!sound.enabled || inSilenceWindow(sound)) return
    const fresh = active.filter((alarm) => !soundedRef.current.has(alarm.id))
    if (fresh.length === 0) return
    for (const alarm of fresh) soundedRef.current.add(alarm.id)
    playTone()
  }, [alarms, sound])

  async function act(alarm: EmsAlarm, action: "ack" | "close" | "release") {
    if (action === "ack") { setAckTarget(alarm); return }
    setBusyId(alarm.id)
    try {
      if (action === "close") await closeAlarm(token, alarm.id)
      else await releaseAlarm(token, alarm.id)
      await load()
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
      await load()
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "维护设置失败")
    } finally {
      setBusyId("")
    }
  }

  function isMaintained(alarm: EmsAlarm) {
    return Boolean(alarm.maintenance_until && new Date(alarm.maintenance_until).getTime() > Date.now())
  }

  const active = alarms.filter((alarm) => alarm.active && !isMaintained(alarm))
  const unacked = active.filter((alarm) => alarm.state === "active")
  const maintained = alarms.filter((alarm) => alarm.active && isMaintained(alarm))
  const resolved = alarms.filter((alarm) => !alarm.active && alarm.state !== "closed")
  const shown = filter === "active" ? active : alarms

  function exportAlarms() {
    const rows = [["时间", "级别", "标题", "详情", "对象", "状态", "确认人", "处理说明"]]
    for (const alarm of shown) {
      rows.push([timeOf(alarm.created_at), alarm.severity, alarm.title, alarm.detail || "", nameOfDevice(alarm.device_id), alarm.state, alarm.acked_by || "", alarm.ack_note || ""])
    }
    const csv = rows.map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(",")).join(String.fromCharCode(10))
    const url = URL.createObjectURL(new Blob([String.fromCharCode(0xFEFF) + csv], { type: "text/csv;charset=utf-8" }))
    const link = document.createElement("a")
    link.href = url
    link.download = "easyems-alarms.csv"
    link.click()
    URL.revokeObjectURL(url)
  }

  return <section className="alarm-page">
    <div className="alarm-view-tabs" role="group" aria-label="报警与类型">
      <button className={tab === "records" ? "active" : ""} onClick={() => setTab("records")}>报警记录</button>
      <button className={tab === "types" ? "active" : ""} onClick={() => setTab("types")}>类型管理</button>
      <button className={tab === "stats" ? "active" : ""} onClick={() => setTab("stats")}>统计与提醒</button>
    </div>
    {tab === "types" ? <AlarmTypesPanel token={token} rootID={rootID} nodes={nodes} canConfigure={canConfigure} onError={onError} /> : tab === "stats" ? <AlarmStatsPanel token={token} onError={onError} sound={sound} onSound={updateSound} onTest={playTone} /> : <>
    <div className="channel-summary">
      <div><strong>{active.length}</strong><span>活动报警</span></div>
      <div className={unacked.length ? "has-warning" : ""}><strong>{unacked.length}</strong><span>未确认</span></div>
      <div><strong>{resolved.length}</strong><span>已恢复待关闭</span></div>
      <div className="alarm-filter">
        {maintained.length > 0 && <span className="alarm-maintained-count">维护中 {maintained.length}</span>}
        <button className="secondary-button" onClick={exportAlarms}>导出 CSV</button>
        <button className={filter === "active" ? "active" : ""} onClick={() => setFilter("active")}>只看需要处理</button>
        <button className={filter === "all" ? "active" : ""} onClick={() => setFilter("all")}>全部记录</button>
      </div>
    </div>

    <div className="alarm-list">
      {shown.map((alarm) => <article className={`panel alarm-card ${alarm.severity} ${alarm.state}`} key={alarm.id}>
        <div className="alarm-head">
          <span className={`alarm-severity ${alarm.severity}`}>{severityNames[alarm.severity] || alarm.severity}</span>
          <strong>{alarm.title}</strong>
          <span className="alarm-state">{stateNames[alarm.state] || alarm.state}</span>
        </div>
        <p>{alarm.detail || "没有更多说明"}</p>
        <div className="alarm-meta">
          <span>对象 {nameOfDevice(alarm.device_id)}</span>
          <span>发生 {timeOf(alarm.created_at)}</span>
          {alarm.acked_by && <span>确认人 {alarm.acked_by}{alarm.ack_note ? ` · ${alarm.ack_note}` : ""}</span>}
        </div>
        {isMaintained(alarm) && <div className="alarm-maintenance">维护中 · 至 {timeOf(alarm.maintenance_until || "")} · {alarm.maintenance_reason || "未填写原因"} · {alarm.maintained_by || "—"}</div>}
        {(alarm.point_id || alarm.device_id) && nodes.some((node) => node.id === (alarm.point_id || alarm.device_id)) && <div className="alarm-drilldown"><button className="text-button" onClick={() => openHistory((alarm.point_id || alarm.device_id)!)}>查看点位趋势</button></div>}
        {canOperate && <div className="alarm-actions">
          {alarm.active && alarm.state === "active" && <button className="primary-button" disabled={busyId === alarm.id} onClick={() => void act(alarm, "ack")}>确认接手</button>}
          {!isMaintained(alarm) && alarm.state !== "closed" && <button className="secondary-button" disabled={busyId === alarm.id} onClick={() => { setMaintaining(alarm); setMaintenanceReason(""); setMaintenanceMinutes(120) }}>维护</button>}
          {isMaintained(alarm) && <button className="secondary-button" disabled={busyId === alarm.id} onClick={() => void act(alarm, "release")}>结束维护</button>}
          {!alarm.active && alarm.state !== "closed" && <button className="secondary-button" disabled={busyId === alarm.id} onClick={() => void act(alarm, "close")}>关闭记录</button>}
        </div>}
      </article>)}
      {shown.length === 0 && <div className="panel empty-state large">
        <strong>{filter === "active" ? "没有需要处理的报警" : "还没有报警记录"}</strong>
        <span>设备离线、通道心跳过期、控制结果未知或规则异常时，会自动出现在这里。</span>
      </div>}
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
        void ackAlarm(token, target.id, note).then(() => load()).catch((cause) => onError(cause instanceof Error ? cause.message : "操作失败")).finally(() => setBusyId(""))
      }}
    />}

    {maintaining && createPortal(<>
      <div className="alarm-type-backdrop" onClick={() => setMaintaining(null)} />
      <section className="alarm-type-modal alarm-maintenance-modal" role="dialog" aria-label="报警维护">
        <div className="alarm-type-modal-head"><div><span className="section-kicker">维护期抑制</span><h2>{maintaining.title}</h2></div><button className="icon-button" aria-label="关闭" onClick={() => setMaintaining(null)}>×</button></div>
        <p className="alarm-maintenance-note">维护期内该报警条件不会重新产生打扰；已有记录保留。维护结束后若条件仍存在，会重新出现。</p>
        <div className="alarm-type-grid">
          <label>维护时长<select value={maintenanceMinutes} onChange={(event) => setMaintenanceMinutes(Number(event.target.value))}><option value={30}>30 分钟</option><option value={120}>2 小时</option><option value={480}>8 小时</option><option value={1440}>24 小时</option></select></label>
          <label>维护原因<input autoFocus value={maintenanceReason} onChange={(event) => setMaintenanceReason(event.target.value)} placeholder="如 检修、备件更换" /></label>
        </div>
        <div className="alarm-type-modal-actions"><button className="secondary-button" onClick={() => setMaintaining(null)}>取消</button><button className="primary-button" disabled={busyId === maintaining.id || !maintenanceReason.trim()} onClick={() => void submitMaintenance()}>{busyId === maintaining.id ? "保存中…" : "开始维护"}</button></div>
      </section>
    </>, document.body)}
  </section>
}
