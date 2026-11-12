import { FormEvent, useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react"
import { IonApp, IonCardContent, IonContent, IonSpinner } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSectionHeader, EmsSelect, EmsSelectOption, EmptyState, InlineNotice, SegmentedTabs, StatusBadge } from "./components/ems"
import DecisionEditor from "./DecisionEditor"
import { writable } from "./writable"
import NodeManager from "./NodeManager"
import DeliveryView from "./DeliveryView"
import AlarmsView from "./AlarmsView"
import HistoryView from "./HistoryView"
import AuditView from "./AuditView"
import type { QualityStats } from "./generated/contract"
import ScreenView from "./ScreenView"
import DeliveryWorkbench from "./DeliveryWorkbench"
import AllocatorsView from "./AllocatorsView"
import PointsView from "./PointsView"
import AggregatesView from "./AggregatesView"
import CommunicationView from "./CommunicationView"
import ModbusTools from "./ModbusTools"
import DeviceLedgerView from "./DeviceLedgerView"
import EnergyUnitsView from "./EnergyUnitsView"
import ResourceGroupsView from "./ResourceGroupsView"
import DispatchTraceView from "./DispatchTraceView"
import OpcuaView from "./OpcuaView"
import UnitWiringView from "./UnitWiringView"
import HealthView from "./HealthView"
import EnergyUnitsStatus from "./EnergyUnitsStatus"
import ConfirmDialog from "./ConfirmDialog"
import FrameDrawer from "./FrameDrawer"
import EnergyFlow, { type DispatchInfo } from "./EnergyFlow"
import AssistantPanel from "./AssistantPanel"
import Icon, { type IconName } from "./Icons"
import KpiStrip, { loadKpiConfig, saveKpiConfig, type KpiConfig } from "./KpiStrip"
import KpiConfigPanel from "./KpiConfigPanel"
import HealthSummary from "./HealthSummary"
import ShiftHandover from "./ShiftHandover"
import CommonCommands from "./CommonCommands"
import {
  ApiError,
  businessId,
  Document,
  EmsAlarm,
  fetchAlarms,
  listUsers,
  login,
  nameOf,
  Node,
  permissionActions,
  Point,
  point,
  request,
  resetUserPassword,
  Snapshot,
  updateUserPolicy,
  type UserAccount,
} from "./api"
import { visiblePagesFor, workbenchFullNames, workbenchNames, type Page, type Workbench } from "./navigation"

const emptyDocument: Document = { table: { rows: [] }, bindings: { points: {}, actions: {} } }
const roleNames: Record<string, string> = { admin: "管理员", implementer: "实施", operator: "操作员", viewer: "查看者" }
const workbenchStorageKey = "easyems-workbench"
type DisplaySettings = { background: number; glass: number }
const defaultDisplay: DisplaySettings = { background: 78, glass: 62 }
const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))
function loadDisplay(): DisplaySettings {
  try {
    const parsed = JSON.parse(localStorage.getItem("easyems-display") || "{}")
    return {
      background: clamp(Number(parsed.background) || defaultDisplay.background, 50, 110),
      glass: clamp(Number(parsed.glass) || defaultDisplay.glass, 30, 90),
    }
  } catch {
    return defaultDisplay
  }
}

function activityDetail(item: Point) {
  if (item.type === "applied") return "运行版本已更新"
  if (item.type === "lastMatch") return "命中状态已记录"
  if (item.type === "enabled") return item.value ? "已启用" : "已停用"
  return item.text || String(item.value || 0)
}

function commandStatus(node: Node) {
  try {
    return JSON.parse(point(node, "lastCommand")?.text || "").status as string
  } catch {
    return ""
  }
}

function errorMessage(error: unknown) {
  if (!(error instanceof Error)) return "操作失败"
  if (error.message.startsWith("used_by_rule")) return "此对象仍被规则引用，请先换绑或删除相关规则"
  if (error.message.startsWith("move_would_break_rule")) return "移动会使规则失去目标，请先调整绑定"
  const messages: Record<string, string> = {
    unauthorized: "登录已过期，请重新登录",
    no_access: "此账号尚未分配 easyems 权限",
    forbidden: "没有执行此操作的权限",
    admin_required: "仅管理员可以配置",
    operation_not_allowed: "此账号只能查看，不能操作",
    publish_required: "请先发布有效规则",
    output_owned: "该输出点位已被另一张已启用决策表占用，请先停用或修改另一张规则",
    command_not_accepted: "设备命令未被接受，请检查连接",
    starting: "应用正在初始化，请稍后重试",
    node_disabled: "对象已停用，请先恢复后再操作",
    has_children: "请先处理此对象下的子节点",
    invalid_move: "不能移动到此位置",
    move_would_break_rule: "移动会使规则失去目标，请先调整绑定",
  }
  return messages[error.message] || error.message
}

function channelState(node: Node) {
  const last = Number(point(node, "lastGoodPoll")?.value || 0)
  const poll = Number(point(node, "pollPeriod")?.value || 1000)
  const timeout = Number(point(node, "timeout")?.value || 1000)
  const maxAge = Math.max(5, (poll * 5) / 1000, (timeout * 3) / 1000)
  const age = Date.now() / 1000 - last
  const errors = Number(point(node, "errorCount")?.value || 0)
  if (!last || age > maxAge) {
    return { key: "offline", label: "离线", detail: last ? `${Math.round(age)} 秒未成功轮询` : "尚未成功轮询" }
  }
  if (errors > 0) return { key: "degraded", label: "不稳定", detail: `${errors} 次通信错误` }
  return { key: "online", label: "在线", detail: `${Math.max(1, Math.round(age))} 秒前成功轮询` }
}

function latestDispatch(rules: Node[], nodes: Node[]): DispatchInfo | null {
  let best: { at: number; info: DispatchInfo } | null = null
  const nameFor = (id: string) => {
    const node = nodes.find((item) => businessId(item) === id)
    return node ? nameOf(node) : id
  }
  for (const rule of rules) {
    const text = point(rule, "lastCheck")?.text
    if (!text) continue
    try {
      const check = JSON.parse(text)
      const at = Date.parse(check.at || "")
      if (!Number.isFinite(at) || (best && at <= best.at)) continue
      let rows: { row_id?: string }[] = []
      try {
        const document = JSON.parse(point(rule, "draft")?.text || point(rule, "applied")?.text || "{}")
        rows = document.table?.rows || []
      } catch { /* malformed draft means no row index, not no dispatch */ }
      const rowIndex = rows.findIndex((row) => row.row_id === check.row)
      best = {
        at,
        info: {
          rule: nameOf(rule),
          at: check.at || "",
          state: check.state || "",
          rowIndex: rowIndex >= 0 ? rowIndex : null,
          actions: (check.actions || []).map((action: { output?: string; requested?: number; applied?: number; blocked?: boolean; reason?: string; status?: string }) => ({
            output: nameFor(action.output || ""),
            requested: Number(action.requested) || 0,
            applied: Number(action.applied) || 0,
            blocked: Boolean(action.blocked),
            reason: action.reason || "",
            status: action.status || "",
          })),
        },
      }
    } catch {
      // ignore malformed lastCheck; the rule page still shows the raw record
    }
  }
  return best?.info ?? null
}

function isDomainNode(type: string) {
  return type === "site" || type === "plant" || type === "energyUnit" || type === "device"
}

function subtreeSnapshot(snapshot: Snapshot, rootID: string): Snapshot {  if (!rootID || !snapshot.roots.includes(rootID)) return snapshot
  const children = new Map<string, Node[]>()
  for (const node of snapshot.nodes) {
    const list = children.get(node.parent) || []
    list.push(node)
    children.set(node.parent, list)
  }
  const nodes: Node[] = []
  const rootNode = snapshot.nodes.find((node) => node.id === rootID)
  if (rootNode) nodes.push(rootNode)
  const visit = (id: string) => {
    for (const child of children.get(id) || []) {
      nodes.push(child)
      visit(child.id)
    }
  }
  visit(rootID)
  return { ...snapshot, roots: [rootID], nodes }
}

// 按站点裁剪：保留选中站点的子树 + 项目级接入对象（通道、规则等），
// 其他站点的单元与设备从视图中消失，避免客户总览把多个站混在一起。
function siteSnapshot(snapshot: Snapshot, siteID: string): Snapshot {
  if (!siteID) return snapshot
  const byID = new Map(snapshot.nodes.map((node) => [node.id, node]))
  const siteIDs = new Set(snapshot.nodes.filter((node) => node.type === "site").map((node) => node.id))
  const ancestorSite = (id: string) => {
    let current = id
    const seen = new Set<string>()
    while (current && !seen.has(current)) {
      seen.add(current)
      if (siteIDs.has(current)) return current
      const node = byID.get(current)
      if (!node) return ""
      current = node.parent
    }
    return ""
  }
  const nodes = snapshot.nodes.filter((node) => {
    const site = ancestorSite(node.id)
    return !site || site === siteID
  })
  return { ...snapshot, nodes }
}

export default function App() {
  const [token, setToken] = useState(() => sessionStorage.getItem("easyems-token") || "")
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [page, setPage] = useState<Page>("devices")
  const [workbench, setWorkbench] = useState<Workbench>(() => {
    const saved = window.localStorage.getItem(workbenchStorageKey)
    return saved === "delivery" || saved === "customer" ? saved : "customer"
  })
  const [siteID, setSiteID] = useState(() => window.localStorage.getItem("easyems-site") || "")
  const [error, setError] = useState("")
  const [email, setEmail] = useState("admin")
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [selectedRule, setSelectedRule] = useState("")
  // ruleAnchor remembers the rule that was open before a copy, so deleting the
  // copy falls back to the source instead of the first rule in the list.
  const ruleAnchor = useRef("")
  const selectRule = useCallback((id: string) => {
    setSelectedRule((prev) => {
      if (prev && prev !== id && id) ruleAnchor.current = prev
      return id
    })
  }, [])
  const [historyPoint, setHistoryPoint] = useState("")
  const [stationID, setStationID] = useState("")
  const [navMenu, setNavMenu] = useState<string | null>(null)
  const navRef = useRef<HTMLElement>(null)
  const [display, setDisplay] = useState<DisplaySettings>(loadDisplay)
  const [displayOpen, setDisplayOpen] = useState(false)
  const displayRef = useRef<HTMLDivElement>(null)
  const [clock, setClock] = useState(() => new Date())
  const [alarmCount, setAlarmCount] = useState(0)
  const [alarms, setAlarms] = useState<EmsAlarm[]>([])
  const [assistantOpen, setAssistantOpen] = useState(false)
  const [tariffDraft, setTariffDraft] = useState("")
  const [kpiConfig, setKpiConfig] = useState<KpiConfig>(loadKpiConfig)
  const [kpiConfigOpen, setKpiConfigOpen] = useState(false)
  const [shiftOpen, setShiftOpen] = useState(false)
  const [commandsOpen, setCommandsOpen] = useState(false)
  const [stageScale, setStageScale] = useState(() => window.innerWidth >= 1000 ? Math.min((window.innerWidth - 8) / 1632, (window.innerHeight - 8) / 1000) : 1)

  const policy = snapshot?.user.policy
  const isAdminUser = !!policy?.admin
  const canConfigureUser = isAdminUser || policy?.role === "implementer"
  const visiblePages = useMemo(
    () => visiblePagesFor(workbench, isAdminUser, canConfigureUser),
    [workbench, isAdminUser, canConfigureUser],
  )
  // 路由守卫：切换工作台或角色变化后，当前页若不在可见集合内，落到该工作台第一个可见页。
  useEffect(() => {
    if (visiblePages.length && !visiblePages.some((def) => def.key === page)) setPage(visiblePages[0].key)
  }, [visiblePages, page])

  // 默认工作台：首次按角色，之后记住上次选择；角色被降权时回落到客户工作台。
  const workbenchInitialized = useRef(false)
  const switchWorkbench = useCallback((next: Workbench) => {
    setWorkbench(next)
    window.localStorage.setItem(workbenchStorageKey, next)
    setNavMenu(null)
  }, [])
  useEffect(() => {
    if (!snapshot?.user.policy) return
    if (workbench === "delivery" && !canConfigureUser) {
      setWorkbench("customer")
      return
    }
    if (!workbenchInitialized.current) {
      workbenchInitialized.current = true
      if (!window.localStorage.getItem(workbenchStorageKey) && canConfigureUser) setWorkbench("delivery")
    }
  }, [snapshot, workbench, canConfigureUser])

  const refresh = useCallback(async () => {
    if (!token) return
    try {
      setSnapshot(await request<Snapshot>(token, "/snapshot"))
      setError("")
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        sessionStorage.removeItem("easyems-token")
        setToken("")
        setSnapshot(null)
      }
      setError(errorMessage(cause))
    }
  }, [token])

  useEffect(() => {
    if (!token) return
    void refresh()
    const timer = window.setInterval(() => void refresh(), 2000)
    return () => window.clearInterval(timer)
  }, [token, refresh])

  useEffect(() => {
    const timer = window.setInterval(() => setClock(new Date()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const lastActivity = useRef(Date.now())
  useEffect(() => {
    const mark = () => { lastActivity.current = Date.now() }
    const events: (keyof WindowEventMap)[] = ["pointerdown", "keydown", "mousemove", "touchstart", "scroll"]
    events.forEach((name) => window.addEventListener(name, mark, { passive: true }))
    return () => events.forEach((name) => window.removeEventListener(name, mark))
  }, [])

  useEffect(() => {
    if (!token) return
    const timeoutSeconds = snapshot?.session_timeout_seconds ?? 1800
    if (timeoutSeconds <= 0) return
    const timer = window.setInterval(() => {
      if (Date.now() - lastActivity.current > timeoutSeconds * 1000) {
        sessionStorage.removeItem("easyems-token")
        window.location.reload()
      }
    }, 1000)
    return () => window.clearInterval(timer)
  }, [token, snapshot?.session_timeout_seconds])

  useEffect(() => {
    const resize = () => setStageScale(window.innerWidth >= 1000 ? Math.min((window.innerWidth - 8) / 1632, (window.innerHeight - 8) / 1000) : 1)
    window.addEventListener("resize", resize)
    resize()
    return () => window.removeEventListener("resize", resize)
  }, [])

  useEffect(() => {
    if (!token) return
    const load = async () => {
      try {
        const result = await fetchAlarms(token)
        const list = result.alarms || []
        setAlarms(list)
        setAlarmCount(list.filter((alarm) => alarm.active && !(alarm.maintenance_until && new Date(alarm.maintenance_until).getTime() > Date.now())).length)
      } catch {
        // 顶栏报警计数失败不阻塞工作台
      }
    }
    void load()
    const timer = window.setInterval(() => void load(), 5000)
    return () => window.clearInterval(timer)
  }, [token])

  useEffect(() => {
    if (!navMenu) return
    const close = (event: PointerEvent) => {
      if (!navRef.current?.contains(event.target as globalThis.Node)) setNavMenu(null)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setNavMenu(null)
    }
    document.addEventListener("pointerdown", close)
    document.addEventListener("keydown", escape)
    return () => {
      document.removeEventListener("pointerdown", close)
      document.removeEventListener("keydown", escape)
    }
  }, [navMenu])

  useEffect(() => {
    const root = document.documentElement
    const glass = display.glass / 100
    root.style.setProperty("--bg-brightness", (display.background / 100).toFixed(2))
    root.style.setProperty("--glass-a", glass.toFixed(2))
    root.style.setProperty("--glass-a-strong", Math.min(0.94, glass + 0.24).toFixed(2))
    root.style.setProperty("--glass-a-soft", Math.max(0.24, glass - 0.12).toFixed(2))
    localStorage.setItem("easyems-display", JSON.stringify(display))
  }, [display])

  useEffect(() => {
    if (!displayOpen) return
    const close = (event: PointerEvent) => {
      if (!displayRef.current?.contains(event.target as globalThis.Node)) setDisplayOpen(false)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setDisplayOpen(false)
    }
    document.addEventListener("pointerdown", close)
    document.addEventListener("keydown", escape)
    return () => {
      document.removeEventListener("pointerdown", close)
      document.removeEventListener("keydown", escape)
    }
  }, [displayOpen])

  const activeSnapshot = useMemo(() => {
    if (!snapshot) return null
    const base = subtreeSnapshot(snapshot, stationID || snapshot.roots[0] || "")
    return siteID ? siteSnapshot(base, siteID) : base
  }, [snapshot, stationID, siteID])

  const siteNodes = useMemo(() => snapshot?.nodes.filter((node) => node.type === "site") ?? [], [snapshot])
  useEffect(() => {
    if (!snapshot) return
    if (siteNodes.length === 0) {
      if (siteID) setSiteID("")
      return
    }
    if (!siteNodes.some((node) => node.id === siteID)) {
      const first = siteNodes[0].id
      setSiteID(first)
      window.localStorage.setItem("easyems-site", first)
    }
  }, [snapshot, siteNodes, siteID])

  async function signIn(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError("")
    try {
      const result = await login(email, password)
      sessionStorage.setItem("easyems-token", result.token)
      setToken(result.token)
      setPassword("")
    } catch (cause) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    setError("")
    try {
      await action()
      await refresh()
      return true
    } catch (cause) {
      setError(errorMessage(cause))
      return false
    } finally {
      setBusy(false)
    }
  }

  if (!token) return <IonApp><IonContent fullscreen><main className="login-page">
    <section className="login-intro">
      <div className="brand"><span className="brand-mark">e</span> easyEMS</div>
      <h1>让能源设备，<br />按清晰的规则运行。</h1>
      <p>设备接入、实时状态与有序决策表，收进同一个工作台。</p>
      <div className="login-proof"><span>一个应用</span><span>本地数据</span><span>无需编写代码</span></div>
    </section>
    <form className="login-form" onSubmit={signIn}>
      <span className="section-kicker">欢迎回来</span>
      <h2>登录工作台</h2>
      <label>账号<input autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>
      <label>密码<input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required /></label>
      {error && <div className="inline-error" role="alert">{error}</div>}
      <button className="primary-button login-button" disabled={busy}>{busy ? "登录中…" : "登录"}</button>
      <small>使用本账号登录，不共享管理员令牌。</small>
    </form>
  </main></IonContent></IonApp>

  if (!snapshot) return <IonApp><IonContent fullscreen><main className="loading-page"><IonSpinner /><p>{error || "正在读取工作台…"}</p></main></IonContent></IonApp>

  const view = activeSnapshot || snapshot
  const isAdmin = isAdminUser
  const canConfigure = canConfigureUser
  const canOperate = canConfigure || view.user.policy.role === "operator"
  const points = view.nodes.filter((node) => node.type === "variable" || node.type === "modbusIo")
  const groups = view.nodes.filter((node) => node.type === "group" || view.roots.includes(node.id))
  const rules = view.nodes.filter((node) => node.type === "decisionTable")
  const channels = view.nodes.filter((node) => node.type === "modbus")
  const ioCounts = new Map<string, number>()
  for (const node of view.nodes) {
    if (node.type === "modbusIo") ioCounts.set(node.parent, (ioCounts.get(node.parent) || 0) + 1)
  }
  const activeRules = rules.filter((rule) => (point(rule, "enabled")?.value || 0) !== 0)
  const dispatch = latestDispatch(activeRules, view.nodes)
  const latestAlarm = alarms.filter((alarm) => alarm.active).sort((a, b) => b.created_at.localeCompare(a.created_at))[0] || null
  const rootNode = view.nodes.find((node) => node.id === view.roots[0])
  const tariff = rootNode ? (point(rootNode, "easyemsTariff")?.value ?? null) : null
  const offlineChannels = channels.filter((channel) => {
    const last = point(channel, "lastGoodPoll")?.value || 0
    const poll = point(channel, "pollPeriod")?.value || 1000
    const timeout = point(channel, "timeout")?.value || 1000
    return !last || clock.getTime() / 1000 - last > Math.max(5, poll * 5 / 1000, timeout * 3 / 1000)
  })
  const stalePoints = points.filter((node) => {
    const value = point(node, "value")
    const time = value?.time ? new Date(value.time).getTime() : NaN
    return value?.value === undefined || !Number.isFinite(time) || clock.getTime() - time > 15000
  })
  const errors = view.nodes.filter((node) => point(node, "error")?.text || (point(node, "errorCount")?.value || 0) > 0 || ["unknown", "failed"].includes(commandStatus(node)))
  const currentRule = (selectedRule ? rules.find((rule) => rule.id === selectedRule) : undefined)
    || rules.find((rule) => rule.id === ruleAnchor.current)
    || rules[0]

  function openPoint(keywords: string[], fallback: Page = "history") {
    const match = keywords.length ? points.find((node) => {
      const name = nameOf(node).toLowerCase()
      return keywords.some((keyword) => name.includes(keyword.toLowerCase())) && point(node, "value") !== undefined
    }) : undefined
    if (match) {
      setHistoryPoint(match.id)
      setPage("history")
    } else {
      setPage(fallback)
    }
  }

  const pageInfo: Record<string, { title: string; subtitle: string }> = {
    devices: { title: "站点总览", subtitle: "设备状态、通信健康与自动运行，一屏看清" },
    screen: { title: "运行画面", subtitle: "按业务节点组织的运行画面" },
    ledger: { title: "设备台账", subtitle: "一台设备一行：业务节点、通道/站号、Profile、健康与验证" },
    points: { title: "设备与点位", subtitle: "批量管理、实时值、质量与回读" },
    channels: { title: "通道", subtitle: "运行健康、端口参数、单元与点位" },
    aggregate: { title: "虚拟聚合", subtitle: "只读虚拟设备：求和、平均、最值、计数与质量传播" },
    deploy: { title: "交付工作台", subtitle: "7 步交付主链：门槛、预检与验收进度" },
    allocators: { title: "功率分配器", subtitle: "一个总目标，按每台 PCS 当前能力分配" },
    energyUnits: { title: "能源设备", subtitle: "能力级规则、站级策略、资源集群与调度追溯" },
    unitWiring: { title: "能源设备接线", subtitle: "为能源设备挂 PCS 并映射功率设定点位（点位只在此层出现）" },
    resourceGroups: { title: "资源集群", subtitle: "一组能源设备共享馈线或功率上限，分配先受组上限约束" },
    dispatchTrace: { title: "调度追溯", subtitle: "每次调度的请求、能力、分配与设备设定，按 dispatchID 复盘" },
    alarms: { title: "报警与事件", subtitle: "该处理的排在前面" },
    history: { title: "历史与追溯", subtitle: "趋势、历史与统一时间轴" },
    health: { title: "通信与健康", subtitle: "通道成功率、延迟与断线，按 5 分钟与 1 小时窗口" },
    query: { title: "设备数据查询", subtitle: "按站点查概览、实时值、告警、趋势与报表" },
    rules: { title: "决策表", subtitle: "有序规则、模拟、发布与运行记录" },
    users: { title: "账号与权限", subtitle: "按人员分配可见与可操作范围" },
    audit: { title: "审计日志", subtitle: "登录、配置、控制与报警操作留痕" },
    delivery: { title: "交付检查", subtitle: "异常、备份与验收边界" },
    opcua: { title: "OPC-UA", subtitle: "端点连接、NodeID 测点与在线状态" },
  }
  const currentInfo = pageInfo[page] || pageInfo.devices
  const labelForRoot = (root: string) => {
    const node = snapshot.nodes.find((item) => item.id === root)
    return node ? nameOf(node) : "本站"
  }
  const stationName = labelForRoot(view.roots[0])
  const tabs = [
    { key: "run", label: "运行", icon: "home" as IconName, workbench: "customer" as Workbench, pages: ["devices", "screen", "alarms", "dispatchTrace", "history", "health", "ledger"] },
    { key: "ems", label: "EMS 业务", icon: "sliders" as IconName, workbench: "delivery" as Workbench, pages: ["energyUnits", "resourceGroups", "allocators", "rules"] },
    { key: "build", label: "接入建模", icon: "devices" as IconName, workbench: "delivery" as Workbench, pages: ["ledger", "unitWiring", "channels", "points", "aggregate", "opcua"] },
    { key: "ops", label: "系统运维", icon: "gear" as IconName, workbench: "delivery" as Workbench, pages: ["deploy", "users", "audit"] },
  ].filter((tab) => tab.workbench === workbench)
  const activeTab = tabs.find((tab) => tab.pages.includes(page))?.key
  const directory = visiblePages

  return <IonApp><IonContent fullscreen><div className="console-viewport"><div className="console-shell" style={{ "--console-scale": String(stageScale) } as CSSProperties}>
    <header className="console-topbar">
      <div className="console-brand"><img className="brand-mark" src="/zizu-brand.png" alt="" /><strong>自足能源AI <small>3.0</small></strong></div>
      {snapshot.roots.length > 1
        ? <select className="console-station" aria-label="当前项目" value={view.roots[0]} onChange={(event) => setStationID(event.target.value)}>
          {snapshot.roots.map((root) => <option key={root} value={root}>{labelForRoot(root)}</option>)}
        </select>
        : <span className="console-station-label" title="当前项目">{labelForRoot(view.roots[0])}</span>}
      {siteNodes.length > 1 && <select className="console-site" aria-label="当前站点" value={siteID}
        onChange={(event) => { setSiteID(event.target.value); window.localStorage.setItem("easyems-site", event.target.value) }}>
        {siteNodes.map((site) => <option key={site.id} value={site.id}>{nameOf(site)}</option>)}
      </select>}
      {canConfigure && <div className="console-workbench" role="group" aria-label="工作台">
        {(["customer", "delivery"] as Workbench[]).map((key) => (
          <button key={key} className={workbench === key ? "active" : ""} aria-pressed={workbench === key} title={workbenchFullNames[key]} onClick={() => switchWorkbench(key)}>{workbenchNames[key]}</button>
        ))}
      </div>}
      <nav className="console-topnav" aria-label="主导航" ref={navRef}>
        {tabs.map((tab) => {
          const subPages = directory.filter((d) => tab.pages.includes(d.key) && (!d.adminOnly || isAdmin) && (!d.configOnly || canConfigure))
          const isActive = activeTab === tab.key
          return <div key={tab.key} className={`topnav-item${isActive ? " active" : ""}`}>
            <button className="topnav-trigger" aria-expanded={navMenu === tab.key} onClick={() => {
              if (subPages.length > 1) setNavMenu(navMenu === tab.key ? null : tab.key)
              else { setPage(tab.pages[0] as Page); setNavMenu(null) }
            }}><Icon name={tab.icon} size={16} /><span>{tab.label}</span>{subPages.length > 1 && <span className="topnav-caret" aria-hidden="true">▾</span>}</button>
            {navMenu === tab.key && subPages.length > 1 && <div className="topnav-dropdown" role="menu">
              {subPages.map((d) => <button key={d.key} role="menuitem" className={page === d.key ? "active" : ""} onClick={() => { setPage(d.key); setNavMenu(null) }}>{d.label}</button>)}
            </div>}
          </div>
        })}
      </nav>
      <div className="console-top-actions">
        <button className="console-icon screen-shortcut" title="运行画面" aria-label="运行画面" onClick={() => setPage("screen")}><Icon name="map" size={25} /></button>
        <div className="console-display" ref={displayRef}>
          <button className="console-icon" title="显示设置" aria-label="显示设置" aria-expanded={displayOpen} onClick={() => { setDisplayOpen(!displayOpen); setTariffDraft(tariff === null ? "" : tariff.toFixed(2)) }}><Icon name="sun" size={17} /></button>
          {displayOpen && <div className="display-panel" role="dialog" aria-label="显示设置">
            <div className="display-head"><strong>显示设置</strong><button onClick={() => { setDisplay(defaultDisplay); setDisplayOpen(false) }}>恢复默认</button></div>
            <label>背景明暗<span>{display.background}%</span>
              <input type="range" min="50" max="110" step="1" value={display.background} onChange={(event) => setDisplay({ ...display, background: Number(event.target.value) })} />
            </label>
            <label>玻璃透明度<span>{display.glass}%</span>
              <input type="range" min="30" max="90" step="1" value={display.glass} onChange={(event) => setDisplay({ ...display, glass: Number(event.target.value) })} />
            </label>
            {isAdmin && <label>站点电价（元/kWh）<span>{tariff === null ? "未设置" : tariff.toFixed(2)}</span>
              <div className="tariff-row">
                <input type="number" min="0" max="100" step="0.01" value={tariffDraft} placeholder="例如 0.65" aria-label="站点电价" onChange={(event) => setTariffDraft(event.target.value)} />
                <button className="secondary-button" disabled={busy} onClick={() => {
                  const price = Number(tariffDraft)
                  if (tariffDraft.trim() === "" || !Number.isFinite(price) || price < 0 || price > 100) {
                    setError("电价必须是 0–100 之间的数字")
                    return
                  }
                  void run(async () => {
                    await request(token, "/tariff", "PUT", { root: view.roots[0], price })
                    setTariffDraft(price.toFixed(2))
                  })
                }}>保存</button>
              </div>
            </label>}
            <small>显示设置保存在本机浏览器；电价保存在本站业务节点，用于“今日用电 × 电价”的电费估算。</small>
          </div>}
        </div>
        <button className="console-icon" title="活动报警" aria-label="活动报警" onClick={() => setPage("alarms")}><Icon name="bell" size={25} />{alarmCount > 0 && <em>{alarmCount}</em>}</button>
        <span className="console-user">楼顶 · {roleNames[view.user.policy.role] || "用户"}</span>
        <time>{clock.toLocaleTimeString("zh-CN", { hour12: false })}</time>
        {isAdmin && <button className="console-profile" aria-label="账号与权限" title={view.user.email || view.user.user_id} onClick={() => setPage("users")}><Icon name="user" size={23} /><span>{(view.user.email || view.user.user_id).split("@")[0]}</span></button>}
        <button className="console-exit" title="退出登录" aria-label="退出登录" onClick={() => { sessionStorage.removeItem("easyems-token"); setToken(""); setSnapshot(null) }}>退出</button>
      </div>
    </header>

    <main className="console-main">
      <div className={`console-page-head ${page === "devices" ? "overview-heading" : ""}`}>
        <h1>{currentInfo.title}</h1>
        {page === "devices" && <HealthSummary
          alarmCount={alarmCount}
          offlineChannels={offlineChannels}
          stalePoints={stalePoints}
          onOpenChannels={() => setPage(canConfigure ? "channels" : "health")}
          onOpenAlarms={() => setPage("alarms")}
        />}
        {page !== "devices" && <p>{currentInfo.subtitle}</p>}
        <div className="console-head-actions">
          <span className="site-pill">{siteNodes.length > 0
            ? `站点 ${Math.max(1, siteNodes.findIndex((node) => node.id === siteID) + 1)} / ${siteNodes.length}`
            : `项目 1 / ${snapshot.roots.length}`}</span>
          {page === "devices" && <>
            <button className="secondary-button head-action" onClick={() => setCommandsOpen(true)}>常用命令</button>
            <button className="secondary-button head-action" onClick={() => setShiftOpen(true)}>班次交接</button>
            <button className="secondary-button head-action" onClick={() => setKpiConfigOpen(true)}>显示设置</button>
          </>}
          {page !== "devices" && <div className="live-status"><span className="pulse" />数据每 2 秒更新</div>}
        </div>
      </div>
      {error && <div className="inline-error global-error" role="alert">{error}<button onClick={() => setError("")}>关闭</button></div>}

      <div className="console-stage">
      {page === "devices" && <DevicesView
        snapshot={view} points={points} groups={groups} rules={rules.length} activeRules={activeRules.length} errors={errors.length}
        alarmCount={alarmCount} busy={busy} isAdmin={canConfigure} canConfigure={canConfigure} canOperate={canOperate} run={run} openPoint={openPoint}
        goRules={() => setPage("rules")} goHistory={() => setPage("history")} goChannels={() => setPage("channels")} goEnergyUnits={() => setPage("energyUnits")} goDispatchTrace={() => setPage("dispatchTrace")} goAlarms={() => setPage("alarms")} dispatch={dispatch} latestAlarm={latestAlarm} tariff={tariff}
        kpiConfig={kpiConfig}
        onError={setError}
      />}

      {page === "screen" && <ScreenView
        token={token} snapshot={view} points={points} groups={groups}
        isAdmin={canConfigure} canOperate={canOperate} busy={busy} run={run} onError={setError}
        openHistory={(id) => { setHistoryPoint(id); setPage("history") }}
        openAlarms={() => setPage("alarms")}
      />}

      {page === "ledger" && <DeviceLedgerView token={token} site={siteID} onError={setError} canConfigure={canConfigure && workbench === "delivery"} canOperate={canOperate && workbench === "delivery"} goChannels={() => setPage("channels")} goPoints={() => setPage("points")} goHistory={() => setPage("history")} goAlarms={() => setPage("alarms")} />}

      {page === "points" && canConfigure && <PointsView
        token={token} nodes={view.nodes} isAdmin={canConfigure} canOperate={canOperate} busy={busy} run={run}
        onError={setError} openHistory={(id) => { setHistoryPoint(id); setPage("history") }}
        goChannels={() => setPage("channels")}
      />}

      {page === "channels" && canConfigure && <ChannelWorkspace
        token={token} channels={channels} ioCounts={ioCounts} isAdmin={canConfigure} canOperate={canOperate} nodes={view.nodes} roots={view.roots} groups={groups}
        templates={view.nodes.filter((node) => node.type === "deviceTemplate")} busy={busy} run={run} onError={setError} goPoints={() => setPage("points")}
      />}

      {page === "aggregate" && canConfigure && <AggregatesView token={token} nodes={view.nodes} groups={groups} canConfigure={canConfigure} />}

      {page === "deploy" && canConfigure && <DeliveryWorkbench token={token} nodes={view.nodes} siteID={siteID} canConfigure={canConfigure} goPage={(next) => setPage(next)} onError={setError} />}

      {page === "allocators" && canConfigure && <AllocatorsView
        token={token} nodes={view.nodes} groups={groups} points={points}
        isAdmin={canConfigure} busy={busy} run={run} onError={setError}
      />}

      {page === "energyUnits" && canConfigure && <EnergyUnitsView token={token} nodes={view.nodes} onError={setError} />}
      {page === "resourceGroups" && canConfigure && <ResourceGroupsView token={token} onError={setError} />}
      {page === "dispatchTrace" && <DispatchTraceView token={token} onError={setError} />}
      {page === "opcua" && canConfigure && <OpcuaView token={token} onError={setError} />}
      {page === "unitWiring" && canConfigure && <UnitWiringView token={token} nodes={view.nodes} onError={setError} />}

      {page === "alarms" && <AlarmsView token={token} rootID={view.roots[0]} nodes={view.nodes} canOperate={canOperate} canConfigure={canConfigure} onError={setError} openHistory={(id) => { setHistoryPoint(id); setPage("history") }} />}

      {page === "health" && <HealthView token={token} onError={setError} />}

      {page === "history" && <HistoryView key={historyPoint || "history"} token={token} points={points} initialPoint={historyPoint || undefined} onError={setError} isAdmin={canConfigure} />}

      {page === "rules" && canConfigure && <section className="rules-page">
        <div className="page-toolbar">
          <div className="rule-tabs">{rules.map((rule) => <button key={rule.id} className={currentRule?.id === rule.id ? "active" : ""} onClick={() => selectRule(rule.id)}>{nameOf(rule)}</button>)}</div>
          {canConfigure && <button className="primary-button" disabled={busy} onClick={() => void (async () => {
            const previous = selectedRule
            setSelectedRule("__creating__")
            const ok = await run(async () => {
              const result = await request<{ id: string }>(token, "/rules", "POST", {
                parent: view.roots[0], description: `新决策表 ${rules.length + 1}`, document: emptyDocument,
              })
              setSelectedRule(result.id)
            })
            if (!ok) setSelectedRule(previous)
          })()}>+ 新建决策表</button>}
        </div>
        {currentRule ? <DecisionEditor key={currentRule.id} token={token} rule={currentRule} nodes={view.nodes} onRefresh={refresh} onError={setError} onSelect={selectRule} readOnly={!canConfigure} /> :
          selectedRule ? <div className="empty-state large"><strong>正在打开决策表…</strong><span>读取最新草稿与点位绑定。</span></div> :
          <div className="empty-state large"><strong>从第一张决策表开始</strong><span>选择多个输入条件，按顺序执行多个输出；第一行命中后停止。</span>{canConfigure && <span>点击右上方“新建决策表”。</span>}</div>}
      </section>}

      {page === "users" && isAdmin && <UsersView snapshot={view} groups={groups} busy={busy} run={run} />}
      {page === "audit" && isAdmin && <AuditView token={token} onError={setError} />}
      {page === "delivery" && isAdmin && <DeliveryView token={token} />}

      <footer className="console-foot">
        <span>数据来自已配置点位；未接入或过期的值显示“—”，不生成模拟数据。</span>
        <span>单机本地运行 · 不依赖云端</span>
      </footer>
      </div>
    </main>

    {page === "devices" && <>
      {!assistantOpen && <button className="assistant-fab" aria-label="打开能源管家" onClick={() => setAssistantOpen(true)}>
        <span>AI</span><Icon name="sparkle" size={12} />
      </button>}
      <AssistantPanel
        points={points} alarmCount={alarmCount} errors={errors.length} rules={rules.length} activeRules={activeRules.length}
        stationName={stationName} open={assistantOpen} onClose={() => setAssistantOpen(false)}
      />
    </>}

    {kpiConfigOpen && <KpiConfigPanel config={kpiConfig} onSave={(next) => { setKpiConfig(next); saveKpiConfig(next) }} onClose={() => setKpiConfigOpen(false)} />}
    {shiftOpen && <ShiftHandover onClose={() => setShiftOpen(false)} />}
    {commandsOpen && <CommonCommands token={token} canOperate={canOperate} onError={setError} onClose={() => setCommandsOpen(false)} />}
  </div></div></IonContent></IonApp>
}

type Run = (action: () => Promise<unknown>) => Promise<boolean>

function DevicesView({ snapshot, points, groups, rules, activeRules, errors, alarmCount, busy, isAdmin, canConfigure, canOperate, run, openPoint, goRules, goHistory, goChannels, goEnergyUnits, goDispatchTrace, goAlarms, dispatch, latestAlarm, tariff, kpiConfig, onError }: {
  snapshot: Snapshot; points: Node[]; groups: Node[]; rules: number; activeRules: number; errors: number; alarmCount: number
  busy: boolean; isAdmin: boolean; canConfigure: boolean; canOperate: boolean; run: Run; goRules: () => void; goHistory: () => void; goChannels: () => void; goEnergyUnits: () => void; goDispatchTrace: () => void; goAlarms: () => void; dispatch: DispatchInfo | null
  latestAlarm: EmsAlarm | null
  tariff: number | null
  kpiConfig: KpiConfig
  openPoint: (keywords: string[], fallback?: Page) => void
  onError: (message: string) => void
}) {
  const [showAdd, setShowAdd] = useState(false)
  const [kind, setKind] = useState("variable")
  const [description, setDescription] = useState("")
  const [initialValue, setInitialValue] = useState(0)
  const [parent, setParent] = useState(snapshot.roots[0])
  const [values, setValues] = useState<Record<string, number>>({})
  const [managedNode, setManagedNode] = useState<Node | null>(null)
  const [showDataNote, setShowDataNote] = useState(false)
  const [confirm, setConfirm] = useState<{ kind: "setpoint" | "manual"; node: Node; value?: number } | null>(null)
  const token = sessionStorage.getItem("easyems-token") || ""

  const activities = useMemo(() => snapshot.nodes.flatMap((node) => node.points
    .filter((item) => ["value", "valueSet", "applied", "enabled", "lastMatch"].includes(item.type))
    .map((item) => ({ node, item })))
    .sort((a, b) => b.item.time.localeCompare(a.item.time)).slice(0, 6), [snapshot])

  return <>
    <KpiStrip points={points} tariff={tariff} config={kpiConfig} onOpen={openPoint} />

    <EnergyFlow points={points} onOpenPoint={openPoint} onOpenTrends={goHistory} dispatch={dispatch} event={{
      activeRules, rules, alarmCount, errors, pointCount: points.length, onOpenRules: goRules,
      onOpenTrace: goDispatchTrace, onOpenAlarms: goAlarms,
      latestAlarm: latestAlarm ? { title: latestAlarm.title, severity: latestAlarm.severity, at: latestAlarm.created_at } : undefined,
    }} />

    <EnergyUnitsStatus nodes={snapshot.nodes} onOpen={goEnergyUnits} token={token} canOperate={canOperate} canConfigure={canConfigure} />

    <InlineNotice tone="info"><b>现场</b>：{showDataNote ? "所有指标均取自已配置点位；未接入、无值或过期显示“—”。能流画面只用于展示，不替代控制、保护或结算。" : "仅展示已绑定点位；未接入指标显示“—”，不生成模拟数据。"}<button className="ml-2 text-ems-accent" onClick={() => setShowDataNote(!showDataNote)}>说明 {showDataNote ? "收起" : "展开"}</button></InlineNotice>

    <section className="content-grid">
      <EmsCard><IonCardContent>
        <EmsSectionHeader kicker="设备树与点位" title="当前状态" actions={<>{isAdmin && <EmsButton variant="secondary" onClick={goChannels}>通道配置</EmsButton>}{isAdmin && <EmsButton variant="secondary" onClick={() => setShowAdd(!showAdd)}>{showAdd ? "收起" : "+ 添加"}</EmsButton>}</>} />
        {showAdd && isAdmin && <form className="add-node-form" onSubmit={(event) => {
          event.preventDefault()
          if (!description.trim()) { onError("请填写名称"); return }
          void run(async () => {
            await request(token, "/nodes", "POST", { parent, type: kind, description, value: kind === "variable" ? initialValue : undefined })
            setDescription("")
            setShowAdd(false)
          })
        }}>
          <div className="grid gap-3 sm:grid-cols-2"><EmsSelect label="类型" value={kind} onIonChange={(event) => setKind(event.detail.value)}><EmsSelectOption value="variable">测试变量</EmsSelectOption><EmsSelectOption value="group">分组</EmsSelectOption></EmsSelect><EmsSelect label="所属位置" value={parent} onIonChange={(event) => setParent(event.detail.value)}>{groups.map((group) => <EmsSelectOption key={group.id} value={group.id}>{snapshot.roots.includes(group.id) ? "根节点" : nameOf(group)}</EmsSelectOption>)}</EmsSelect></div>
          <div className="grid gap-3 sm:grid-cols-2"><EmsInput label="名称" value={description} onIonInput={(event) => setDescription(event.detail.value || "")} placeholder={kind === "variable" ? "例如：室温" : "例如：1 号电站"} />{kind === "variable" && <EmsInput label="初始值" type="number" value={initialValue} onIonInput={(event) => setInitialValue(Number(event.detail.value))} />}</div>
          {kind === "variable" && <p className="m-0 text-xs text-ems-muted">测试变量用于验证数据与规则，不代表真实物理设备。</p>}
          <EmsButton type="submit" disabled={busy}>添加到设备树</EmsButton>
        </form>}
        {isAdmin && <details className="modbus-setup"><summary>接入 Modbus TCP / RTU</summary><p className="form-note">端口、单元与点位已统一在“通道”页配置。</p><button className="secondary-button" onClick={goChannels}>打开通道配置</button></details>}
        <div className="device-list">
          {snapshot.nodes.filter((node) => isDomainNode(node.type) || node.type === "group" || node.type === "modbus" || node.type === "variable" || node.type === "modbusIo").map((node) => {
            if (isDomainNode(node.type)) {
              const domainTypeName = { site: "站点", plant: "电站", energyUnit: "储能柜", device: "设备" }[node.type] || node.type
              const domainIcon = { site: "站", plant: "电", energyUnit: "柜", device: "设" }[node.type] || "域"
              const rated = point(node, "ratedPowerKW")?.text
              return <div className="device-line domain" key={`${node.parent}/${node.id}`}>
                <div className={`device-icon ${node.type}`} aria-hidden="true">{domainIcon}</div>
                <div className="device-name"><strong>{nameOf(node)}</strong><span>{domainTypeName}{rated ? ` · 额定 ${rated} kW` : ""}</span></div>
              </div>
            }
            const value = point(node, "value")?.value
            const target = point(node, "valueSet")?.value
            const units = point(node, "units")?.text || ""
            const errorCount = point(node, "errorCount")?.value || 0
            const manual = (point(node, "easyemsManual")?.value || 0) !== 0
            const disabled = (point(node, "disabled")?.value || 0) !== 0
            const command = commandStatus(node)
            const currentValue = values[node.id] ?? value ?? 0
            const typeName = { group: "设备分组", modbus: "Modbus 连接", modbusIo: "Modbus 点位", variable: "测试变量" }[node.type]
            return <div className="device-line" key={`${node.parent}/${node.id}`}>
              <div className={`device-icon ${node.type}`} aria-hidden="true">{node.type === "group" ? "组" : node.type === "modbus" ? "总" : node.type === "modbusIo" ? "M" : "测"}</div>
              <div className="device-name"><strong>{nameOf(node)}</strong><span>{typeName}{node.type !== "group" && node.type !== "modbus" && ` · 当前 ${value ?? "—"}${units}`}{target !== undefined && ` · 目标 ${target}`}{errorCount > 0 && ` · 通信错误 ${errorCount}`}</span></div>
              {manual && <StatusBadge tone="warning">人工接管</StatusBadge>}
              {disabled && <StatusBadge tone="neutral">已停用</StatusBadge>}
              {command && <StatusBadge tone={command === "confirmed" ? "success" : command === "failed" || command === "unknown" ? "danger" : "info"}>{({ accepted: "已接受", confirmed: "已回读", unknown: "结果未知", failed: "失败" } as Record<string, string>)[command] || command}</StatusBadge>}
              {isAdmin && <EmsButton variant="quiet" size="small" onClick={() => setManagedNode(node)}>管理</EmsButton>}
              {node.type === "modbusIo" && !writable(node) && <StatusBadge tone="neutral">只读</StatusBadge>}
              {(node.type === "variable" || writable(node)) && canOperate && !disabled && <div className="device-control"><EmsInput aria-label={`${nameOf(node)}操作值`} type="number" value={currentValue} onIonInput={(event) => setValues({ ...values, [node.id]: Number(event.detail.value) })} />
                {node.type === "variable" && <EmsButton variant="secondary" size="small" disabled={busy} onClick={() => void run(() => request(token, `/nodes/${node.id}/value`, "POST", { value: currentValue, point_type: "value" }))}>更新当前值</EmsButton>}
                <EmsButton variant="secondary" size="small" disabled={busy} onClick={() => setConfirm({ kind: "setpoint", node, value: currentValue })}>下发目标值</EmsButton>
                {manual
                  ? <EmsButton variant="secondary" size="small" disabled={busy} onClick={() => void run(() => request(token, `/nodes/${node.id}/manual`, "POST", { manual: false }))}>恢复自动</EmsButton>
                  : <EmsButton variant="quiet" size="small" disabled={busy} onClick={() => setConfirm({ kind: "manual", node })}>接管自动</EmsButton>}
              </div>}
            </div>
          })}
          {!snapshot.nodes.some((node) => isDomainNode(node.type) || node.type === "group" || node.type === "modbus" || node.type === "variable" || node.type === "modbusIo") && <EmptyState title="设备树还是空的" description="管理员可添加分组、测试变量；Modbus 端口与点位在“通道”页配置。" />}
        </div>
      </IonCardContent></EmsCard>

      <EmsCard><IonCardContent><EmsSectionHeader kicker="真实操作记录" title="最近变化" />
        <div className="activity-list">{activities.map(({ node, item }, index) => <div className="activity" key={`${node.id}/${item.type}/${item.time}/${index}`}><span className="activity-dot" /><div><strong>{nameOf(node)}</strong><span>{{ value: "当前值更新", valueSet: "目标值下发", applied: "规则发布", enabled: "自动运行切换", lastMatch: "规则命中" }[item.type]} · {activityDetail(item)}</span></div><time>{new Date(item.time).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}</time></div>)}
          {activities.length === 0 && <EmptyState title="尚无设备或规则变化" description="设备值更新、规则发布与人工操作会显示在这里。" />}</div>
      </IonCardContent></EmsCard>
    </section>
    {managedNode && <NodeManager key={managedNode.id} token={token} node={snapshot.nodes.find((node) => node.id === managedNode.id) || managedNode} snapshot={snapshot} run={run} onClose={() => setManagedNode(null)} />}
    {confirm && <ConfirmDialog
      title={confirm.kind === "manual" ? "人工接管" : "下发目标值"}
      message={confirm.kind === "manual"
        ? `接管后「${nameOf(confirm.node)}」将停止自动调度，自动策略不再控制它，直到你显式恢复自动。确定要接管吗？`
        : `将把「${nameOf(confirm.node)}」的功率目标设为 ${confirm.value} kW，设备会立即按此调节。确认下发？`}
      danger={confirm.kind === "manual"}
      confirmLabel={confirm.kind === "manual" ? "确认接管" : "确认下发"}
      busy={busy}
      onCancel={() => setConfirm(null)}
      onConfirm={() => {
        const target = confirm
        setConfirm(null)
        void run(async () => {
          if (target.kind === "manual") await request(token, `/nodes/${target.node.id}/manual`, "POST", { manual: true })
          else await request(token, `/nodes/${target.node.id}/value`, "POST", { value: target.value, point_type: "valueSet" })
        })
      }}
    />}
  </>
}

function ChannelWorkspace({ token, channels, ioCounts, isAdmin, canOperate, nodes, roots, groups, templates, busy, run, onError, goPoints }: {
  token: string; channels: Node[]; ioCounts: Map<string, number>; isAdmin: boolean; canOperate: boolean; nodes: Node[]; roots: string[]; groups: Node[]; templates: Node[]
  busy: boolean; run: Run; onError: (message: string) => void; goPoints: () => void
}) {
  const [tab, setTab] = useState<"status" | "config" | "tools">("status")
  const [frameChannel, setFrameChannel] = useState<Node | null>(null)
  const [quality, setQuality] = useState<Record<string, QualityStats>>({})
  useEffect(() => {
    if (tab !== "status") return
    let cancelled = false
    const load = async () => {
      try {
        const result = await request<{ channels: QualityStats[] }>(token, "/ems/quality")
        if (!cancelled) setQuality(Object.fromEntries((result.channels || []).map((item) => [item.channel_id, item])))
      } catch {
        // Quality is supplementary; channel health still renders from the node.
      }
    }
    void load()
    const timer = window.setInterval(() => void load(), 2000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [tab, token])
  const exportReport = async () => {
    try {
      const [qualityResult, stateResult] = await Promise.all([
        request<{ channels: QualityStats[]; devices: QualityStats[] }>(token, "/ems/quality"),
        request<{ values: { device_id: string; quality: string }[] }>(token, "/ems/state"),
      ])
      const channelName = (id: string) => { const node = channels.find((item) => item.id === id); return node ? nameOf(node) : id }
      const pct = (item: QualityStats) => item.polls > 0 ? item.success_rate.toFixed(1) : ""
      const ms = (value: number) => value > 0 ? Math.round(value).toString() : ""
      const timeOf = (value?: string) => value && !value.startsWith("0001") ? new Date(value).toLocaleString() : ""
      const rows: string[][] = [
        ["通信测试报告"],
        ["导出时间", new Date().toLocaleString()],
        ["说明", "统计为本次运行期间的轮询计数，进程重启后清零；交付验收时附在验证记录后"],
        [],
        ["通道", "轮询次数", "失败次数", "成功率%", "平均延迟ms", "最大延迟ms", "最近成功", "最近错误"],
      ]
      for (const item of qualityResult.channels || []) {
        rows.push([channelName(item.channel_id), String(item.polls), String(item.failures), pct(item), ms(item.avg_latency_ms), ms(item.max_latency_ms), timeOf(item.last_success), item.last_error || ""])
      }
      rows.push([], ["设备", "通道", "健康", "轮询次数", "失败次数", "成功率%", "平均延迟ms", "最大延迟ms", "最近成功", "最近错误"])
      const pointCounts: Record<string, Record<string, number>> = {}
      for (const value of stateResult.values || []) {
        const bucket = pointCounts[value.device_id] || (pointCounts[value.device_id] = {})
        bucket[value.quality] = (bucket[value.quality] || 0) + 1
      }
      for (const item of qualityResult.devices || []) {
        rows.push([item.device_id || "", channelName(item.channel_id), item.health || "", String(item.polls), String(item.failures), pct(item), ms(item.avg_latency_ms), ms(item.max_latency_ms), timeOf(item.last_success), item.last_error || ""])
      }
      rows.push([], ["设备点位质量", "正常", "过期", "无效", "未初始化"])
      for (const [deviceID, bucket] of Object.entries(pointCounts)) {
        rows.push([deviceID, String(bucket.good || 0), String(bucket.stale || 0), String(bucket.invalid || 0), String(bucket.uninitialized || 0)])
      }
      const csv = rows.map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(",")).join(String.fromCharCode(10))
      const url = URL.createObjectURL(new Blob([String.fromCharCode(0xFEFF) + csv], { type: "text/csv;charset=utf-8" }))
      const link = document.createElement("a")
      link.href = url
      link.download = `easyems-通信报告-${new Date().toISOString().slice(0, 10)}.csv`
      link.click()
      URL.revokeObjectURL(url)
    } catch {
      onError("通信报告导出失败：质量统计接口不可用")
    }
  }
  return <section className="channel-workspace grid gap-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <SegmentedTabs ariaLabel="通道" value={tab} onChange={setTab} options={[{ value: "status", label: "运行状态" }, { value: "config", label: "端口配置" }, { value: "tools", label: "点表与模板" }]} />
      <div className="flex flex-wrap gap-2">
        {tab === "status" && <EmsButton variant="secondary" onClick={() => void exportReport()}>通信报告</EmsButton>}
        <EmsButton variant="secondary" onClick={goPoints}>点位表</EmsButton>
      </div>
    </div>
    {tab === "status" ? <ChannelsView channels={channels} ioCounts={ioCounts} isAdmin={isAdmin} canOperate={canOperate} quality={quality} goConfigure={() => setTab("config")} onFrames={setFrameChannel} onRestart={(id) => void run(() => request(token, `/ems/channels/${id}/restart`, "POST", {}))} /> :
      tab === "config" ? <CommunicationView token={token} nodes={nodes} roots={roots} groups={groups} canConfigure={isAdmin} canOperate={canOperate} run={run} onError={onError} goPoints={goPoints} /> :
        <ModbusTools token={token} buses={channels} templates={templates} busy={busy} run={run} />}
    <FrameDrawer token={token} channel={frameChannel} open={Boolean(frameChannel)} onClose={() => setFrameChannel(null)} onError={onError} />
  </section>
}

function ChannelsView({ channels, ioCounts, isAdmin, canOperate, quality, goConfigure, onFrames, onRestart }: {
  channels: Node[]; ioCounts: Map<string, number>; isAdmin: boolean; canOperate: boolean; quality: Record<string, QualityStats>; goConfigure: () => void; onFrames: (node: Node) => void; onRestart: (id: string) => void
}) {
  const rows = channels.map((node) => ({ node, state: channelState(node) }))
  const online = rows.filter((row) => row.state.key === "online").length
  const degraded = rows.filter((row) => row.state.key === "degraded").length
  const offline = rows.filter((row) => row.state.key === "offline").length
  return <section className="grid gap-4">
    <div className="ems-metric-strip">
      <div className="ems-metric ems-metric-success"><span className="ems-metric-label">在线</span><strong className="ems-metric-value">{online}</strong></div>
      <div className={`ems-metric ${degraded ? "ems-metric-warning" : ""}`}><span className="ems-metric-label">不稳定</span><strong className="ems-metric-value">{degraded}</strong></div>
      <div className={`ems-metric ${offline ? "ems-metric-danger" : ""}`}><span className="ems-metric-label">离线</span><strong className="ems-metric-value">{offline}</strong></div>
      <div className="ems-metric"><span className="ems-metric-label">通信配置</span><EmsButton variant="quiet" size="small" onClick={goConfigure}>{isAdmin ? "端口、单元与点位" : "查看端口配置"} →</EmsButton></div>
    </div>
    <div className="channel-grid">
      {rows.map(({ node, state }) => {
        const protocol = point(node, "protocol")?.text || "Modbus"
        const path = protocol === "TCP" ? point(node, "uri")?.text : point(node, "port")?.text
        const poll = point(node, "pollPeriod")?.value
        const baud = point(node, "baud")?.text || (point(node, "baud")?.value ? String(point(node, "baud")?.value) : "")
        const tone = state.key === "online" ? "success" : state.key === "degraded" ? "warning" : "danger"
        return <EmsCard key={node.id}>
          <IonCardContent className="grid gap-3">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div><StatusBadge tone="info" dot={false}>{protocol}</StatusBadge><h3 className="mb-0 mt-1 text-base font-semibold text-ems-ink">{nameOf(node)}</h3></div>
              <StatusBadge tone={tone}>{state.label}</StatusBadge>
            </div>
            <p className="m-0 break-all text-xs text-ems-muted">{path || "未设置通信地址"}</p>
            <div className="grid grid-cols-3 gap-2">
              <div><span className="block text-xs text-ems-muted">点位</span><strong className="text-lg text-ems-ink">{ioCounts.get(node.id) || 0}</strong></div>
              <div><span className="block text-xs text-ems-muted">轮询周期</span><strong className="text-lg text-ems-ink">{poll ? `${poll} ms` : "—"}</strong></div>
              <div><span className="block text-xs text-ems-muted">{protocol === "RTU" ? "波特率" : "协议"}</span><strong className="text-lg text-ems-ink">{protocol === "RTU" ? (baud || "—") : "TCP"}</strong></div>
            </div>
            {(() => {
              const q = quality[node.id] as (QualityStats & { window_5m?: { success_rate: number; polls: number; errors?: Record<string, number> } }) | undefined
              const win = q?.window_5m
              const topError = win?.errors ? Object.entries(win.errors).sort((a, b) => b[1] - a[1])[0] : undefined
              return <div className="flex flex-wrap gap-3 text-xs text-ems-muted">
                <span>{q ? `${q.success_rate.toFixed(1)}% 成功率` : "— 成功率"}</span>
                <span>{q && q.polls > 0 ? `${Math.round(q.avg_latency_ms)} ms 平均` : "— 延迟"}</span>
                <span className={q && q.failures > 0 ? "text-ems-amber" : ""}>{q ? `${q.failures} 错误` : "— 错误"}</span>
                <span>近 5 分钟 {win && win.polls > 0 ? `${win.success_rate.toFixed(0)}%` : "—"}</span>
                {topError && <span className="text-ems-amber">{topError[0]} ×{topError[1]}</span>}
              </div>
            })()}
            <div className="flex flex-wrap items-center justify-between gap-2 border-t border-ems-line pt-2">
              <span className="text-xs text-ems-muted">{state.detail}</span>
              <div className="flex flex-wrap gap-1">
                {isAdmin && <EmsButton variant="quiet" size="small" onClick={() => onRestart(node.id)}>重启</EmsButton>}
                {canOperate && <EmsButton variant="quiet" size="small" onClick={() => onFrames(node)}>报文</EmsButton>}
                <EmsButton variant="secondary" size="small" onClick={goConfigure}>{isAdmin ? "配置端口" : "查看端口"}</EmsButton>
              </div>
            </div>
          </IonCardContent>
        </EmsCard>
      })}
    </div>
    {channels.length === 0 && <EmsCard><IonCardContent><EmptyState title="还没有通信通道" description="先新增一个 Modbus TCP 或 RTU 通道，接入第一台设备。" action={<EmsButton onClick={goConfigure}>+ 新增通道</EmsButton>} /></IonCardContent></EmsCard>}
    <InlineNotice tone="info"><strong>状态口径</strong>：在线表示最近一次完整轮询仍在时效内；不稳定表示有通信错误但仍有成功轮询；离线表示超过时效或从未成功。端口参数、单元与点位在同一工作区的“端口配置”中维护。</InlineNotice>
  </section>
}

function toggleAction(list: string[], key: string) {
  return list.includes(key) ? list.filter((item) => item !== key) : [...list, key]
}

function UsersView({ snapshot, groups, busy, run }: { snapshot: Snapshot; groups: Node[]; busy: boolean; run: Run }) {
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [role, setRole] = useState("viewer")
  const [parent, setParent] = useState(snapshot.roots[0])
  const [actions, setActions] = useState<string[]>([])
  const [accounts, setAccounts] = useState<UserAccount[]>([])
  const [notice, setNotice] = useState("")
  const token = sessionStorage.getItem("easyems-token") || ""

  const load = useCallback(async () => {
    setAccounts((await listUsers(token)).users || [])
  }, [token])

  useEffect(() => {
    void load().catch(() => setAccounts([]))
  }, [load])

  return <section className="panel users-panel"><div className="panel-heading"><div><span className="section-kicker">动作级权限</span><h2>账号与权限</h2><p>角色是默认动作集的快捷方式；勾选动作白名单后按白名单覆盖角色默认，未勾选的动作一律拒绝。</p></div></div>
    {notice && <div className="success-message" role="status">{notice}</div>}
    <form className="user-form" onSubmit={(event) => {
      event.preventDefault()
      void run(async () => {
        await request(token, "/users", "POST", { parent, email, password, role, actions })
        setEmail("")
        setPassword("")
        setActions([])
        setNotice("已创建账号")
        await load()
      })
    }}><label>登录账号<input value={email} onChange={(event) => setEmail(event.target.value)} required /></label><label>初始密码<input type="password" autoComplete="new-password" minLength={12} value={password} onChange={(event) => setPassword(event.target.value)} required /><small>至少 12 位，不写入导出或日志。</small></label><label>角色<select value={role} onChange={(event) => setRole(event.target.value)}>{Object.entries(roleNames).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label><label>可见与操作范围<select value={parent} onChange={(event) => setParent(event.target.value)}>{groups.map((group) => <option key={group.id} value={group.id}>{snapshot.roots.includes(group.id) ? "整个设备树" : nameOf(group)}</option>)}</select></label><fieldset className="permission-actions"><legend>动作白名单（留空 = 按角色默认）</legend>{permissionActions.map((action) => <label key={action.key} className="comm-check"><input type="checkbox" checked={actions.includes(action.key)} onChange={() => setActions(toggleAction(actions, action.key))} />{action.label}</label>)}</fieldset><button className="primary-button" disabled={busy}>创建账号</button></form>
    <div className="user-list">
      {accounts.length === 0 && <p className="inline-note">尚未创建其他账号。</p>}
      {accounts.map((account) => <UserRow key={account.id} account={account} token={token} busy={busy} run={run} onSaved={() => void load()} />)}
    </div>
    <div className="permission-note"><strong>服务端权限边界</strong><p>通用 Simple IoT 节点接口与工程界面未开放；每个请求都检查当前账号、动作与树范围，未知动作默认拒绝并记审计。</p></div>
  </section>
}

function UserRow({ account, token, busy, run, onSaved }: { account: UserAccount; token: string; busy: boolean; run: Run; onSaved: () => void }) {
  const [role, setRole] = useState(account.policy.role || "viewer")
  const [actions, setActions] = useState<string[]>(account.policy.actions || [])
  const [newPassword, setNewPassword] = useState("")
  const [showReset, setShowReset] = useState(false)
  const [notice, setNotice] = useState("")
  return <div className="user-row">
    <div className="user-row-head"><strong>{account.email}</strong><span>{roleNames[role] || role}{account.policy.admin ? " · 管理员" : ""}</span></div>
    <label>角色<select value={role} onChange={(event) => setRole(event.target.value)}>{Object.entries(roleNames).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
    <fieldset className="permission-actions"><legend>动作白名单</legend>{permissionActions.map((action) => <label key={action.key} className="comm-check"><input type="checkbox" checked={actions.includes(action.key)} onChange={() => setActions(toggleAction(actions, action.key))} />{action.label}</label>)}</fieldset>
    {notice && <div className="success-message" role="status">{notice}</div>}
    <div className="user-row-actions">
      <button className="secondary-button" disabled={busy} onClick={() => void run(async () => { await updateUserPolicy(token, account.id, role, actions); setNotice("已保存权限"); onSaved() })}>保存权限</button>
      <button className="secondary-button danger-text" disabled={busy} onClick={() => setShowReset(!showReset)}>重置密码</button>
    </div>
    {showReset && <div className="user-reset"><label>新密码<input type="password" autoComplete="new-password" minLength={12} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} /><small>至少 12 位，仅写入本机账号，不记审计/导出。</small></label><button className="secondary-button" disabled={busy || newPassword.length < 12} onClick={() => void run(async () => { await resetUserPassword(token, account.id, newPassword); setNewPassword(""); setShowReset(false); setNotice("已重置密码") })}>确认重置</button></div>}
  </div>
}
