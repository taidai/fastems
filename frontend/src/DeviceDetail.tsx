import { useCallback, useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { EmsButton, EmsOverlay, InlineNotice } from "./components/ems"
import { ackAlarm, closeAlarm, downloadCSV, EmsAlarm, HistoryResponse, request } from "./api"
import AckDialog from "./AckDialog"
import type { CommandOutcome, LedgerDevice, StateValue, UnitCommand, UnitModel } from "./deviceTypes"

type Tab = "overview" | "points" | "commands" | "history" | "alarms" | "verify"

const healthNames: Record<string, string> = { healthy: "在线", degraded: "不稳定", offline: "离线", unknown: "未知" }
const qualityNames: Record<string, string> = { good: "正常", stale: "过期", invalid: "无效", uninitialized: "无值" }
const stateNames: Record<string, string> = {
  queued: "排队", running: "执行中", confirmed: "已确认", failed: "失败", timeout: "超时", unknown: "结果未知",
}
const roleNames: Record<string, string> = { info: "提示", warning: "警告", critical: "严重" }
const pointRoleNames: Record<string, string> = { telemetry: "遥测", status: "遥信", energy: "电度", setpoint: "定值", protection: "压板" }

function formatNumber(value?: number) {
  if (value === undefined || !Number.isFinite(value)) return "—"
  return Math.abs(value) >= 1000 ? value.toFixed(0) : value.toFixed(2).replace(/\.?0+$/, "")
}

function formatValue(value?: StateValue) {
  if (!value) return "—"
  if (value.value.kind === "number") return formatNumber(value.value.number)
  if (value.value.kind === "bool") return value.value.bool ? "合" : "分"
  if (value.value.kind === "enum") return value.value.enum || "—"
  return "—"
}

function timeText(value?: string) {
  if (!value) return "—"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString("zh-CN", { hour12: false })
}

function isTerminal(state: string) {
  return ["confirmed", "failed", "timeout", "unknown"].includes(state)
}

export default function DeviceDetail({ token, device, canConfigure, canOperate, onClose, onError, goChannels, goAlarms }: {
  token: string
  device: LedgerDevice
  canConfigure: boolean
  canOperate: boolean
  onClose: () => void
  onError: (message: string) => void
  goChannels: () => void
  goAlarms: () => void
}) {
  const base = `/modbus/${encodeURIComponent(device.channel_id)}/units/${device.server_id}`
  const deviceID = `${device.channel_id}/${device.server_id}`
  const [tab, setTab] = useState<Tab>("overview")
  const [model, setModel] = useState<UnitModel | null>(null)
  const [values, setValues] = useState<Record<string, StateValue>>({})
  const [alarms, setAlarms] = useState<EmsAlarm[]>([])
  const [outcomes, setOutcomes] = useState<CommandOutcome[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [ackTarget, setAckTarget] = useState<EmsAlarm | null>(null)
  const [error, setError] = useState("")
  const [query, setQuery] = useState("")
  const [paramValues, setParamValues] = useState<Record<string, Record<string, number>>>({})
  const [pending, setPending] = useState<{ command: UnitCommand; params: Record<string, number> } | null>(null)
  const [presets, setPresets] = useState<Record<string, Record<string, number>>>(() => {
    try { return JSON.parse(localStorage.getItem(`fastiot-command-presets:${deviceID}`) || "{}") } catch { return {} }
  })
  const [preview, setPreview] = useState<UnitCommand | null>(null)
  const [historyPoint, setHistoryPoint] = useState("")
  const [historyHours, setHistoryHours] = useState(24)
  const [history, setHistory] = useState<HistoryResponse | null>(null)
  const [historyLoading, setHistoryLoading] = useState(false)
  const [verified, setVerified] = useState(false)
  const [verifier, setVerifier] = useState("")
  const [verifiedAt, setVerifiedAt] = useState("")
  const [deviceSerial, setDeviceSerial] = useState("")
  const [verification, setVerification] = useState("")

  const report = useCallback((cause: unknown, fallback: string) => {
    const message = cause instanceof Error ? cause.message : fallback
    setError(message)
    onError(message)
  }, [onError])

  const loadModel = useCallback(async () => {
    try {
      const result = await request<UnitModel>(token, `${base}/model`)
      const commands = result.commands || []
      const points = result.points || []
      setModel({ ...result, commands, points, profiles: result.profiles || [] })
      setHistoryPoint((current) => current || points[0]?.id || "")
      setParamValues(Object.fromEntries(commands.map((command) => [
        command.id,
        Object.fromEntries((command.parameters || []).map((parameter) => [parameter.name, parameter.default])),
      ])))
    } catch (cause) {
      report(cause, "设备模型读取失败")
    }
  }, [base, report, token])

  const loadLive = useCallback(async () => {
    try {
      const [stateResult, alarmResult, commandResult] = await Promise.all([
        request<{ values: StateValue[] }>(token, "/ems/state"),
        request<{ alarms: EmsAlarm[] }>(token, "/ems/alarms"),
        request<{ commands: CommandOutcome[] }>(token, `${base}/commands`),
      ])
      setValues(Object.fromEntries((stateResult.values || []).filter((value) => value.device_id === deviceID).map((value) => [value.point_id, value])))
      setAlarms(alarmResult.alarms || [])
      setOutcomes(commandResult.commands || [])
    } catch (cause) {
      report(cause, "设备状态读取失败")
    }
  }, [base, deviceID, report, token])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    void loadModel().finally(() => { if (!cancelled) setLoading(false) })
    void loadLive()
    const timer = window.setInterval(() => void loadLive(), 2000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [loadLive, loadModel])

  useEffect(() => {
    if (!model) return
    const config = model.model
    setVerified(Boolean(config?.verified))
    setVerifier(config?.verifier || "")
    setVerifiedAt(config?.verified_at || "")
    setDeviceSerial(config?.device_serial || "")
    setVerification(config?.verification || "")
  }, [model])

  const profile = useMemo(() => model?.profiles.find((item) => item.id === model?.model?.profile_id), [model])
  const rolesByPointID = useMemo(() => {
    const specRoles = new Map((profile?.point_specs || []).map((spec) => [spec.name, spec.role || ""]))
    const roles: Record<string, string> = {}
    for (const [specName, pointID] of Object.entries(model?.model?.point_map || {})) {
      const role = specRoles.get(specName)
      if (pointID && role) roles[pointID] = role
    }
    return roles
  }, [model, profile])
  const modelName = model?.model?.name || ""
  const businessName = device.business_node_name || model?.model?.business_node_id || ""
  const displayName = businessName || (modelName && modelName !== `站号 ${device.server_id}` ? modelName : `${device.channel_name} · 站号 ${device.server_id}`)
  const points = model?.points || []
  const mappedPointIDs = useMemo(() => new Set(Object.values(model?.model?.point_map || {})), [model])
  const pointIDs = useMemo(() => new Set(points.map((point) => point.id)), [points])
  const deviceAlarms = useMemo(() => alarms.filter((alarm) =>
    alarm.device_id === deviceID || pointIDs.has(alarm.device_id || "") || pointIDs.has(alarm.point_id || "")
  ), [alarms, deviceID, pointIDs])
  const activeAlarms = deviceAlarms.filter((alarm) => alarm.active)
  const activeCommand = outcomes.find((outcome) => !isTerminal(outcome.state))
  const badPoints = points.filter((point) => {
    const value = values[point.id]
    return value && value.quality !== "good"
  }).length

  const shownPoints = useMemo(() => {
    const text = query.trim().toLowerCase()
    if (!text) return points
    return points.filter((point) => `${point.name} ${point.address} ${point.io_type}`.toLowerCase().includes(text))
  }, [points, query])

  const loadHistory = useCallback(async () => {
    if (!historyPoint) { setHistory(null); return }
    setHistoryLoading(true)
    try {
      const end = new Date()
      const start = new Date(end.getTime() - historyHours * 3600_000)
      setHistory(await request<HistoryResponse>(token, `/history?node_id=${encodeURIComponent(historyPoint)}&type=value&start=${encodeURIComponent(start.toISOString())}&end=${encodeURIComponent(end.toISOString())}&limit=500`))
    } catch (cause) {
      report(cause, "历史读取失败")
    } finally {
      setHistoryLoading(false)
    }
  }, [historyHours, historyPoint, report, token])

  useEffect(() => {
    if (tab === "history") void loadHistory()
  }, [loadHistory, tab])

  const commandParams = (command: UnitCommand) => paramValues[command.id] || Object.fromEntries((command.parameters || []).map((parameter) => [parameter.name, parameter.default]))

  function savePreset(command: UnitCommand) {
    const next = { ...presets, [command.id]: commandParams(command) }
    setPresets(next)
    localStorage.setItem(`fastiot-command-presets:${deviceID}`, JSON.stringify(next))
  }

  function applyPreset(command: UnitCommand) {
    const preset = presets[command.id]
    if (!preset) return
    setParamValues((current) => ({ ...current, [command.id]: { ...commandParams(command), ...preset } }))
  }

  const commandTargets = (command: UnitCommand) => {
    const params = commandParams(command)
    const targets: { pointID: string; name: string; value: number }[] = []
    if (command.point_id) targets.push({ pointID: command.point_id, name: command.point_name, value: command.value })
    for (const parameter of command.parameters || []) {
      if (parameter.point_id) targets.push({ pointID: parameter.point_id, name: parameter.label || parameter.point_name, value: params[parameter.name] ?? parameter.default })
    }
    return targets
  }

  async function execute(command: UnitCommand, params: Record<string, number>) {
    if (!canOperate) { setError("当前账号没有操作权限"); return }
    setBusy(true)
    setError("")
    try {
      const requestID = globalThis.crypto?.randomUUID?.() || `cmd-${Date.now()}-${Math.random().toString(16).slice(2)}`
      const outcome = await request<CommandOutcome>(token, `${base}/commands`, "POST", {
        request_id: requestID, command_id: command.id, parameters: params,
      })
      setOutcomes((current) => [outcome, ...current.filter((item) => item.request_id !== outcome.request_id)])
      setPending(null)
      for (let i = 0; i < 12; i += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 500))
        const result = await request<{ commands: CommandOutcome[] }>(token, `${base}/commands`)
        setOutcomes(result.commands || [])
        const latest = (result.commands || []).find((item) => item.request_id === requestID)
        if (latest && isTerminal(latest.state)) break
      }
    } catch (cause) {
      report(cause, "命令执行失败")
    } finally {
      setBusy(false)
    }
  }

  async function saveVerification() {
    if (!model?.model || !model.model.profile_id) { setError("先绑定 Profile 才能记录验证"); return }
    if (verified && (!verifier.trim() || !verifiedAt || !deviceSerial.trim())) {
      setError("标记已验证时必须填写验证人、日期和设备序列号")
      return
    }
    setBusy(true)
    try {
      const config = {
        ...model.model,
        verified,
        verifier: verifier.trim(),
        verified_at: verifiedAt,
        device_serial: deviceSerial.trim(),
        verification: verification.trim(),
      }
      const updated = await request<UnitModel>(token, `${base}/model`, "PUT", config)
      setModel(updated)
      setError("")
    } catch (cause) {
      report(cause, "验证记录保存失败")
    } finally {
      setBusy(false)
    }
  }

  async function settleAlarm(alarm: EmsAlarm, action: "ack" | "close") {
    if (action === "ack") { setAckTarget(alarm); return }
    setBusy(true)
    try {
      await closeAlarm(token, alarm.id)
      await loadLive()
    } catch (cause) {
      report(cause, "报警关闭失败")
    } finally {
      setBusy(false)
    }
  }

  const requiredMissing = model?.missing || []
  const commandsReady = Boolean(model?.commands.length) && (model?.commands || []).every((command) => command.available)
  const mappedCount = points.filter((point) => mappedPointIDs.has(point.id)).length
  const latestOutcome = outcomes[0]

  return createPortal(<>
    <div className="device-workspace-backdrop" onClick={onClose} />
    <section className="device-workspace" role="dialog" aria-label="设备工作区">
      <header className="device-workspace-head">
        <div>
          <span className="section-kicker">设备工作区</span>
          <h2>{displayName}</h2>
          <p>{device.channel_name} / 站号 {device.server_id} · {device.protocol} · {device.address || "未设置地址"}</p>
        </div>
        <div className="device-head-status">
          <span className={`ledger-health ${device.health}`}>{healthNames[device.health] || device.health}</span>
          <span className={device.verified ? "ledger-verified" : "ledger-unverified"}>{device.verified ? "已验证" : "未验证"}</span>
          {activeAlarms.length > 0 && <span className="device-alarm-chip">{activeAlarms.length} 条活动报警</span>}
          <button className="icon-button" aria-label="关闭" onClick={onClose}>×</button>
        </div>
      </header>

      <nav className="device-tabs" role="tablist" aria-label="设备详情">
        {([
          ["overview", "概览"], ["points", `点位 ${points.length}`], ["commands", `命令 ${model?.commands.length || 0}`],
          ["history", "历史"], ["alarms", `报警 ${activeAlarms.length}`], ["verify", "验证"],
        ] as [Tab, string][]).map(([key, label]) => <button key={key} role="tab" aria-selected={tab === key} className={tab === key ? "active" : ""} onClick={() => setTab(key)}>{label}</button>)}
      </nav>

      {error && <div className="device-inline-error" role="alert">{error}<button onClick={() => setError("")}>关闭</button></div>}

      <div className="device-workspace-body">
        {loading && <div className="empty-state"><strong>正在读取设备模型…</strong></div>}

        {!loading && tab === "overview" && <div className="device-overview">
          <div className="device-kpis">
            <div className="device-kpi"><span>通信</span><strong className={device.health === "healthy" ? "good" : device.health === "offline" ? "bad" : "warn"}>{healthNames[device.health] || device.health}</strong><small>{device.success_rate > 0 ? `${device.success_rate.toFixed(0)}% · ${Math.round(device.avg_latency_ms)}ms` : "暂无通信统计"}</small></div>
            <div className="device-kpi"><span>点位</span><strong>{points.length}</strong><small>{badPoints ? `${badPoints} 个质量异常` : "质量正常"}</small></div>
            <div className="device-kpi"><span>命令</span><strong>{model?.commands.length || 0}</strong><small>{commandsReady ? "全部可用" : "存在未映射或只读"}</small></div>
            <div className="device-kpi"><span>报警</span><strong className={activeAlarms.length ? "bad" : ""}>{activeAlarms.length}</strong><small>{deviceAlarms.length} 条历史</small></div>
          </div>

          <div className="device-overview-grid">
            <section className="device-panel">
              <div className="device-panel-head"><h3>设备身份</h3><button className="secondary-button" onClick={goChannels}>端口与设备模型</button></div>
              <dl className="device-facts">
                <div><dt>业务名称</dt><dd>{businessName || <span className="device-missing">未绑定业务节点</span>}</dd></div>
                <div><dt>Profile</dt><dd>{profile ? `${profile.vendor} ${profile.model}` : <span className="device-missing">未绑定 Profile</span>}</dd></div>
                <div><dt>设备序列号</dt><dd>{model?.model?.device_serial || "—"}</dd></div>
                <div><dt>通讯地址</dt><dd>{device.address || "—"}</dd></div>
                <div><dt>点表映射</dt><dd>{mappedCount} / {points.length}{requiredMissing.length ? ` · 缺 ${requiredMissing.join("、")}` : ""}</dd></div>
                <div><dt>最近命令</dt><dd>{latestOutcome ? `${stateNames[latestOutcome.state] || latestOutcome.state}${latestOutcome.reason ? ` · ${latestOutcome.reason}` : ""}` : "—"}</dd></div>
              </dl>
              {!profile && <button className="primary-button" onClick={goChannels}>去通道绑定 Profile</button>}
            </section>

            <section className="device-panel">
              <div className="device-panel-head"><h3>快捷操作</h3></div>
              <div className="device-quick-actions">
                <button className="secondary-button" onClick={() => setTab("commands")}>执行命令</button>
                <button className="secondary-button" onClick={() => setTab("history")}>查看趋势</button>
                <button className="secondary-button" onClick={() => setTab("verify")}>现场验证</button>
                <button className="secondary-button" onClick={goAlarms}>打开报警页</button>
              </div>
              <p className="device-note">命令会写入设备并回读确认；离线、点位未映射或只读时会阻止执行。</p>
            </section>

            <section className="device-panel device-recent-alarms">
              <div className="device-panel-head"><h3>最近报警</h3><button className="secondary-button" onClick={() => setTab("alarms")}>全部</button></div>
              {deviceAlarms.slice(0, 3).map((alarm) => <div className="device-alarm-row" key={alarm.id}><span className={`device-severity ${alarm.severity}`}>{roleNames[alarm.severity] || alarm.severity}</span><strong>{alarm.title}</strong><small>{timeText(alarm.updated_at)} · {alarm.state}</small></div>)}
              {deviceAlarms.length === 0 && <div className="empty-state"><span>该设备暂无报警。</span></div>}
            </section>
          </div>
        </div>}

        {!loading && tab === "points" && <div className="device-tab-page">
          <div className="device-toolbar"><input aria-label="搜索点位" placeholder="搜索点位名称、地址或类型" value={query} onChange={(event) => setQuery(event.target.value)} /><span>已映射 {mappedCount} / {points.length}</span></div>
          <div className="device-table-wrap"><table className="device-table"><thead><tr><th>点位</th><th>角色</th><th>地址</th><th>类型</th><th>读/写</th><th>缩放 / 单位</th><th>当前值</th><th>质量</th><th>更新时间</th><th>映射</th></tr></thead><tbody>
            {shownPoints.map((point) => {
              const value = values[point.id]
              const role = rolesByPointID[point.id]
              return <tr key={point.id}>
                <td><strong>{point.name || point.id}</strong><small>{point.id}</small></td>
                <td>{role ? <span className={`point-role ${role}`}>{pointRoleNames[role] || role}</span> : "—"}</td>
                <td>{point.address}</td><td>{point.io_type || "—"}</td><td>{point.read_only ? "只读" : "可写"}</td>
                <td>{point.scale && point.scale !== 1 ? point.scale : "1"} {point.unit || ""}</td>
                <td className="device-value">{formatValue(value)}</td>
                <td><span className={`device-quality ${value?.quality || "uninitialized"}`}>{qualityNames[value?.quality || "uninitialized"] || value?.quality || "无值"}</span></td>
                <td>{timeText(value?.source_time)}</td>
                <td>{mappedPointIDs.has(point.id) ? <span className="ledger-verified">已映射</span> : <span className="ledger-unverified">未映射</span>}</td>
              </tr>
            })}</tbody></table></div>
          {points.length === 0 && <div className="empty-state"><strong>该站号还没有点位</strong><span>在通道的“点表与模板”中新增点位并绑定 Profile。</span></div>}
        </div>}

        {!loading && tab === "commands" && <div className="device-tab-page">
          <div className="device-panel">
            <div className="device-panel-head"><h3>命名命令</h3><span className="device-note">{canOperate ? "执行前会显示目标点位和值；不会自动重试。" : "当前账号只读。"}</span></div>
            {!model?.commands.length && <div className="empty-state"><strong>没有可用命令</strong><span>{profile ? "Profile 没有可写命令，或点位未映射。" : "先绑定 Profile。"}</span></div>}
            <div className="device-command-list">
              {(model?.commands || []).map((command) => {
                const params = commandParams(command)
                const latest = outcomes.find((outcome) => outcome.command_id === command.id)
                return <div className={`device-command ${command.available ? "" : "unavailable"}`} key={command.id}>
                  <div className="device-command-main"><strong>{command.name}</strong><small>{command.point_name}{command.reason ? ` · ${command.reason}` : ""}</small></div>
                  <div className="device-command-params">{(command.parameters || []).map((parameter) => <label key={parameter.name}>{parameter.label || parameter.name}
                    <input type="number" min={parameter.has_range ? parameter.min : undefined} max={parameter.has_range ? parameter.max : undefined} value={params[parameter.name] ?? parameter.default} disabled={!command.available || !canOperate} onChange={(event) => setParamValues((current) => ({ ...current, [command.id]: { ...params, [parameter.name]: Number(event.target.value) } }))} />
                    {parameter.has_range && <small>{parameter.min}–{parameter.max}{parameter.unit || ""}</small>}
                  </label>)}</div>
                  <div className="device-command-actions">
                    {latest && <span className={`device-command-state ${latest.state}`}>{stateNames[latest.state] || latest.state}</span>}
                    {(command.parameters || []).length > 0 && <><button className="secondary-button" onClick={() => savePreset(command)}>存预设</button>{presets[command.id] && <button className="secondary-button" onClick={() => applyPreset(command)}>套预设</button>}</>}
                    <button className="secondary-button" disabled={!command.available} onClick={() => setPreview(command)}>影响预览</button>
                    <button className="primary-button" disabled={!command.available || !canOperate || busy} onClick={() => setPending({ command, params })}>执行</button>
                  </div>
                </div>
              })}
            </div>
          </div>
          <div className="device-panel">
            <div className="device-panel-head"><h3>执行记录</h3><small>本次运行内存中的命令结果</small></div>
            <div className="device-table-wrap"><table className="device-table"><thead><tr><th>时间</th><th>命令</th><th>状态</th><th>原因</th><th>request_id</th></tr></thead><tbody>
              {outcomes.map((outcome) => <tr key={outcome.request_id}><td>{timeText(outcome.updated_at)}</td><td>{model?.commands.find((command) => command.id === outcome.command_id)?.name || outcome.command_id}</td><td><span className={`device-command-state ${outcome.state}`}>{stateNames[outcome.state] || outcome.state}</span></td><td>{outcome.reason || "—"}</td><td><small>{outcome.request_id}</small></td></tr>)}
            </tbody></table>{outcomes.length === 0 && <div className="empty-state"><span>还没有命令记录。</span></div>}</div>
          </div>
        </div>}

        {!loading && tab === "history" && <div className="device-tab-page">
          <div className="device-toolbar">
            <select aria-label="历史点位" value={historyPoint} onChange={(event) => setHistoryPoint(event.target.value)}>{points.map((point) => <option key={point.id} value={point.id}>{point.name || point.id}</option>)}</select>
            <div className="device-range" role="group" aria-label="历史范围">{[1, 24, 168].map((hours) => <button key={hours} className={historyHours === hours ? "active" : ""} onClick={() => setHistoryHours(hours)}>{hours === 1 ? "1 小时" : hours === 24 ? "24 小时" : "7 天"}</button>)}</div>
            <button className="secondary-button" disabled={historyLoading} onClick={() => void loadHistory()}>{historyLoading ? "读取中…" : "刷新"}</button>
          </div>
          {history && history.points.length > 0 ? <>
            <div className="device-history-chart"><TrendChart points={history.points.map((point) => ({ time: point.time, value: point.value })).filter((point): point is { time: string; value: number } => typeof point.value === "number")} /></div>
            <div className="device-history-stats"><span>最新 <b>{formatNumber(history.points[history.points.length - 1]?.value)}</b></span><span>最小 <b>{formatNumber(Math.min(...history.points.map((point) => point.value ?? 0)))}</b></span><span>最大 <b>{formatNumber(Math.max(...history.points.map((point) => point.value ?? 0)))}</b></span><span>样本 <b>{history.points.length}</b></span></div>
            <div className="device-table-wrap"><table className="device-table"><thead><tr><th>时间</th><th>数值</th><th>文本</th><th>来源</th></tr></thead><tbody>{history.points.slice(-8).reverse().map((point, index) => <tr key={`${point.time}-${index}`}><td>{timeText(point.time)}</td><td>{formatNumber(point.value)}</td><td>{point.text || "—"}</td><td><small>{point.origin || "—"}</small></td></tr>)}</tbody></table></div>
          </> : <div className="empty-state"><strong>{historyLoading ? "正在读取历史…" : "该点位在所选范围内没有历史"}</strong><span>历史按变化归档；没有变化时可能只有一条记录。</span></div>}
        </div>}

        {!loading && tab === "alarms" && <div className="device-tab-page">
          <div className="device-panel-head"><h3>设备报警</h3><button className="secondary-button" onClick={goAlarms}>打开报警页</button></div>
          {deviceAlarms.length === 0 && <div className="empty-state"><strong>该设备暂无报警</strong><span>通道离线、设备通信中断和命令失败会在这里出现。</span></div>}
          <div className="device-alarm-list">{deviceAlarms.map((alarm) => <div className={`device-alarm-card ${alarm.severity}`} key={alarm.id}>
            <div><span className={`device-severity ${alarm.severity}`}>{roleNames[alarm.severity] || alarm.severity}</span><strong>{alarm.title}</strong><small>{alarm.detail || "无详细原因"}</small></div>
            <div className="device-alarm-meta"><span>{alarm.state}</span><small>{timeText(alarm.updated_at)}</small></div>
            <div className="device-alarm-actions">{alarm.state === "active" && <button className="secondary-button" disabled={busy || !canOperate} onClick={() => void settleAlarm(alarm, "ack")}>确认</button>}{alarm.state !== "closed" && <button className="secondary-button" disabled={busy || !canOperate} onClick={() => void settleAlarm(alarm, "close")}>关闭</button>}</div>
          </div>)}</div>
        </div>}

        {!loading && tab === "verify" && <div className="device-tab-page device-verify-page">
          <section className="device-panel">
            <div className="device-panel-head"><h3>验证清单</h3><button className="secondary-button" onClick={() => void downloadCSV(token, "/profile-verifications", "fastiot-profile-verifications.csv").catch((cause) => report(cause, "验证记录导出失败"))}>导出验证记录</button></div>
            <div className="device-checklist">
              <Check ok={Boolean(profile)} label="Profile 已绑定" detail={profile ? `${profile.vendor} ${profile.model}` : "未绑定"} />
              <Check ok={Boolean(model?.model) && requiredMissing.length === 0} label="必填点位已映射" detail={requiredMissing.length ? `缺少 ${requiredMissing.join("、")}` : `${mappedCount} 个点位已映射`} />
              <Check ok={commandsReady} label="命令点位可用" detail={commandsReady ? `${model?.commands.length || 0} 条命令可执行` : "有未映射或只读点位"} />
              <Check ok={device.health === "healthy"} label="通信在线" detail={healthNames[device.health] || device.health} />
              <Check ok={Boolean(model?.model?.verified)} label="现场验证已记录" detail={model?.model?.verified ? `${model.model.verifier || "—"} · ${model.model.verified_at || "—"}` : "未记录"} />
            </div>
            {profile?.verification && <p className="device-note">Profile 依据：{profile.verification}</p>}
          </section>
          <section className="device-panel">
            <div className="device-panel-head"><h3>验证记录</h3></div>
            <div className="device-form-grid">
              <label><span>验证人</span><input value={verifier} disabled={!canConfigure} onChange={(event) => setVerifier(event.target.value)} placeholder="现场实施人员" /></label>
              <label><span>验证日期</span><input type="date" value={verifiedAt} disabled={!canConfigure} onChange={(event) => setVerifiedAt(event.target.value)} /></label>
              <label><span>设备序列号</span><input value={deviceSerial} disabled={!canConfigure} onChange={(event) => setDeviceSerial(event.target.value)} placeholder="铭牌序列号" /></label>
              <label className="device-form-check"><input type="checkbox" checked={verified} disabled={!canConfigure} onChange={(event) => setVerified(event.target.checked)} /> 已按 Profile 逐点核对</label>
              <label className="device-form-wide"><span>验证说明</span><textarea value={verification} disabled={!canConfigure} onChange={(event) => setVerification(event.target.value)} placeholder="地址、缩放、字序、写权限、回读结果等" rows={3} /></label>
            </div>
            <div className="device-form-actions"><button className="primary-button" disabled={!canConfigure || busy} onClick={() => void saveVerification()}>{busy ? "保存中…" : "保存验证记录"}</button>{!canConfigure && <span className="device-note">只有管理员或实施人员可以修改验证记录。</span>}</div>
          </section>
        </div>}
      </div>
    </section>

    {preview && <div className="device-confirm-backdrop" onClick={() => setPreview(null)}><section className="device-confirm" role="dialog" aria-label="影响预览" onClick={(event) => event.stopPropagation()}>
      <h3>{preview.name} · 影响预览</h3>
      <p>不会下发任何值。确认目标点位和当前值后再执行。</p>
      <div className="device-target-list">{commandTargets(preview).map((target) => <div key={target.pointID}><strong>{target.name}</strong><span>目标 {formatNumber(target.value)}</span><small>当前 {formatValue(values[target.pointID])}</small></div>)}</div>
      <div className="device-confirm-actions"><button className="secondary-button" onClick={() => setPreview(null)}>关闭</button><button className="primary-button" disabled={!canOperate} onClick={() => { setPending({ command: preview, params: commandParams(preview) }); setPreview(null) }}>去执行</button></div>
    </section></div>}

    {ackTarget && <AckDialog
      alarm={ackTarget}
      busy={busy}
      onCancel={() => setAckTarget(null)}
      onConfirm={(note) => {
        const target = ackTarget
        setAckTarget(null)
        setBusy(true)
        void ackAlarm(token, target.id, note).then(() => loadLive()).catch((cause) => report(cause, "报警确认失败")).finally(() => setBusy(false))
      }}
    />}

    {pending && <EmsOverlay open title={`确认执行「${pending.command.name}」`} busy={busy} onClose={() => setPending(null)}
      actions={<>
        <EmsButton variant="secondary" disabled={busy} onClick={() => setPending(null)}>取消</EmsButton>
        <EmsButton busy={busy} busyLabel="执行中…" onClick={() => void execute(pending.command, pending.params)}>确认执行</EmsButton>
      </>}>
      <p className="m-0 text-sm">{displayName} · {device.channel_name} / 站号 {device.server_id}</p>
      <div className="mt-3 grid gap-2">{commandTargets(pending.command).map((target) => <div key={target.pointID} className="flex items-center justify-between gap-3 rounded-lg border border-ems-line px-3 py-2 text-sm"><strong>{target.name}</strong><span>写入 {formatNumber(target.value)}</span><small className="text-ems-muted">当前 {formatValue(values[target.pointID])}</small></div>)}</div>
      <InlineNotice tone="warning">命令会直接写入设备并等待回读；失败不会自动重试，结果未知时需要现场核对。</InlineNotice>
    </EmsOverlay>}
  </>, document.body)
}

function Check({ ok, label, detail }: { ok: boolean; label: string; detail: string }) {
  return <div className={`device-check ${ok ? "ok" : "pending"}`}><span>{ok ? "✓" : "!"}</span><div><strong>{label}</strong><small>{detail}</small></div></div>
}

function TrendChart({ points }: { points: { time: string; value: number }[] }) {
  if (points.length < 2) return <div className="empty-state"><span>样本不足，无法绘制趋势。</span></div>
  const width = 900
  const height = 220
  const padding = 18
  const values = points.map((point) => point.value)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const coords = points.map((point, index) => {
    const x = padding + (index / (points.length - 1)) * (width - padding * 2)
    const y = height - padding - ((point.value - min) / span) * (height - padding * 2)
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(" ")
  return <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label="设备点位趋势">
    <defs><linearGradient id="device-trend-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="rgba(47,111,159,.35)" /><stop offset="1" stopColor="rgba(47,111,159,0)" /></linearGradient></defs>
    <line x1={padding} y1={height - padding} x2={width - padding} y2={height - padding} stroke="rgba(84,105,129,.24)" />
    <line x1={padding} y1={padding} x2={width - padding} y2={padding} stroke="rgba(84,105,129,.12)" />
    <polyline points={`${padding},${height - padding} ${coords} ${width - padding},${height - padding}`} fill="url(#device-trend-fill)" stroke="none" />
    <polyline points={coords} fill="none" stroke="#2f6f9f" strokeWidth="2.4" strokeLinejoin="round" strokeLinecap="round" />
    <text x={padding} y={14} fill="#7c8d9f" fontSize="11">{formatNumber(max)}</text>
    <text x={padding} y={height - 4} fill="#7c8d9f" fontSize="11">{formatNumber(min)}</text>
  </svg>
}
