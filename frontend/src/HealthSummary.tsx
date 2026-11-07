import { useState } from "react"
import { createPortal } from "react-dom"
import { nameOf, Node, point } from "./api"

function channelDetail(node: Node) {
  const last = Number(point(node, "lastGoodPoll")?.value || 0)
  const poll = Number(point(node, "pollPeriod")?.value || 1000)
  const timeout = Number(point(node, "timeout")?.value || 1000)
  const errors = Number(point(node, "errorCount")?.value || 0)
  const maxAge = Math.max(5, (poll * 5) / 1000, (timeout * 3) / 1000)
  const age = Date.now() / 1000 - last
  const protocol = point(node, "protocol")?.text || "Modbus"
  const path = protocol.toUpperCase() === "RTU" ? point(node, "port")?.text : point(node, "uri")?.text
  if (!last) return { key: "offline", label: "离线", detail: "尚未成功轮询", protocol, path, errors }
  if (age > maxAge) return { key: "offline", label: "离线", detail: `${Math.round(age)} 秒未成功轮询`, protocol, path, errors }
  if (errors > 0) return { key: "degraded", label: "不稳定", detail: `${errors} 次通信错误`, protocol, path, errors }
  return { key: "online", label: "在线", detail: `${Math.max(1, Math.round(age))} 秒前成功轮询`, protocol, path, errors }
}

function pointDetail(node: Node) {
  const value = point(node, "value")
  const at = value?.time ? new Date(value.time) : null
  const age = at ? Math.max(0, Math.round((Date.now() - at.getTime()) / 1000)) : null
  return { value: value?.value, units: point(node, "units")?.text || "", age }
}

type Drill = { kind: "alarms" | "offline" | "stale" } | null

export default function HealthSummary({ alarmCount, offlineChannels, stalePoints, onOpenChannels, onOpenAlarms }: {
  alarmCount: number
  offlineChannels: Node[]
  stalePoints: Node[]
  onOpenChannels: () => void
  onOpenAlarms: () => void
}) {
  const [drill, setDrill] = useState<Drill>(null)

  return <div className="health-summary">
    <button type="button" className={`health-pill ${alarmCount ? "has-warning" : ""}`} onClick={() => setDrill({ kind: "alarms" })}>告警 <b>{alarmCount}</b></button>
    <i>·</i>
    <button type="button" className={`health-pill ${offlineChannels.length ? "has-error" : ""}`} onClick={() => setDrill({ kind: "offline" })}>核心设备离线 <b>{offlineChannels.length}</b></button>
    <i>·</i>
    <button type="button" className={`health-pill ${stalePoints.length ? "has-warning" : ""}`} onClick={() => setDrill({ kind: "stale" })}>核心测点陈旧 <b>{stalePoints.length}</b></button>
    <button className="head-link" onClick={onOpenChannels}>查看健康 <span>›</span></button>

    {drill && createPortal(<div className="device-confirm-backdrop" onClick={() => setDrill(null)}>
      <section className="device-confirm health-drilldown" role="dialog" aria-label="健康详情" onClick={(event) => event.stopPropagation()}>
        <span className="section-kicker">通信详情</span>
        <h3>{drill.kind === "alarms" ? "活动报警" : drill.kind === "offline" ? "核心设备离线" : "核心测点陈旧"}</h3>
        {drill.kind === "alarms" && <div className="health-drill-body">
          <p>{alarmCount ? `当前有 ${alarmCount} 项活动报警。` : "当前没有活动报警。"}</p>
          <button className="secondary-button" onClick={onOpenAlarms}>打开报警与事件 →</button>
        </div>}
        {drill.kind === "offline" && <div className="health-drill-body">
          {offlineChannels.length === 0 && <p>所有通信通道都在时效内成功轮询。</p>}
          {offlineChannels.map((node) => {
            const detail = channelDetail(node)
            return <div className="health-drill-row" key={node.id}>
              <strong>{nameOf(node)}</strong>
              <span className={`health-drill-state ${detail.key}`}>{detail.label}</span>
              <small>{detail.protocol} · {detail.path || "未设置通信地址"} · {detail.detail}{detail.errors > 0 ? ` · ${detail.errors} 次错误` : ""}</small>
            </div>
          })}
        </div>}
        {drill.kind === "stale" && <div className="health-drill-body">
          {stalePoints.length === 0 && <p>所有核心测点都在时效内更新。</p>}
          {stalePoints.map((node) => {
            const detail = pointDetail(node)
            return <div className="health-drill-row" key={node.id}>
              <strong>{nameOf(node)}</strong>
              <small>当前 {detail.value ?? "—"}{detail.units ? ` ${detail.units}` : ""} · {detail.age === null ? "尚未采集" : `${detail.age} 秒未更新`}</small>
            </div>
          })}
        </div>}
        <div className="device-confirm-actions">
          <button className="secondary-button" onClick={() => setDrill(null)}>关闭</button>
        </div>
      </section>
    </div>, document.body)}
  </div>
}
