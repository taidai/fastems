import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from "react"
import { nameOf, Node, point } from "./api"
import Icon, { type IconName } from "./Icons"

const nodes = [
  { key: "pv", label: "光伏发电区", keywords: ["光伏", "PV"], icon: "solar" as IconName },
  { key: "ess", label: "储能系统", keywords: ["储能", "电池", "ESS"], icon: "battery" as IconName },
  { key: "grid", label: "并网配电", keywords: ["电网", "并网", "配电", "关口"], icon: "bolt" as IconName },
  { key: "load", label: "厂区末端负载", keywords: ["负载", "负荷", "用电"], icon: "factory" as IconName },
  { key: "charge", label: "充电区", keywords: ["充电", "桩"], icon: "plug" as IconName },
]
type FlowNode = typeof nodes[number]

export function sumByKeywords(points: Node[], keywords: string[], unitsMatch?: string): number | null {
  let total = 0
  let found = false
  for (const node of points) {
    const name = nameOf(node)
    if (!keywords.some((keyword) => name.toLowerCase().includes(keyword.toLowerCase()))) continue
    const units = point(node, "units")?.text || ""
    if (unitsMatch && !units.toLowerCase().includes(unitsMatch.toLowerCase())) continue
    if (!unitsMatch && /wh/i.test(units)) continue
    const value = point(node, "value")?.value
    if (value === undefined || !Number.isFinite(value)) continue
    total += value
    found = true
  }
  return found ? total : null
}

function powerValue(points: Node[], keywords: string[]): number | null {
  return sumByKeywords(points, keywords)
}

type NodeHealth = { state: "bound" | "stale" | "unbound"; at: string }

function nodeHealth(points: Node[], keywords: string[]): NodeHealth {
  const now = Date.now()
  let found = false
  let fresh = false
  let at = ""
  for (const node of points) {
    const name = nameOf(node)
    if (!keywords.some((keyword) => name.toLowerCase().includes(keyword.toLowerCase()))) continue
    if (/wh/i.test(point(node, "units")?.text || "")) continue
    const value = point(node, "value")
    if (value?.value === undefined) continue
    found = true
    if (value.time && Date.parse(value.time) > Date.parse(at || "1970-01-01")) at = value.time
    const time = value.time ? new Date(value.time).getTime() : NaN
    if (Number.isFinite(time) && now - time <= 15000) fresh = true
  }
  return { state: !found ? "unbound" : fresh ? "bound" : "stale", at }
}

function FlowCard({ item, value, health, onOpen }: { item: FlowNode; value: number | null; health: NodeHealth; onOpen?: (keywords: string[]) => void }) {
  const bound = value !== null
  const stateText = health.state === "bound" ? "已绑定" : health.state === "stale" ? "点位过期" : "未接入"
  return <button
    type="button"
    className={`flow-node ${item.key} ${health.state}`}
    onClick={() => onOpen?.(item.keywords)}
    title={`${item.label} · ${stateText}${health.at ? ` · 采样 ${new Date(health.at).toLocaleTimeString("zh-CN", { hour12: false })}` : ""}`}
  >
    <span className="flow-icon" aria-hidden="true"><Icon name={item.icon} size={38} /></span>
    <span className="flow-node-text">
      <small>{item.label}</small>
      <strong>{bound ? value.toFixed(1) : "—"}<em> kW</em></strong>
    </span>
    <i className={`flow-state ${health.state}`} aria-hidden="true" />
  </button>
}

type FlowEvent = {
  activeRules: number
  rules: number
  alarmCount: number
  errors: number
  pointCount: number
  onOpenRules: () => void
  onOpenTrace?: () => void
  onOpenAlarms?: () => void
  latestAlarm?: { title: string; severity: string; at: string }
}

export type DispatchAction = {
  output: string
  requested: number
  applied: number
  blocked: boolean
  reason: string
  status?: string
}

export type DispatchInfo = {
  rule: string
  at: string
  state: string
  rowIndex: number | null
  actions: DispatchAction[]
}

const stateLabels: Record<string, string> = {
  fired: "已触发",
  "already-fired": "已执行",
  "waiting-hold": "持续确认中",
  cooldown: "冷却中",
  "no-match": "未命中",
}

function stateLabel(state: string) {
  return stateLabels[state] || (state ? state : "未知状态")
}

function timeLabel(at: string) {
  if (!at) return "—"
  const date = new Date(at)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString("zh-CN", { hour12: false })
}

function actionLabel(dispatch: DispatchInfo | null | undefined) {
  if (!dispatch) return "等待首次评估"
  if (!dispatch.actions.length) return "不执行动作"
  const action = dispatch.actions.find((item) => !item.blocked) || dispatch.actions[0]
  const status = action.blocked ? "已拦截" : action.status === "confirmed" ? "已回读"
    : action.status === "unknown" ? "结果未知" : action.status === "failed" ? "失败"
      : action.reason?.startsWith("ramping") ? "爬坡中" : "已下发"
  const reason = action.blocked && action.reason ? `（${action.reason}）` : ""
  return `${action.output} ${format(action.requested)} → ${format(action.applied)} · ${status}${reason}`
}

function rowLabel(dispatch: DispatchInfo) {
  return dispatch.rowIndex === null ? "未命中行" : `命中第 ${dispatch.rowIndex + 1} 行`
}

const paths: Record<string, string> = {
  pv: "M420 74H472L525 122 690 185",
  load: "M718 190 906 126 960 76h100",
  grid: "M846 275 710 239V202",
  ess: "M302 259h44l43 12H520L710 193",
  charge: "M1306 205 1343 240 1324 279",
}

export default function EnergyFlow({ points, siteName, onOpenPoint, onOpenTrends, event, dispatch }: {
  points: Node[]
  siteName?: string
  onOpenPoint?: (keywords: string[], fallback?: "channels" | "alarms") => void
  onOpenTrends?: () => void
  event?: FlowEvent
  dispatch?: DispatchInfo | null
}) {
  const [view, setView] = useState<"scene" | "2d" | "3d">("3d")
  const [camera, setCamera] = useState<"iso" | "top">("iso")
  const [night, setNight] = useState(false)
  const [labels, setLabels] = useState(true)
  const [full, setFull] = useState(false)
  const panel = useRef<HTMLElement>(null)
  const canvas = useRef<HTMLDivElement>(null)
  const values = useMemo(() => new Map(nodes.map((item) => [item.key, powerValue(points, item.keywords)])), [points])
  const health = useMemo(() => new Map(nodes.map((item) => [item.key, nodeHealth(points, item.keywords)])), [points])
  const bound = [...values.values()].filter((value) => value !== null).length
  const viewName = view === "3d" ? "三维视图" : view === "2d" ? "二维视图" : "能量流向"

  useEffect(() => {
    const change = () => setFull(Boolean(document.fullscreenElement))
    document.addEventListener("fullscreenchange", change)
    return () => document.removeEventListener("fullscreenchange", change)
  }, [])

  function toggleFullscreen() {
    if (document.fullscreenElement) {
      void document.exitFullscreen()
    } else if (panel.current?.requestFullscreen) {
      void panel.current.requestFullscreen().catch(() => setFull(false))
    }
  }

  function move(event: ReactPointerEvent<HTMLDivElement>) {
    if (view !== "3d" || !canvas.current) return
    const rect = canvas.current.getBoundingClientRect()
    const x = (event.clientX - rect.left) / rect.width - 0.5
    const y = (event.clientY - rect.top) / rect.height - 0.5
    canvas.current.style.setProperty("--px", `${(-x * 5).toFixed(1)}px`)
    canvas.current.style.setProperty("--py", `${(-y * 3).toFixed(1)}px`)
  }

  function reset() {
    canvas.current?.style.setProperty("--px", "0px")
    canvas.current?.style.setProperty("--py", "0px")
  }

  return <section className="flow-panel panel" ref={panel}>
    <div className="flow-head">
      <div className="flow-title">
        <h2>站点能源流 · {viewName}</h2>
        <span className="flow-sub">{siteName || "当前站点"}</span>
      </div>
      <div className="flow-views" role="group" aria-label="能流视图">
        {([{ key: "scene", label: "能流图" }, { key: "2d", label: "2D能流" }, { key: "3d", label: "3D能流" }] as const).map((item) =>
          <button key={item.key} className={view === item.key ? "active" : ""} onClick={() => setView(item.key)}>{item.label}</button>)}
        <button className="flow-trend" onClick={onOpenTrends}>趋势 + 预测 <span>›</span></button>
      </div>
    </div>
    <div className="flow-bar">
      <div className="flow-status">
        <span className="flow-live"><i />调度 {dispatch ? `${dispatch.rule} · ${stateLabel(dispatch.state)}` : "未启用"}</span>
        {dispatch && <span>{timeLabel(dispatch.at)}</span>}
        {dispatch && <span className="flow-action" title={actionLabel(dispatch)}>{actionLabel(dispatch)}</span>}
        <button type="button" className="inline-link" onClick={event?.onOpenTrace || event?.onOpenRules}>调度详情 ›</button>
      </div>
      <div className="flow-tools">
        <div className="flow-tool-group" role="group" aria-label="场景视角">
          <button className={camera === "iso" ? "active" : ""} disabled={view === "2d"} onClick={() => setCamera("iso")}><Icon name="cube" size={18} />等距</button>
          <button className={camera === "top" ? "active" : ""} disabled={view === "2d"} onClick={() => setCamera("top")}><Icon name="map" size={18} />俯视</button>
        </div>
        <button className={night ? "active" : ""} disabled={view === "2d"} onClick={() => setNight(!night)}><Icon name="sun" size={18} />夜间显示</button>
        <button className={labels ? "active" : ""} onClick={() => setLabels(!labels)}><Icon name="tag" size={18} />标签</button>
        <button onClick={toggleFullscreen}><Icon name="expand" size={18} />{full ? "退出" : "全屏"}</button>
      </div>
    </div>

    {view === "2d" ? <div className="flow-2d" role="group" aria-label="2D 能流示意">
      {nodes.map((item) => <FlowCard key={item.key} item={item} value={values.get(item.key) ?? null} health={health.get(item.key) || { state: "unbound", at: "" }} onOpen={onOpenPoint} />)}
      <span className="flow-2d-bus" aria-hidden="true"><i>交流母线</i></span>
      <span className="flow-2d-note">光伏 + 储能 + 电网 → 母线 → 末端负载 / 充电区 · 数值取自已绑定点位</span>
    </div> : <div
      ref={canvas}
      className={`flow-canvas mode-${view} camera-${camera} ${night ? "night" : ""} ${labels ? "" : "hide-labels"}`}
      onPointerMove={move}
      onPointerLeave={reset}
    >
      <img className="flow-photo" src="/station-model.png" alt="光伏厂房、储能柜、办公楼、变压器和充电区的等距三维示意" draggable={false} />
      <div className="flow-shade" aria-hidden="true" />
      <svg className="flow-lines" viewBox="0 0 1480 370" preserveAspectRatio="none" aria-hidden="true">
        <g className="flow-outline">
          <path d="M420 74H472L525 122 702 190" />
          <path d="M710 190 906 126 960 76h100" />
          <path d="M710 192v47l149 40" />
          <path d="M302 259h44l43 12H520L710 193" />
        </g>
        {nodes.map((item) => <path key={item.key} d={paths[item.key]} className={`flow-line ${item.key} ${values.get(item.key) != null ? "bound" : ""}`} />)}
        <g className="flow-dots">
          <circle cx="525" cy="122" r="6" className="pv" />
          <circle cx="906" cy="126" r="6" className="load" />
          <circle cx="389" cy="271" r="6" className="ess" />
          <circle cx="710" cy="193" r="12" className="grid" />
          <circle cx="1324" cy="279" r="5" className="grid" />
        </g>
      </svg>
      {nodes.map((item) => <FlowCard key={item.key} item={item} value={values.get(item.key) ?? null} health={health.get(item.key) || { state: "unbound", at: "" }} onOpen={onOpenPoint} />)}
    </div>}

    <div className="flow-legend">
      <span><i className="pv" />光伏</span>
      <span><i className="ess" />储能</span>
      <span><i className="load" />负载</span>
      <span><i className="grid" />电网</span>
      <span><i className="unbound" />未接入</span>
      <span className="flow-total">光伏 {format(values.get("pv"))} + 储能 {format(values.get("ess"))} + 电网 {format(values.get("grid"))} → 末端 {format(values.get("load"))}</span>
    </div>
    {event && <div className="flow-event">
      <span className="event-label">今日事件</span>
      <span>{dispatch ? `最近评估 ${timeLabel(dispatch.at)} · ${dispatch.rule} · ${rowLabel(dispatch)}` : "暂无可核实的调度事件"}</span>
      {event.latestAlarm
        ? <span className={`event-alarm ${event.latestAlarm.severity}`}>最新报警 {event.latestAlarm.title} · {timeLabel(event.latestAlarm.at)}</span>
        : <span>{dispatch ? actionLabel(dispatch) : "—"}</span>}
      <button onClick={event.onOpenAlarms || event.onOpenRules}>活动报警 {event.alarmCount} 条 · 点位 {event.pointCount} 个 ›</button>
    </div>}
  </section>
}

function format(value: number | null | undefined) {
  return value === null || value === undefined ? "—" : value.toFixed(1)
}
