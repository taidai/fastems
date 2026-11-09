import { useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { ApiError, downloadCSV, nameOf, Node, point, request } from "./api"

type Run = (action: () => Promise<unknown>) => Promise<boolean>

type PortForm = {
  parent: string
  description: string
  protocol: "TCP" | "RTU"
  uri: string
  port: string
  baud: string
  data_bits: number
  parity: string
  stop_bits: number
  poll_period: number
  timeout: number
  disabled: boolean
}

type UnitModelConfig = {
  name: string
  profile_id: string
  business_node_id?: string
  point_map: Record<string, string>
  verified: boolean
  verifier?: string
  verified_at?: string
  device_serial?: string
  verification?: string
}

type UnitPoint = {
  id: string
  name: string
  server_id: number
  address: number
  io_type: string
  read_only: boolean
  unit?: string
  scale: number
}

type ProfilePointSpec = {
  name: string
  function: string
  address: number
  format: string
  scale: number
  offset: number
  unit: string
  read_only: boolean
  role?: string
  note?: string
}

const pointRoleNames: Record<string, string> = { telemetry: "遥测", status: "遥信", energy: "电度", setpoint: "定值", protection: "压板" }

type ProfileCommandSpec = {
  id: string
  name: string
  point: string
  value: number
  note?: string
}

type UnitProfile = {
  id: string
  vendor: string
  model: string
  protocol: string
  points: number
  commands: number
  verified: boolean
  verification?: string
  custom?: boolean
  point_specs?: ProfilePointSpec[]
  command_specs?: ProfileCommandSpec[]
}

type UnitCommandParameter = {
  name: string
  label: string
  point_id?: string
  point_name: string
  unit?: string
  kind?: string
  has_range?: boolean
  min?: number
  max?: number
  default?: number
  available: boolean
  reason?: string
}

type UnitCommand = {
  id: string
  name: string
  point_id?: string
  point_name: string
  value: number
  parameters?: UnitCommandParameter[]
  available: boolean
  reason?: string
}

type UnitModelData = {
  channel_id: string
  server_id: number
  model?: UnitModelConfig
  profiles: UnitProfile[]
  profile_errors?: string[]
  points: UnitPoint[]
  commands: UnitCommand[]
  missing?: string[]
}

type CommandState = {
  request_id: string
  state: string
  reason?: string
}

type LineTestStation = { server_id: number; responded: boolean; latency_ms: number; probe: string; error: string }
type LineTestResult = { connected: boolean; latency_ms: number; probed: number; stations: LineTestStation[] }
type ScanFound = { server_id: number; responded: boolean; latency_ms: number; probe: string; error: string }
type ScanResult = { scanned: number; found: ScanFound[]; errors: string[] }
type PortTemplate = { name: string; protocol: string; uri: string; port: string; baud: string; data_bits: number; parity: string; stop_bits: number; poll_period: number; timeout: number; disabled: boolean }

type PointForm = {
  parent: string
  description: string
  server_id: number
  address: number
  io_type: string
  data_format: string
  read_only: boolean
  scale: number
  offset: number
  units: string
  disabled: boolean
}

function channelState(node: Node): { key: string; label: string; detail: string } {
  const last = point(node, "lastGoodPoll")?.value || 0
  const poll = point(node, "pollPeriod")?.value || 1000
  const timeout = point(node, "timeout")?.value || 1000
  const errors = point(node, "errorCount")?.value || 0
  const maxAge = Math.max(5, (poll * 5) / 1000, (timeout * 3) / 1000)
  if (!last) return { key: "offline", label: "离线", detail: "尚未成功轮询" }
  const age = Date.now() / 1000 - last
  if (age > maxAge) return { key: "offline", label: "离线", detail: `${Math.round(age)} 秒未成功轮询` }
  if (errors > 0) return { key: "degraded", label: "不稳定", detail: `${errors} 次通信错误` }
  return { key: "online", label: "在线", detail: `${Math.max(1, Math.round(age))} 秒前成功轮询` }
}

function defaultPort(parent: string): PortForm {
  return {
    parent, description: "", protocol: "TCP", uri: "127.0.0.1:1502",
    port: "COM1", baud: "9600", data_bits: 8, parity: "N", stop_bits: 1,
    poll_period: 1000, timeout: 1000, disabled: false,
  }
}

function defaultPoint(parent: string, serverID: number): PointForm {
  return {
    parent, description: "", server_id: serverID, address: 0,
    io_type: "modbusHoldingRegister", data_format: "uint16", read_only: true,
    scale: 1, offset: 0, units: "", disabled: false,
  }
}

const portTemplatesKey = "easyems-port-templates"

function portTemplateOf(form: PortForm): PortTemplate {
  return {
    name: "", protocol: form.protocol, uri: form.uri, port: form.port, baud: form.baud,
    data_bits: form.data_bits, parity: form.parity, stop_bits: form.stop_bits,
    poll_period: form.poll_period, timeout: form.timeout, disabled: form.disabled,
  }
}

function portDiff(original: PortForm, current: PortForm): { label: string; before: string; after: string }[] {
  const diffs: { label: string; before: string; after: string }[] = []
  const changed = (label: string, before: string, after: string) => { if (before !== after) diffs.push({ label, before, after }) }
  changed("端口名称", original.description, current.description)
  changed("协议", original.protocol, current.protocol)
  if (current.protocol === "TCP") {
    changed("目标地址", original.uri, current.uri)
  } else {
    changed("串口", original.port, current.port)
    changed("波特率", original.baud, current.baud)
    changed("数据位", String(original.data_bits), String(current.data_bits))
    changed("校验位", original.parity, current.parity)
    changed("停止位", String(original.stop_bits), String(current.stop_bits))
  }
  changed("轮询周期", `${original.poll_period} ms`, `${current.poll_period} ms`)
  changed("超时", `${original.timeout} ms`, `${current.timeout} ms`)
  changed("停用", original.disabled ? "是" : "否", current.disabled ? "是" : "否")
  return diffs
}

function channelScanParams(channel: Node) {
  const isRTU = (point(channel, "protocol")?.text || "TCP").toUpperCase() === "RTU"
  if (!isRTU) return { protocol: "tcp", address: point(channel, "uri")?.text || "", baud: 9600 }
  const port = point(channel, "port")?.text || ""
  const baudText = point(channel, "baud")?.text || String(point(channel, "baud")?.value || 9600)
  const dataBits = point(channel, "dataBits")?.value || 8
  const parity = point(channel, "parity")?.text || "N"
  const stopBits = point(channel, "stopBits")?.value || 1
  return { protocol: "rtu", address: `${port}@${baudText}@${dataBits}@${parity}@${stopBits}`, baud: Number(baudText) || 9600 }
}

export default function CommunicationView({ token, nodes, roots, groups, canConfigure, canOperate, run, onError, goPoints }: {
  token: string
  nodes: Node[]
  roots: string[]
  groups: Node[]
  canConfigure: boolean
  canOperate: boolean
  run: Run
  onError: (message: string) => void
  goPoints: () => void
}) {
  const channels = useMemo(() => nodes.filter((node) => node.type === "modbus"), [nodes])
  const points = useMemo(() => nodes.filter((node) => node.type === "modbusIo"), [nodes])
  const [selectedID, setSelectedID] = useState("")
  const [tab, setTab] = useState<"basic" | "network" | "serial" | "other">("basic")
  const [query, setQuery] = useState("")
  const [portForm, setPortForm] = useState<PortForm | null>(null)
  const [portEditing, setPortEditing] = useState("")
  const [pointForm, setPointForm] = useState<PointForm | null>(null)
  const [test, setTest] = useState({ server_id: 1, address: 0, io_type: "modbusHoldingRegister" })
  const [testResult, setTestResult] = useState("")
  const [busy, setBusy] = useState(false)
  const [modelUnit, setModelUnit] = useState<number | null>(null)
  const [modelData, setModelData] = useState<UnitModelData | null>(null)
  const [modelDraft, setModelDraft] = useState<UnitModelConfig | null>(null)
  const [modelTab, setModelTab] = useState<"model" | "commands">("model")
  const [modelBusy, setModelBusy] = useState(false)
  const [modelNotice, setModelNotice] = useState("")
  const [commandResults, setCommandResults] = useState<Record<string, CommandState>>({})
  const [commandValues, setCommandValues] = useState<Record<string, Record<string, number>>>({})
  const [portOriginal, setPortOriginal] = useState<PortForm | null>(null)
  const [portTemplates, setPortTemplates] = useState<PortTemplate[]>(() => {
    try { return JSON.parse(localStorage.getItem(portTemplatesKey) || "[]") as PortTemplate[] } catch { return [] }
  })
  const [portTemplateName, setPortTemplateName] = useState("")
  const [lineTest, setLineTest] = useState<LineTestResult | null>(null)
  const [lineTestBusy, setLineTestBusy] = useState(false)
  const [lineTestError, setLineTestError] = useState("")
  const [scanFrom, setScanFrom] = useState(1)
  const [scanTo, setScanTo] = useState(16)
  const [scanResult, setScanResult] = useState<ScanResult | null>(null)
  const [scanBusy, setScanBusy] = useState(false)
  const [scanError, setScanError] = useState("")

  useEffect(() => {
    if (!selectedID && channels[0]) setSelectedID(channels[0].id)
    if (selectedID && !channels.some((channel) => channel.id === selectedID)) setSelectedID(channels[0]?.id || "")
  }, [channels, selectedID])

  useEffect(() => {
    setLineTest(null)
    setLineTestError("")
    setScanResult(null)
    setScanError("")
  }, [selectedID])

  const selected = channels.find((channel) => channel.id === selectedID) || channels[0]
  const channelPoints = selected ? points.filter((node) => node.parent === selected.id) : []
  const units = useMemo(() => {
    const map = new Map<number, Node[]>()
    for (const node of channelPoints) {
      const server = point(node, "id")?.value || 1
      map.set(server, [...(map.get(server) || []), node])
    }
    return [...map.entries()].sort((a, b) => a[0] - b[0])
  }, [channelPoints])
  const state = selected ? channelState(selected) : null
  const filteredChannels = channels.filter((channel) => {
    if (!query.trim()) return true
    const text = `${nameOf(channel)} ${point(channel, "uri")?.text || ""} ${point(channel, "port")?.text || ""}`.toLowerCase()
    return text.includes(query.trim().toLowerCase())
  })

  function openAddPort() {
    setPortEditing("")
    setPortForm(defaultPort(roots[0] || groups[0]?.id || ""))
    setPortOriginal(null)
    setTest({ server_id: 1, address: 0, io_type: "modbusHoldingRegister" })
    setTestResult("")
  }

  function openEditPort() {
    if (!selected) return
    setPortEditing(selected.id)
    const form: PortForm = {
      parent: selected.parent,
      description: nameOf(selected),
      protocol: (point(selected, "protocol")?.text || "TCP").toUpperCase() === "RTU" ? "RTU" : "TCP",
      uri: point(selected, "uri")?.text || "127.0.0.1:1502",
      port: point(selected, "port")?.text || "COM1",
      baud: point(selected, "baud")?.text || String(point(selected, "baud")?.value || 9600),
      data_bits: point(selected, "dataBits")?.value || 8,
      parity: point(selected, "parity")?.text || "N",
      stop_bits: point(selected, "stopBits")?.value || 1,
      poll_period: point(selected, "pollPeriod")?.value || 1000,
      timeout: point(selected, "timeout")?.value || 1000,
      disabled: (point(selected, "disabled")?.value || 0) !== 0,
    }
    setPortForm(form)
    setPortOriginal({ ...form })
    setTest({ server_id: 1, address: 0, io_type: "modbusHoldingRegister" })
    setTestResult("")
  }

  async function savePort() {
    if (!portForm) return
    setBusy(true)
    const ok = await run(async () => {
      const body = {
        ...portForm,
        baud: String(portForm.baud),
        poll_period: Number(portForm.poll_period) || 1000,
        timeout: Number(portForm.timeout) || 1000,
      }
      if (portEditing) await request(token, `/modbus/${portEditing}`, "PUT", body)
      else await request(token, "/modbus", "POST", body)
    })
    setBusy(false)
    if (ok) setPortForm(null)
  }

  async function deletePort() {
    if (!selected || !window.confirm(`确定删除端口“${nameOf(selected)}”及其下所有点位吗？`)) return
    setBusy(true)
    const ok = await run(async () => { await request(token, `/ems/channels/${selected.id}`, "DELETE") })
    setBusy(false)
    if (ok) setSelectedID("")
  }

  function openAddPoint(serverID = 1) {
    if (!selected) return
    setPointForm(defaultPoint(selected.id, serverID))
  }

  async function savePoint() {
    if (!pointForm) return
    if (!pointForm.description.trim()) { onError("点位名称不能为空"); return }
    setBusy(true)
    const ok = await run(async () => { await request(token, `/modbus/${pointForm.parent}/io`, "POST", pointForm) })
    setBusy(false)
    if (ok) setPointForm(null)
  }

  async function openUnitModel(server: number) {
    if (!selected) return
    setModelUnit(server)
    setModelData(null)
    setModelDraft(null)
    setModelTab("model")
    setModelNotice("")
    setCommandResults({})
    try {
      const data = await request<UnitModelData>(token, `/modbus/${selected.id}/units/${server}/model`)
      setModelData(data)
      initializeCommandValues(data)
      setModelDraft(data.model
        ? { ...data.model, point_map: { ...data.model.point_map } }
        : { name: `站号 ${server}`, profile_id: "", point_map: {}, verified: false, verifier: "", verified_at: "", device_serial: "", verification: "" })
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "设备模型读取失败")
    }
  }

  function selectProfile(profileID: string) {
    if (!modelData || !modelDraft || modelUnit === null) return
    const profile = modelData.profiles.find((item) => item.id === profileID)
    const pointMap: Record<string, string> = {}
    for (const spec of profile?.point_specs || []) {
      const match = modelData.points.find((item) => item.server_id === modelUnit && item.name === spec.name)
      if (match) pointMap[spec.name] = match.id
    }
    setModelDraft({ ...modelDraft, profile_id: profileID, point_map: pointMap })
    setModelNotice(profile ? `已按名称自动匹配 ${Object.keys(pointMap).length} 个点位` : "")
  }

  function mapPoint(name: string, pointID: string) {
    if (!modelDraft) return
    const pointMap = { ...modelDraft.point_map }
    if (pointID) pointMap[name] = pointID
    else delete pointMap[name]
    setModelDraft({ ...modelDraft, point_map: pointMap })
  }

  async function saveUnitModel() {
    if (!modelData || !modelDraft || modelUnit === null) return
    setModelBusy(true)
    try {
      const data = await request<UnitModelData>(token, `/modbus/${modelData.channel_id}/units/${modelUnit}/model`, "PUT", modelDraft)
      setModelData(data)
      initializeCommandValues(data)
      if (data.model) setModelDraft({ ...data.model, point_map: { ...data.model.point_map } })
      setModelNotice("设备模型已保存")
      setModelTab("commands")
    } catch (cause) {
      setModelNotice(cause instanceof Error ? cause.message : "保存失败")
    } finally {
      setModelBusy(false)
    }
  }

  function initializeCommandValues(data: UnitModelData) {
    const values: Record<string, Record<string, number>> = {}
    for (const command of data.commands) {
      if (!command.parameters?.length) continue
      values[command.id] = Object.fromEntries(command.parameters.map((parameter) => [parameter.name, parameter.default || 0]))
    }
    setCommandValues(values)
  }

  function setCommandValue(commandID: string, parameter: string, value: number) {
    setCommandValues((current) => ({ ...current, [commandID]: { ...(current[commandID] || {}), [parameter]: value } }))
  }

  async function executeCommand(command: UnitCommand) {
    if (!modelData || modelUnit === null || !canOperate) return
    if (!window.confirm(`执行命令“${command.name}”？

目标：${command.point_name} = ${command.value}
执行后会等待设备回读。`)) return
    const requestID = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
    setCommandResults((prev) => ({ ...prev, [command.id]: { request_id: requestID, state: "accepted" } }))
    try {
      let outcome = await request<CommandState>(token, `/modbus/${modelData.channel_id}/units/${modelUnit}/commands`, "POST", {
        request_id: requestID,
        command_id: command.id,
        parameters: commandValues[command.id] || {},
      })
      setCommandResults((prev) => ({ ...prev, [command.id]: outcome }))
      const deadline = Date.now() + 15000
      while (!["confirmed", "failed", "unknown"].includes(outcome.state) && Date.now() < deadline) {
        await new Promise((resolve) => window.setTimeout(resolve, 700))
        outcome = await request<CommandState>(token, `/modbus/${modelData.channel_id}/units/${modelUnit}/commands/${encodeURIComponent(requestID)}`)
        setCommandResults((prev) => ({ ...prev, [command.id]: outcome }))
      }
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "命令执行失败"
      onError(message)
      setCommandResults((prev) => ({ ...prev, [command.id]: { request_id: requestID, state: "failed", reason: message } }))
    }
  }

  function commandStateLabel(state: string) {
    return ({ accepted: "已接受", executing: "执行中", confirmed: "已回读", failed: "失败", unknown: "结果未知" } as Record<string, string>)[state] || state
  }

  async function runLineTest() {
    if (!selected) return
    setLineTestBusy(true)
    setLineTestError("")
    setLineTest(null)
    try {
      const result = await request<LineTestResult>(token, `/modbus/${encodeURIComponent(selected.id)}/line-test`, "POST", { from: scanFrom, to: scanTo })
      setLineTest(result)
    } catch (cause) {
      setLineTestError(cause instanceof Error ? cause.message : "线路测试失败")
    } finally {
      setLineTestBusy(false)
    }
  }

  async function runScanInline() {
    if (!selected) return
    setScanBusy(true)
    setScanError("")
    setScanResult(null)
    try {
      const params = channelScanParams(selected)
      const result = await request<ScanResult>(token, "/ems/scan", "POST", {
        protocol: params.protocol, address: params.address, baud: params.baud,
        from: scanFrom, to: scanTo, timeout_ms: 300,
      })
      setScanResult(result)
    } catch (cause) {
      if (cause instanceof ApiError && cause.message === "channel_busy") {
        setScanError("该通道正在运行，无法扫描；下方单元列表即为已轮询站号。")
      } else {
        setScanError(cause instanceof Error ? cause.message : "扫描失败")
      }
    } finally {
      setScanBusy(false)
    }
  }

  function savePortTemplate() {
    if (!portForm || !portTemplateName.trim()) {
      onError("模板名称不能为空")
      return
    }
    const template: PortTemplate = { ...portTemplateOf(portForm), name: portTemplateName.trim() }
    const next = [...portTemplates.filter((item) => item.name !== template.name), template]
    setPortTemplates(next)
    localStorage.setItem(portTemplatesKey, JSON.stringify(next))
    setPortTemplateName("")
  }

  function applyPortTemplate(name: string) {
    const template = portTemplates.find((item) => item.name === name)
    if (!template || !portForm) return
    setPortForm({
      ...portForm, protocol: template.protocol as "TCP" | "RTU", uri: template.uri, port: template.port,
      baud: template.baud, data_bits: template.data_bits, parity: template.parity, stop_bits: template.stop_bits,
      poll_period: template.poll_period, timeout: template.timeout, disabled: template.disabled,
    })
  }

  const modelProfile = modelData?.profiles.find((item) => item.id === modelDraft?.profile_id)
  const modelPoints = modelData?.points.filter((item) => item.server_id === modelUnit) || []
  const modelRequiredPoints = new Set((modelProfile?.command_specs || []).map((command) => command.point))

  return <section className="communication-page">
    <aside className="panel communication-tree">
      <div className="communication-tree-head"><strong>站点结构</strong><span>{channels.length} 端口</span></div>
      <input aria-label="搜索端口" placeholder="搜索端口 / 地址" value={query} onChange={(event) => setQuery(event.target.value)} />
      <div className="communication-tree-body">
        {filteredChannels.map((channel) => <div key={channel.id} className="tree-branch">
          <button className={`tree-port ${selected?.id === channel.id ? "active" : ""}`} onClick={() => setSelectedID(channel.id)}>
            <span className={`channel-state ${channelState(channel).key}`}><i /></span>{nameOf(channel)}
          </button>
          {selected?.id === channel.id && <div className="tree-units">
            {units.map(([server, list]) => <button key={server} className="tree-unit" onClick={() => setTab("basic")}>站号 {server}<small>{list.length} 点位</small></button>)}
            {units.length === 0 && <span className="tree-empty">暂无单元</span>}
          </div>}
        </div>)}
        {channels.length === 0 && <div className="empty-state"><span>还没有通信端口</span></div>}
      </div>
    </aside>

    <div className="communication-main">
      <div className="panel communication-ports">
        <div className="communication-ports-head">
          <div><span className="section-kicker">端口与目标</span><h2>端口列表</h2></div>
          {canConfigure && <div className="communication-actions"><button className="primary-button" onClick={openAddPort}>+ 新增端口</button></div>}
        </div>
        <div className="communication-table-wrap">
          <table className="communication-table">
            <thead><tr><th>端口名称</th><th>类型</th><th>目标地址</th><th>状态</th><th>点位</th><th>操作</th></tr></thead>
            <tbody>
              {channels.map((channel) => <tr key={channel.id} className={selected?.id === channel.id ? "selected" : ""}>
                <td><strong>{nameOf(channel)}</strong></td>
                <td>{point(channel, "protocol")?.text || "TCP"}</td>
                <td>{point(channel, "uri")?.text || `${point(channel, "port")?.text || "—"} @ ${point(channel, "baud")?.text || point(channel, "baud")?.value || "—"}`}</td>
                <td><span className={`channel-state ${channelState(channel).key}`}><i />{channelState(channel).label}</span></td>
                <td>{points.filter((node) => node.parent === channel.id).length}</td>
                <td className="communication-row-actions"><button onClick={() => { setSelectedID(channel.id); setTab("basic") }}>查看</button></td>
              </tr>)}
              {channels.length === 0 && <tr><td colSpan={6}><div className="empty-state"><span>还没有端口，先新增一个 Modbus TCP/RTU 端口。</span></div></td></tr>}
            </tbody>
          </table>
        </div>
      </div>

      {selected && <div className="panel communication-detail">
        <div className="communication-detail-head">
          <div><span className="section-kicker">端口详情</span><h2>{nameOf(selected)}</h2></div>
          <div>{canConfigure && <><button className="secondary-button" onClick={openEditPort}>编辑</button><button className="secondary-button danger-text" disabled={busy} onClick={() => void deletePort()}>删除</button></>}</div>
        </div>
        <div className="communication-tabs" role="group" aria-label="端口详情">
          {([["basic", "基本信息"], ["network", "网络配置"], ["serial", "串口配置"], ["other", "其他配置"]] as const).map(([key, label]) => <button key={key} className={tab === key ? "active" : ""} onClick={() => setTab(key)}>{label}</button>)}
        </div>
        {tab === "basic" && <dl className="communication-fields">
          <div><dt>端口 ID</dt><dd>{selected.id}</dd></div>
          <div><dt>端口名称</dt><dd>{nameOf(selected)}</dd></div>
          <div><dt>端口类型</dt><dd>{point(selected, "protocol")?.text || "TCP"}</dd></div>
          <div><dt>端口状态</dt><dd>{state?.label} · {state?.detail}</dd></div>
          <div><dt>所属站点</dt><dd>{groups.find((group) => group.id === selected.parent) ? nameOf(groups.find((group) => group.id === selected.parent)!) : selected.parent}</dd></div>
          <div><dt>单元数 / 点位数</dt><dd>{units.length} / {channelPoints.length}</dd></div>
        </dl>}
        {tab === "network" && <dl className="communication-fields">
          <div><dt>协议</dt><dd>Modbus {point(selected, "protocol")?.text || "TCP"}</dd></div>
          <div><dt>{point(selected, "protocol")?.text === "RTU" ? "串口" : "目标地址"}</dt><dd>{point(selected, "uri")?.text || point(selected, "port")?.text || "—"}</dd></div>
          <div><dt>轮询周期</dt><dd>{point(selected, "pollPeriod")?.value || 1000} ms</dd></div>
          <div><dt>超时</dt><dd>{point(selected, "timeout")?.value || 1000} ms</dd></div>
        </dl>}
        {tab === "serial" && <dl className="communication-fields">
          <div><dt>串口</dt><dd>{point(selected, "port")?.text || "—"}</dd></div>
          <div><dt>波特率</dt><dd>{point(selected, "baud")?.text || point(selected, "baud")?.value || "—"}</dd></div>
          <div><dt>数据位 / 校验 / 停止位</dt><dd>{point(selected, "dataBits")?.value || 8} / {point(selected, "parity")?.text || "N"} / {point(selected, "stopBits")?.value || 1}</dd></div>
          <div><dt>说明</dt><dd>RTU 参数按端口配置建立；修改后保存并重启通道生效。</dd></div>
        </dl>}
        {tab === "other" && <dl className="communication-fields">
          <div><dt>启用状态</dt><dd>{(point(selected, "disabled")?.value || 0) !== 0 ? "已停用" : "启用"}</dd></div>
          <div><dt>最近成功轮询</dt><dd>{point(selected, "lastGoodPoll")?.value ? new Date((point(selected, "lastGoodPoll")?.value || 0) * 1000).toLocaleString("zh-CN", { hour12: false }) : "—"}</dd></div>
          <div><dt>通信错误</dt><dd>{point(selected, "errorCount")?.value || 0} 次</dd></div>
        </dl>}

        <div className="communication-units-head"><strong>单元列表</strong>{canConfigure && <button className="secondary-button" onClick={() => openAddPoint(units[0]?.[0] || 1)}>+ 新增单元/点位</button>}</div>
        <div className="communication-table-wrap units">
          <table className="communication-table">
            <thead><tr><th>单元 ID</th><th>单元名称</th><th>设备地址</th><th>协议名称</th><th>点位数</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {units.map(([server, list]) => <tr key={server}>
                <td>{server}</td>
                <td>站号 {server}</td>
                <td>{server}</td>
                <td>Modbus</td>
                <td>{list.length}</td>
                <td>{state?.label || "—"}</td>
                <td className="communication-row-actions"><button onClick={() => void openUnitModel(server)}>{canConfigure ? "设备模型" : "命令"}</button><button onClick={goPoints}>查看点位</button></td>
              </tr>)}
              {units.length === 0 && <tr><td colSpan={7}><div className="empty-state"><span>该端口还没有点位；新增一个点位后会自动形成单元。</span></div></td></tr>}
            </tbody>
          </table>
        </div>

        {canConfigure && <div className="comm-test">
          <div className="form-section-title"><strong>通道线路测试与扫描</strong><span>只读探测连通性与站号可读性，不写入设备、不改配置；1–247，最多 64 个站号。</span></div>
          <div className="channel-tool-row">
            <label>起始站号<input type="number" min={1} max={247} value={scanFrom} onChange={(event) => setScanFrom(Number(event.target.value))} /></label>
            <label>结束站号<input type="number" min={1} max={247} value={scanTo} onChange={(event) => setScanTo(Number(event.target.value))} /></label>
            <div className="channel-tool-actions">
              <button className="secondary-button" disabled={lineTestBusy || scanBusy} title="用当前端口持久化参数，验证是否连通、站号是否可读" onClick={() => void runLineTest()}>{lineTestBusy ? "测试中…" : "线路测试"}</button>
              <button className="secondary-button" disabled={scanBusy || lineTestBusy} title="临时打开总线，搜索当前范围内有哪些站号响应" onClick={() => void runScanInline()}>{scanBusy ? "扫描中…" : "扫描站号"}</button>
            </div>
          </div>
          {lineTestError && <div className="inline-error" role="alert">{lineTestError}</div>}
          {lineTest && <div className="channel-tool-result">
            <span className={`unit-map-state ${lineTest.connected ? "ok" : "warn"}`}>{lineTest.connected ? `已连接 · ${lineTest.stations.filter((s) => s.responded).length}/${lineTest.probed} 站号响应` : "连接失败"}</span>
            <div className="channel-tool-list">
              {lineTest.stations.map((s) => <span key={s.server_id} className={`unit-map-state ${s.responded ? "ok" : ""}`}>站号 {s.server_id}：{s.responded ? `${s.latency_ms} ms · ${s.probe}` : (s.error || "无响应")}</span>)}
            </div>
          </div>}
          {scanError && <div className="inline-error" role="alert">{scanError}</div>}
          {scanResult && <div className="channel-tool-result">
            <span className="unit-map-state ok">扫描 {scanResult.scanned} 个站号，{scanResult.found.filter((s) => s.responded).length} 个响应</span>
            <div className="channel-tool-list">
              {scanResult.found.map((s) => <span key={s.server_id} className={`unit-map-state ${s.responded ? "ok" : ""}`}>站号 {s.server_id}：{s.responded ? `${s.latency_ms} ms · ${s.probe}` : "无响应"}</span>)}
            </div>
          </div>}
        </div>}
      </div>}

      {modelUnit !== null && createPortal(<>
        <div className="comm-modal-backdrop" onClick={() => setModelUnit(null)} />
        <section className="comm-modal unit-model-modal" role="dialog" aria-label="单元设备模型">
          <div className="comm-modal-head">
            <div><span className="section-kicker">Modbus 单元</span><h2>站号 {modelUnit} · 设备模型</h2></div>
            <button className="icon-button" aria-label="关闭" onClick={() => setModelUnit(null)}>×</button>
          </div>
          <div className="unit-model-tabs" role="group" aria-label="设备模型">
            <button className={modelTab === "model" ? "active" : ""} onClick={() => setModelTab("model")}>模型与点位</button>
            <button className={modelTab === "commands" ? "active" : ""} onClick={() => setModelTab("commands")}>命令执行</button>
          </div>

          {!modelData || !modelDraft ? <div className="empty-state"><span>正在读取单元配置…</span></div> : <>
            {modelTab === "model" && <>
              <div className="comm-form-grid">
                <label>设备名称<input disabled={!canConfigure} value={modelDraft.name} onChange={(event) => setModelDraft({ ...modelDraft, name: event.target.value })} /></label>
                <label>设备 Profile<select disabled={!canConfigure} value={modelDraft.profile_id} onChange={(event) => selectProfile(event.target.value)}>
                  <option value="">选择 Profile</option>
                  {modelData.profiles.filter((profile) => profile.points > 0).map((profile) => <option key={profile.id} value={profile.id}>{profile.vendor} {profile.model}{profile.custom ? "（自定义）" : ""}{profile.verified ? "" : "（未验证）"}</option>)}
                </select></label>
                <label>业务节点（可选）<select disabled={!canConfigure} value={modelDraft.business_node_id || ""} onChange={(event) => setModelDraft({ ...modelDraft, business_node_id: event.target.value })}><option value="">不绑定业务节点</option>{groups.map((group) => <option key={group.id} value={group.id}>{roots.includes(group.id) ? "根节点" : nameOf(group)}</option>)}</select></label>
                <label>验证人<input disabled={!canConfigure} value={modelDraft.verifier || ""} onChange={(event) => setModelDraft({ ...modelDraft, verifier: event.target.value })} placeholder="现场核对人员" /></label>
                <label>验证日期<input disabled={!canConfigure} type="date" value={modelDraft.verified_at || ""} onChange={(event) => setModelDraft({ ...modelDraft, verified_at: event.target.value })} /></label>
                <label>设备序列号<input disabled={!canConfigure} value={modelDraft.device_serial || ""} onChange={(event) => setModelDraft({ ...modelDraft, device_serial: event.target.value })} placeholder="铭牌/设备编码" /></label>
                <label>验证说明<input disabled={!canConfigure} value={modelDraft.verification || ""} onChange={(event) => setModelDraft({ ...modelDraft, verification: event.target.value })} placeholder="地址/缩放/字序/写入核对结论" /></label>
                <label className="comm-check"><input type="checkbox" disabled={!canConfigure} checked={modelDraft.verified} onChange={(event) => setModelDraft({ ...modelDraft, verified: event.target.checked })} /> 已完成现场验证</label>
              </div>

              {(modelData.profile_errors || []).length > 0 && <div className="inline-note" role="alert">自定义 Profile 文件有 {modelData.profile_errors!.length} 个未加载：{modelData.profile_errors!.join("；")}</div>}

              {!modelProfile ? <div className="empty-state"><span>先选择设备 Profile，再按名称自动匹配点位。</span></div> : <>
                <div className="unit-map-head">
                  <div><strong>点位映射</strong><span>{Object.keys(modelDraft.point_map).length} / {modelProfile.point_specs?.length || 0} 已映射</span></div>
                  <span className="inline-note">可写命令点位必须映射；未映射的遥测不会影响命令。</span>
                </div>
                <div className="unit-map-wrap">
                  <table className="unit-map-table">
                    <thead><tr><th>Profile 变量</th><th>角色</th><th>地址</th><th>单位</th><th>读写</th><th>映射到点位</th><th>状态</th></tr></thead>
                    <tbody>
                      {(modelProfile.point_specs || []).map((spec) => {
                        const mapped = modelDraft.point_map[spec.name] || ""
                        const required = modelRequiredPoints.has(spec.name)
                        return <tr key={spec.name}>
                          <td><strong>{spec.name}</strong>{spec.note && <small>{spec.note}</small>}</td>
                          <td>{spec.role ? <span className={`point-role ${spec.role}`}>{pointRoleNames[spec.role] || spec.role}</span> : "—"}</td>
                          <td>{spec.address}</td>
                          <td>{spec.unit || "—"}</td>
                          <td>{spec.read_only ? "只读" : "可写"}</td>
                          <td><select disabled={!canConfigure} value={mapped} onChange={(event) => mapPoint(spec.name, event.target.value)}>
                            <option value="">未映射</option>
                            {modelPoints.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.address}</option>)}
                          </select></td>
                          <td><span className={`unit-map-state ${mapped ? "ok" : required ? "warn" : ""}`}>{mapped ? "已映射" : required ? "需映射" : "可选"}</span></td>
                        </tr>
                      })}
                    </tbody>
                  </table>
                </div>
                {(modelData.missing?.length || 0) > 0 && <div className="unit-model-warning">还有 {modelData.missing?.length} 个 Profile 变量未映射；若它们在命令中使用，命令会显示为不可用。</div>}
              </>}

              {modelNotice && <div className="success-message" role="status">{modelNotice}</div>}
              <div className="comm-modal-actions">
                <button className="secondary-button" onClick={() => setModelUnit(null)}>取消</button>
                {canConfigure && <button className="secondary-button" onClick={() => void downloadCSV(token, "/profile-verifications?format=csv", "easyems-profile-verifications.csv").catch((cause) => onError(cause instanceof Error ? cause.message : "导出失败"))}>导出验证记录</button>}
                {canConfigure && <button className="primary-button" disabled={modelBusy || !modelDraft.profile_id} onClick={() => void saveUnitModel()}>{modelBusy ? "保存中…" : "保存设备模型"}</button>}
              </div>
            </>}

            {modelTab === "commands" && <>
              {!modelData.model ? <div className="empty-state"><span>先保存设备模型，命令才会出现。</span></div> : <>
                <p className="comm-modal-note">命令只写已映射的可写点位；执行后等待设备回读，超时会标记“结果未知”，不会自动重试。</p>
                <div className="unit-command-list">
                  {modelData.commands.map((command) => {
                    const result = commandResults[command.id]
                    return <div className={`unit-command-card ${command.available ? "" : "disabled"}`} key={command.id}>
                      <div className="unit-command-main"><strong>{command.name}</strong><span>{command.point_name ? `${command.point_name} · 写 ${command.value}` : `${command.parameters?.length || 0} 个参数`}</span></div>
                      {command.available ? <button className="primary-button" disabled={!canOperate || modelBusy} onClick={() => void executeCommand(command)}>执行</button> : <span className="unit-map-state warn">{command.reason || "不可用"}</span>}
                      {command.parameters && command.parameters.length > 0 && <div className="unit-command-params">{command.parameters.map((parameter) => <label key={parameter.name}>{parameter.label}<input type="number" step="any" min={parameter.has_range ? parameter.min : undefined} max={parameter.has_range ? parameter.max : undefined} disabled={!canOperate || !parameter.available} value={commandValues[command.id]?.[parameter.name] ?? parameter.default ?? 0} onChange={(event) => setCommandValue(command.id, parameter.name, Number(event.target.value))} /><span>{parameter.unit || ""}</span>{parameter.reason && <small>{parameter.reason}</small>}</label>)}</div>}
                      {result && <div className={`command-result ${result.state}`}><span>{commandStateLabel(result.state)}</span>{result.reason && <small>{result.reason}</small>}<code>{result.request_id}</code></div>}
                    </div>
                  })}
                  {modelData.commands.length === 0 && <div className="empty-state"><span>该 Profile 没有可执行命令。</span></div>}
                </div>
              </>}
              <div className="comm-modal-actions"><button className="secondary-button" onClick={() => setModelUnit(null)}>关闭</button></div>
            </>}
          </>}
        </section>
      </>, document.body)}

      {portForm && createPortal(<>
        <div className="comm-modal-backdrop" onClick={() => setPortForm(null)} />
        <section className="comm-modal" role="dialog" aria-label="端口配置">
          <div className="comm-modal-head"><div><span className="section-kicker">Modbus 端口</span><h2>{portEditing ? "编辑端口" : "新增端口"}</h2></div><button className="icon-button" aria-label="关闭" onClick={() => setPortForm(null)}>×</button></div>
          <div className="comm-form-grid">
            <label>所属节点<select value={portForm.parent} onChange={(event) => setPortForm({ ...portForm, parent: event.target.value })}>{groups.map((group) => <option key={group.id} value={group.id}>{roots.includes(group.id) ? "整个设备树" : nameOf(group)}</option>)}</select></label>
            <label>端口名称<input value={portForm.description} onChange={(event) => setPortForm({ ...portForm, description: event.target.value })} placeholder="如 1# 配电室 TCP" /></label>
            <label>协议<select value={portForm.protocol} onChange={(event) => setPortForm({ ...portForm, protocol: event.target.value as "TCP" | "RTU" })}><option value="TCP">Modbus TCP</option><option value="RTU">Modbus RTU</option></select></label>
            {portForm.protocol === "TCP"
              ? <label>目标地址<input value={portForm.uri} onChange={(event) => setPortForm({ ...portForm, uri: event.target.value })} placeholder="192.168.1.10:502" /></label>
              : <><label>串口<input value={portForm.port} onChange={(event) => setPortForm({ ...portForm, port: event.target.value })} placeholder="COM3" /></label><label>波特率<input value={portForm.baud} onChange={(event) => setPortForm({ ...portForm, baud: event.target.value })} /></label><label>数据位<select value={portForm.data_bits} onChange={(event) => setPortForm({ ...portForm, data_bits: Number(event.target.value) })}>{[5,6,7,8].map((value) => <option key={value} value={value}>{value}</option>)}</select></label><label>校验位<select value={portForm.parity} onChange={(event) => setPortForm({ ...portForm, parity: event.target.value })}><option value="N">无 (N)</option><option value="E">偶 (E)</option><option value="O">奇 (O)</option></select></label><label>停止位<select value={portForm.stop_bits} onChange={(event) => setPortForm({ ...portForm, stop_bits: Number(event.target.value) })}><option value={1}>1</option><option value={2}>2</option></select></label></>}
            <label>轮询周期（ms）<input type="number" min={200} value={portForm.poll_period} onChange={(event) => setPortForm({ ...portForm, poll_period: Number(event.target.value) })} /></label>
            <label>超时（ms）<input type="number" min={100} value={portForm.timeout} onChange={(event) => setPortForm({ ...portForm, timeout: Number(event.target.value) })} /></label>
            <label className="comm-check"><input type="checkbox" checked={portForm.disabled} onChange={(event) => setPortForm({ ...portForm, disabled: event.target.checked })} /> 停用端口</label>
          </div>
          <div className="comm-test">
            <div className="form-section-title"><strong>只读连接测试</strong><span>保存前用当前参数读取一次，不写入设备。</span></div>
            <div className="modbus-test-row">
              <label>站号<input type="number" min={1} max={247} value={test.server_id} onChange={(event) => { setTest({ ...test, server_id: Number(event.target.value) }); setTestResult("") }} /></label>
              <label>地址<input type="number" min={0} max={65535} value={test.address} onChange={(event) => { setTest({ ...test, address: Number(event.target.value) }); setTestResult("") }} /></label>
              <label>只读类型<select value={test.io_type} onChange={(event) => { setTest({ ...test, io_type: event.target.value }); setTestResult("") }}><option value="modbusHoldingRegister">保持寄存器</option><option value="modbusInputRegister">输入寄存器</option><option value="modbusCoil">线圈</option><option value="modbusDiscreteInput">离散输入</option></select></label>
              <button type="button" className="secondary-button" disabled={busy} onClick={() => void run(async () => {
                const result = await request<{ value: unknown }>(token, "/modbus/test", "POST", {
                  ...portForm, baud: String(portForm.baud), poll_period: Number(portForm.poll_period) || 1000, timeout: Number(portForm.timeout) || 1000, ...test,
                })
                setTestResult(`读取成功：${JSON.stringify(result.value)}`)
              })}>测试只读连接</button>
            </div>
            {testResult && <div className="success-message" role="status">{testResult}</div>}
          </div>
          {portOriginal && (() => {
            const diffs = portDiff(portOriginal, portForm)
            return diffs.length > 0 && <div className="comm-test port-diff">
              <div className="form-section-title"><strong>参数变更</strong><span className="port-diff-warning">保存前核对前值 → 新值</span></div>
              <dl className="communication-fields">
                {diffs.map((diff) => <div key={diff.label}><dt>{diff.label}</dt><dd>{diff.before} → {diff.after}</dd></div>)}
              </dl>
            </div>
          })()}
          <div className="comm-test">
            <div className="form-section-title"><strong>端口配置模板</strong><span>本机保存，跨端口套用参数</span></div>
            <div className="modbus-tool-fields">
              <label>模板名称<input value={portTemplateName} onChange={(event) => setPortTemplateName(event.target.value)} placeholder="例如：标准 9600 RTU" /></label>
              {portTemplates.length > 0 && <label>套用模板<select value="" onChange={(event) => { if (event.target.value) applyPortTemplate(event.target.value) }}>
                <option value="">选择模板…</option>
                {portTemplates.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
              </select></label>}
            </div>
            <div><button className="secondary-button" disabled={busy || !portTemplateName.trim()} onClick={savePortTemplate}>保存为模板</button></div>
          </div>
          <div className="comm-modal-actions"><button className="secondary-button" onClick={() => setPortForm(null)}>取消</button><button className="primary-button" disabled={busy || !portForm.description.trim()} onClick={() => void savePort()}>{busy ? "保存中…" : "保存端口"}</button></div>
        </section>
      </>, document.body)}

      {pointForm && createPortal(<>
        <div className="comm-modal-backdrop" onClick={() => setPointForm(null)} />
        <section className="comm-modal" role="dialog" aria-label="新增单元点位">
          <div className="comm-modal-head"><div><span className="section-kicker">单元与点位</span><h2>新增单元/点位</h2></div><button className="icon-button" aria-label="关闭" onClick={() => setPointForm(null)}>×</button></div>
          <p className="comm-modal-note">当前模型以点位承载单元：同一站号的第一个点位创建后，单元会出现在单元列表。</p>
          <div className="comm-form-grid">
            <label>点位名称<input value={pointForm.description} onChange={(event) => setPointForm({ ...pointForm, description: event.target.value })} placeholder="如 1#箱变有功功率" /></label>
            <label>站号<input type="number" min={1} max={247} value={pointForm.server_id} onChange={(event) => setPointForm({ ...pointForm, server_id: Number(event.target.value) })} /></label>
            <label>地址<input type="number" min={0} max={65535} value={pointForm.address} onChange={(event) => setPointForm({ ...pointForm, address: Number(event.target.value) })} /></label>
            <label>类型<select value={pointForm.io_type} onChange={(event) => setPointForm({ ...pointForm, io_type: event.target.value })}><option value="modbusHoldingRegister">保持寄存器</option><option value="modbusInputRegister">输入寄存器</option><option value="modbusCoil">线圈</option><option value="modbusDiscreteInput">离散输入</option></select></label>
            <label>数据格式<select value={pointForm.data_format} onChange={(event) => setPointForm({ ...pointForm, data_format: event.target.value })}><option value="uint16">uint16</option><option value="int16">int16</option><option value="uint32">uint32</option><option value="int32">int32</option><option value="float32">float32</option></select></label>
            <label>缩放<input type="number" step="any" value={pointForm.scale} onChange={(event) => setPointForm({ ...pointForm, scale: Number(event.target.value) })} /></label>
            <label>偏移<input type="number" step="any" value={pointForm.offset} onChange={(event) => setPointForm({ ...pointForm, offset: Number(event.target.value) })} /></label>
            <label>单位<input value={pointForm.units} onChange={(event) => setPointForm({ ...pointForm, units: event.target.value })} /></label>
            <label className="comm-check"><input type="checkbox" checked={pointForm.read_only} onChange={(event) => setPointForm({ ...pointForm, read_only: event.target.checked })} /> 只读</label>
          </div>
          <div className="comm-modal-actions"><button className="secondary-button" onClick={() => setPointForm(null)}>取消</button><button className="primary-button" disabled={busy || !pointForm.description.trim()} onClick={() => void savePoint()}>{busy ? "保存中…" : "保存点位"}</button></div>
        </section>
      </>, document.body)}
    </div>
  </section>
}
