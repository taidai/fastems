import { useEffect, useMemo, useState } from "react"
import { fetchHistory, Node, point, request } from "./api"
import SafetyModal from "./SafetyModal"
import ConfirmDialog from "./ConfirmDialog"

type Run = (action: () => Promise<unknown>) => Promise<boolean>

type Quality = "normal" | "stale" | "offline" | "unknown"
type PointRole = "telemetry" | "status" | "energy" | "setpoint" | "protection"
type PointRow = {
  node: Node
  name: string
  bus?: Node
  busName: string
  busAddress: string
  serverID: number
  address: number
  ioType: string
  format: string
  scale: number
  offset: number
  units: string
  readOnly: boolean
  disabled: boolean
  value?: number
  valueSet?: number
  valueTime: string
  errorCount: number
  command: string
  manual: boolean
  quality: Quality
  role: PointRole
}

const ioTypeNames: Record<string, string> = {
  modbusCoil: "线圈",
  modbusDiscreteInput: "离散输入",
  modbusHoldingRegister: "保持寄存器",
  modbusInputRegister: "输入寄存器",
}
const qualityNames: Record<Quality, string> = { normal: "正常", stale: "过期", offline: "离线", unknown: "无值" }
const formatOptions = ["uint16", "int16", "uint32", "int32", "float32"]
const ioTypeOptions = Object.keys(ioTypeNames)
const roleNames: Record<PointRole, string> = {
  telemetry: "遥测", status: "遥信", energy: "电度", setpoint: "定值", protection: "压板",
}

function pointRole(row: { ioType: string; readOnly: boolean; name: string; units: string }): PointRole {
  const coil = row.ioType === "modbusCoil" || row.ioType === "modbusCoilRead" || row.ioType === "modbusDiscreteInput"
  if (coil) return row.readOnly ? "status" : "protection"
  if (row.readOnly) return /kwh|mwh|电量|电能/i.test(`${row.name} ${row.units}`) ? "energy" : "telemetry"
  return "setpoint"
}

function roleLabel(role: PointRole) { return roleNames[role] || role }

const savedViewsKey = "easyems-point-views"
type SavedPointView = { name: string; query: string; bus: string; quality: string; access: string; role: string }
const qualityFilterNames: Record<string, string> = { normal: "正常", stale: "过期", offline: "离线", unknown: "无值", disabled: "已停用" }

function qualityReason(row: { disabled: boolean; quality: Quality; valueTime: string; errorCount: number }) {
  if (row.disabled) return "已停用"
  if (row.quality === "offline") return "通道或设备离线"
  if (row.quality === "stale") return row.valueTime ? "数据已过期" : "尚未采集"
  if (row.quality === "unknown") return row.errorCount > 0 ? "最近采集失败" : "无有效数据"
  return "数据有效"
}

function text(node: Node | undefined, type: string) {
  return node ? point(node, type)?.text || "" : ""
}
function numberValue(node: Node | undefined, type: string, fallback = 0) {
  const value = node ? point(node, type)?.value : undefined
  return typeof value === "number" ? value : fallback
}
function boolValue(node: Node, type: string) {
  return numberValue(node, type, 0) !== 0
}
function commandStatus(node: Node) {
  try {
    return JSON.parse(text(node, "lastCommand")).status || ""
  } catch {
    return ""
  }
}
function timeLabel(iso: string) {
  if (!iso) return "—"
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleTimeString("zh-CN", { hour12: false })
}
function ageLabel(iso: string) {
  if (!iso) return "无时间戳"
  const age = Math.max(0, Date.now() - new Date(iso).getTime())
  if (!Number.isFinite(age)) return "无时间戳"
  if (age < 1000) return "刚刚"
  return `${Math.round(age / 1000)} 秒前`
}
function busState(bus?: Node): "online" | "degraded" | "offline" | "unknown" {
  if (!bus) return "unknown"
  const last = numberValue(bus, "lastGoodPoll", 0)
  if (!last) return "offline"
  const poll = numberValue(bus, "pollPeriod", 1000)
  const timeout = numberValue(bus, "timeout", 1000)
  const maxAge = Math.max(5, (poll * 5) / 1000, (timeout * 3) / 1000)
  if (Date.now() / 1000 - last > maxAge) return "offline"
  if (numberValue(bus, "errorCount", 0) > 0) return "degraded"
  return "online"
}
function pointQuality(row: Pick<PointRow, "disabled" | "bus" | "value" | "valueTime" | "errorCount">): Quality {
  if (row.disabled) return "unknown"
  const state = busState(row.bus)
  if (state === "offline" || state === "unknown") return "offline"
  if (row.value === undefined || !row.valueTime) return "unknown"
  const age = Date.now() - new Date(row.valueTime).getTime()
  if (!Number.isFinite(age) || age > 15000) return "stale"
  if (state === "degraded" || row.errorCount > 0) return "stale"
  return "normal"
}
function displayValue(row: PointRow) {
  if (row.value === undefined) return "—"
  return `${row.value}${row.units ? ` ${row.units}` : ""}`
}
function csvCell(value: string | number) {
  const text = String(value)
  return /[",\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}
function downloadCSV(rows: PointRow[]) {
  const header = "名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位"
  const body = rows.map((row) => [
    row.name, row.serverID, row.address, ioTypeNames[row.ioType] || row.ioType,
    row.format, row.readOnly ? "是" : "否", row.scale, row.offset, row.units,
  ].map(csvCell).join(","))
  const blob = new Blob([[header, ...body].join("\n")], { type: "text/csv;charset=utf-8" })
  const url = URL.createObjectURL(blob)
  const link = document.createElement("a")
  link.href = url
  link.download = "easyems-points.csv"
  link.click()
  URL.revokeObjectURL(url)
}

function PointSparkline({ token, nodeId }: { token: string; nodeId: string }) {
  const [values, setValues] = useState<number[]>([])
  useEffect(() => {
    let cancelled = false
    const end = new Date()
    const start = new Date(end.getTime() - 3600_000)
    fetchHistory(token, nodeId, start, end).then((result) => {
      if (cancelled) return
      setValues((result.points || []).map((item) => Number(item.value)).filter((value) => Number.isFinite(value)))
    }).catch(() => { if (!cancelled) setValues([]) })
    return () => { cancelled = true }
  }, [nodeId, token])
  if (values.length < 2) return <div className="point-sparkline empty">最近一小时样本不足</div>
  const width = 260, height = 64, pad = 5
  const min = Math.min(...values), max = Math.max(...values), span = max - min || 1
  const points = values.map((value, index) => `${pad + (index / (values.length - 1)) * (width - pad * 2)},${height - pad - ((value - min) / span) * (height - pad * 2)}`).join(" ")
  return <div className="point-sparkline"><svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label="最近一小时趋势"><polyline points={points} fill="none" stroke="#2f6f9f" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" /><text x={pad} y="10" fontSize="9" fill="#7c8d9f">{max.toFixed(1)}</text><text x={pad} y={height - 2} fontSize="9" fill="#7c8d9f">{min.toFixed(1)}</text></svg></div>
}

export default function PointsView({ token, nodes, isAdmin, canOperate, busy, run, onError, openHistory, goChannels }: {
  token: string
  nodes: Node[]
  isAdmin: boolean
  canOperate: boolean
  busy: boolean
  run: Run
  onError: (message: string) => void
  openHistory: (nodeID: string) => void
  goChannels: () => void
}) {
  const [query, setQuery] = useState("")
  const [busFilter, setBusFilter] = useState("")
  const [qualityFilter, setQualityFilter] = useState("all")
  const [accessFilter, setAccessFilter] = useState("all")
  const [roleFilter, setRoleFilter] = useState("all")
  const [savedViews, setSavedViews] = useState<SavedPointView[]>(() => {
    try { return JSON.parse(localStorage.getItem(savedViewsKey) || "[]") as SavedPointView[] } catch { return [] }
  })
  const [selected, setSelected] = useState<Set<string>>(() => new Set())
  const [openID, setOpenID] = useState("")
  const [safetyOpen, setSafetyOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<PointRow | null>(null)
  const [writeValue, setWriteValue] = useState(0)
  const [pendingWrite, setPendingWrite] = useState(false)
  const [notice, setNotice] = useState("")
  const [batchEdit, setBatchEdit] = useState(false)
  const [batchScale, setBatchScale] = useState("1")
  const [batchOffset, setBatchOffset] = useState("0")
  const [batchNotice, setBatchNotice] = useState("")
  const [createOpen, setCreateOpen] = useState(false)
  const [createChannel, setCreateChannel] = useState("")
  const [createPrefix, setCreatePrefix] = useState("")
  const [createStartIndex, setCreateStartIndex] = useState("1")
  const [createServer, setCreateServer] = useState("1")
  const [createAddress, setCreateAddress] = useState("0")
  const [createCount, setCreateCount] = useState("1")
  const [createStep, setCreateStep] = useState("1")
  const [createType, setCreateType] = useState("modbusHoldingRegister")
  const [createFormat, setCreateFormat] = useState("uint16")
  const [createScale, setCreateScale] = useState("1")
  const [createOffset, setCreateOffset] = useState("0")
  const [createUnits, setCreateUnits] = useState("")
  const [createReadOnly, setCreateReadOnly] = useState(true)
  const [createDisabled, setCreateDisabled] = useState(false)
  const [createError, setCreateError] = useState("")
  const [createProgress, setCreateProgress] = useState("")

  const buses = useMemo(() => nodes.filter((node) => node.type === "modbus"), [nodes])
  const busMap = useMemo(() => new Map(buses.map((bus) => [bus.id, bus])), [buses])
  const rows = useMemo(() => nodes.filter((node) => node.type === "modbusIo").map((node) => {
    const bus = busMap.get(node.parent)
    const base = {
      node,
      name: text(node, "description") || node.id,
      bus,
      busName: text(bus, "description") || "未命名通道",
      busAddress: `${text(bus, "protocol") || "Modbus"} ${text(bus, "uri") || text(bus, "port") || ""}`.trim(),
      serverID: numberValue(node, "id", 1),
      address: numberValue(node, "address", 0),
      ioType: text(node, "modbusIoType"),
      format: text(node, "dataFormat") || "uint16",
      scale: numberValue(node, "scale", 1),
      offset: numberValue(node, "offset", 0),
      units: text(node, "units"),
      readOnly: boolValue(node, "readOnly"),
      disabled: boolValue(node, "disabled"),
      value: point(node, "value")?.value,
      valueSet: point(node, "valueSet")?.value,
      valueTime: point(node, "value")?.time || "",
      errorCount: numberValue(node, "errorCount", 0),
      command: commandStatus(node),
      manual: boolValue(node, "easyemsManual"),
    }
    return { ...base, quality: pointQuality(base), role: pointRole(base) }
  }).sort((a, b) => a.busName.localeCompare(b.busName, "zh-CN") || a.serverID - b.serverID || a.address - b.address), [nodes, busMap])

  const visible = useMemo(() => rows.filter((row) => {
    if (busFilter && row.node.parent !== busFilter) return false
    if (roleFilter !== "all" && row.role !== roleFilter) return false
    if (accessFilter === "readonly" && !row.readOnly) return false
    if (accessFilter === "writable" && row.readOnly) return false
    if (qualityFilter === "disabled" && !row.disabled) return false
    if (qualityFilter !== "all" && qualityFilter !== "disabled" && row.quality !== qualityFilter) return false
    if (!query.trim()) return true
    const needle = query.trim().toLowerCase()
    return [row.name, row.busName, String(row.address), String(row.serverID), row.units, row.format]
      .some((value) => value.toLowerCase().includes(needle))
  }), [rows, busFilter, accessFilter, qualityFilter, query])

  const openRow = rows.find((row) => row.node.id === openID) || null
  useEffect(() => {
    setSelected((previous) => {
      const ids = new Set(rows.map((row) => row.node.id))
      const next = new Set([...previous].filter((id) => ids.has(id)))
      return next.size === previous.size ? previous : next
    })
  }, [rows])
  useEffect(() => {
    if (openRow) setWriteValue(openRow.valueSet ?? openRow.value ?? 0)
  }, [openID])

  const counts = useMemo(() => ({
    total: rows.length,
    normal: rows.filter((row) => row.quality === "normal" && !row.disabled).length,
    stale: rows.filter((row) => row.quality === "stale" && !row.disabled).length,
    offline: rows.filter((row) => row.quality === "offline" && !row.disabled).length,
    writable: rows.filter((row) => !row.readOnly).length,
  }), [rows])
  const visibleIDs = visible.map((row) => row.node.id)
  const allVisible = visibleIDs.length > 0 && visibleIDs.every((id) => selected.has(id))
  const selectedRows = rows.filter((row) => selected.has(row.node.id))
  const createPointsInChannel = rows.filter((row) => row.node.parent === createChannel).length
  const createPreview = useMemo(() => {
    const prefix = createPrefix.trim()
    if (!prefix) return []
    const count = Number(createCount)
    const start = Number(createStartIndex)
    const server = Number(createServer)
    const address = Number(createAddress)
    const step = Number(createStep)
    if (!Number.isInteger(count) || count < 1 || count > 200) return []
    if (!Number.isInteger(start) || start < 0) return []
    if (!Number.isInteger(server) || server < 1 || server > 247) return []
    if (!Number.isInteger(address) || address < 0) return []
    if (!Number.isInteger(step) || step < 1) return []
    return Array.from({ length: count }, (_, index) => ({ name: `${prefix}${start + index}`, address: address + index * step }))
  }, [createCount, createStartIndex, createServer, createAddress, createStep, createPrefix])
  const createAddressWarning = useMemo(() => {
    if (!createChannel || !createPreview.length) return ""
    const existing = new Set(rows.filter((row) => row.node.parent === createChannel).map((row) => `${row.serverID}:${row.ioType}:${row.address}`))
    const overlap = createPreview.filter((point) => existing.has(`${Number(createServer)}:${createType}:${point.address}`)).length
    return overlap ? `${overlap} 个地址与当前通道现有点位重叠，请确认是否有意复用。` : ""
  }, [createChannel, createPreview, createServer, createType, rows])

  function toggle(id: string) {
    setSelected((previous) => {
      const next = new Set(previous)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }
  function toggleVisible() {
    setSelected((previous) => {
      const next = new Set(previous)
      if (allVisible) visibleIDs.forEach((id) => next.delete(id))
      else visibleIDs.forEach((id) => next.add(id))
      return next
    })
  }
  function pointInput(row: PointRow, patch: Record<string, unknown> = {}) {
    return {
      parent: row.node.parent,
      description: row.name,
      server_id: row.serverID,
      address: row.address,
      io_type: row.ioType,
      data_format: row.format,
      read_only: row.readOnly,
      scale: row.scale || 1,
      offset: row.offset,
      units: row.units,
      disabled: row.disabled,
      ...patch,
    }
  }
  function batchDisabled(disabled: boolean) {
    if (!selectedRows.length) return
    void run(async () => {
      for (const row of selectedRows) await request(token, `/modbus/io/${row.node.id}`, "PUT", pointInput(row, { disabled }))
    })
  }
  function batchDelete() {
    if (!selectedRows.length) return
    if (!window.confirm(`确定删除选中的 ${selectedRows.length} 个点位吗？此操作会同时删除点位配置与历史。`)) return
    void run(async () => {
      for (const row of selectedRows) await request(token, `/nodes/${row.node.id}`, "DELETE", { parent: row.node.parent })
    })
    setSelected(new Set())
  }
  function changeCreateType(value: string) {
    setCreateType(value)
    if (value === "modbusInputRegister" || value === "modbusDiscreteInput") setCreateReadOnly(true)
    if (value === "modbusCoil" || value === "modbusDiscreteInput") setCreateFormat("uint16")
  }
  function openCreate() {
    setCreateChannel(busFilter || buses[0]?.id || "")
    setCreatePrefix("")
    setCreateStartIndex("1")
    setCreateServer("1")
    setCreateAddress("0")
    setCreateCount("1")
    setCreateStep("1")
    setCreateType("modbusHoldingRegister")
    setCreateFormat("uint16")
    setCreateScale("1")
    setCreateOffset("0")
    setCreateUnits("")
    setCreateReadOnly(true)
    setCreateDisabled(false)
    setCreateError("")
    setCreateProgress("")
    setCreateOpen(true)
  }
  function validateCreate(): string {
    if (!buses.length) return "还没有 Modbus 通道，请先在通道视图接入。"
    if (!createChannel) return "请选择通道"
    if (!createPrefix.trim()) return "名称前缀不能为空"
    const count = Number(createCount)
    const start = Number(createStartIndex)
    const server = Number(createServer)
    const address = Number(createAddress)
    const step = Number(createStep)
    if (!Number.isInteger(count) || count < 1 || count > 200) return "数量必须是 1–200 的整数"
    if (!Number.isInteger(start) || start < 0) return "起始序号必须是非负整数"
    if (!Number.isInteger(server) || server < 1 || server > 247) return "站号必须是 1–247"
    if (!Number.isInteger(address) || address < 0) return "起始地址必须是非负整数"
    if (!Number.isInteger(step) || step < 1) return "地址步长必须是正整数"
    const scale = Number(createScale)
    const offset = Number(createOffset)
    if (!Number.isFinite(scale) || scale === 0) return "缩放必须是非 0 数字"
    if (!Number.isFinite(offset)) return "偏移必须是数字"
    const maxAddress = ["uint32", "int32", "float32"].includes(createFormat) ? 65534 : 65535
    const lastAddress = address + (count - 1) * step
    if (lastAddress > maxAddress) return `最后一个地址 ${lastAddress} 超出范围（最大 ${maxAddress}）`
    const existing = new Set(rows.filter((row) => row.node.parent === createChannel).map((row) => row.name))
    const clash = createPreview.find((point) => existing.has(point.name))
    if (clash) return `名称与现有点位冲突：${clash.name}`
    return ""
  }
  async function createPoints() {
    const problem = validateCreate()
    if (problem) {
      setCreateError(problem)
      return
    }
    setCreateError("")
    const inputs = createPreview.map((point) => ({
      parent: createChannel,
      description: point.name,
      server_id: Number(createServer),
      address: point.address,
      io_type: createType,
      data_format: createType === "modbusCoil" || createType === "modbusDiscreteInput" ? "uint16" : createFormat,
      read_only: createReadOnly,
      scale: Number(createScale) || 1,
      offset: Number(createOffset),
      units: createUnits,
      disabled: createDisabled,
    }))
    const created: string[] = []
    setCreateProgress(`创建中 0/${inputs.length}`)
    const ok = await run(async () => {
      try {
        for (let index = 0; index < inputs.length; index += 1) {
          const result = await request<{ id: string }>(token, `/modbus/${createChannel}/io`, "POST", inputs[index])
          created.push(result.id)
          setCreateProgress(`创建中 ${index + 1}/${inputs.length}`)
        }
      } catch (cause) {
        for (const id of [...created].reverse()) {
          try { await request(token, `/nodes/${id}`, "DELETE", { parent: createChannel }) } catch { /* best-effort rollback */ }
        }
        throw new Error(`批量新增失败：${cause instanceof Error ? cause.message : "未知错误"}；已回滚 ${created.length} 个点位`)
      }
    })
    setCreateProgress("")
    if (ok) {
      setCreateOpen(false)
      setBatchNotice(`已新增 ${created.length} 个点位`)
      setBusFilter(createChannel)
      setQuery("")
      setSelected(new Set(created))
    }
  }
  useEffect(() => {
    if (!createOpen) return
    const close = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) setCreateOpen(false)
    }
    document.addEventListener("keydown", close)
    return () => document.removeEventListener("keydown", close)
  }, [createOpen, busy])

  function openBatchEdit() {
    if (!selectedRows.length) return
    setBatchScale(String(selectedRows[0].scale || 1))
    setBatchOffset(String(selectedRows[0].offset || 0))
    setBatchNotice("")
    setBatchEdit(true)
  }
  function applyBatchScaleOffset() {
    const scale = Number(batchScale)
    const offset = Number(batchOffset)
    if (!Number.isFinite(scale) || scale === 0) {
      onError("缩放必须是非 0 数字")
      return
    }
    if (!Number.isFinite(offset)) {
      onError("偏移必须是数字")
      return
    }
    if (!selectedRows.length) return
    const count = selectedRows.length
    void run(async () => {
      for (const row of selectedRows) {
        await request(token, `/modbus/io/${row.node.id}`, "PUT", pointInput(row, { scale, offset }))
      }
      setBatchEdit(false)
      setBatchNotice(`已更新 ${count} 个点位的缩放 / 偏移`)
    })
  }

  function writePoint() {
    if (!openRow || openRow.readOnly) return
    setPendingWrite(true)
  }
  function confirmWrite() {
    if (!openRow || openRow.readOnly) return
    const target = openRow
    const value = writeValue
    setPendingWrite(false)
    void run(async () => {
      await request(token, `/nodes/${target.node.id}/value`, "POST", { value, point_type: "valueSet" })
      setNotice(`已下发 ${value}${target.units ? ` ${target.units}` : ""}，等待回读。`)
    })
  }
  function toggleManual() {
    if (!openRow) return
    void run(() => request(token, `/nodes/${openRow.node.id}/manual`, "POST", { manual: !openRow.manual }))
  }
  function startEdit() {
    if (!openRow) return
    setDraft({ ...openRow })
    setEditing(true)
  }
  function saveEdit() {
    if (!draft) return
    if (!draft.name.trim() || draft.address < 0 || draft.serverID < 1 || draft.serverID > 247) {
      onError("名称不能为空，站号 1–247，地址不能为负")
      return
    }
    void run(async () => {
      await request(token, `/modbus/io/${draft.node.id}`, "PUT", pointInput(draft, {
        description: draft.name.trim(), server_id: draft.serverID, address: draft.address,
        io_type: draft.ioType, data_format: draft.format, read_only: draft.readOnly,
        scale: draft.scale || 1, offset: draft.offset, units: draft.units, disabled: draft.disabled,
      }))
      setEditing(false)
      setDraft(null)
      setNotice("点位配置已保存。")
    })
  }

  return <section className="points-page">
    <div className="point-stats">
      <span><strong>{counts.total}</strong> 全部</span>
      <span className="ok"><strong>{counts.normal}</strong> 正常</span>
      <span className="warn"><strong>{counts.stale}</strong> 过期</span>
      <span className="bad"><strong>{counts.offline}</strong> 离线</span>
      <span><strong>{counts.writable}</strong> 可写</span>
      <span><strong>{selected.size}</strong> 已选</span>
      <span className="point-live-note"><i />实时值每 2 秒刷新</span>
    </div>

    <div className="panel point-shell">
      <div className="point-toolbar">
        <input aria-label="搜索点位" placeholder="搜索名称、通道、地址或单位" value={query} onChange={(event) => setQuery(event.target.value)} />
        <select aria-label="过滤通道" value={busFilter} onChange={(event) => setBusFilter(event.target.value)}>
          <option value="">全部通道</option>
          {buses.map((bus) => <option key={bus.id} value={bus.id}>{text(bus, "description") || bus.id}</option>)}
        </select>
        <select aria-label="过滤质量" value={qualityFilter} onChange={(event) => setQualityFilter(event.target.value)}>
          <option value="all">全部质量</option><option value="normal">正常</option><option value="stale">过期</option>
          <option value="offline">离线</option><option value="unknown">无值</option><option value="disabled">已停用</option>
        </select>
        <select aria-label="过滤角色" value={roleFilter} onChange={(event) => setRoleFilter(event.target.value)}>
          <option value="all">全部角色</option><option value="telemetry">遥测</option><option value="status">遥信</option>
          <option value="energy">电度</option><option value="setpoint">定值</option><option value="protection">压板</option>
        </select>
        <select aria-label="过滤读写" value={accessFilter} onChange={(event) => setAccessFilter(event.target.value)}>
          <option value="all">全部读写</option><option value="writable">可写</option><option value="readonly">只读</option>
        </select>
        {(() => {
          const isDefault = !query.trim() && !busFilter && qualityFilter === "all" && roleFilter === "all" && accessFilter === "all"
          const viewName = [
            query.trim() ? `搜"${query.trim()}"` : "",
            busFilter ? (buses.find((bus) => bus.id === busFilter) ? text(buses.find((bus) => bus.id === busFilter)!, "description") || busFilter : busFilter) : "",
            qualityFilter !== "all" ? qualityFilterNames[qualityFilter] || qualityFilter : "",
            roleFilter !== "all" ? roleNames[roleFilter as PointRole] || roleFilter : "",
            accessFilter !== "all" ? (accessFilter === "writable" ? "可写" : "只读") : "",
          ].filter(Boolean).join("·")
          const matched = savedViews.find((view) => view.query === query && view.bus === busFilter && view.quality === qualityFilter && view.role === roleFilter && view.access === accessFilter)
          const saveViews = (views: SavedPointView[]) => { setSavedViews(views); localStorage.setItem(savedViewsKey, JSON.stringify(views)) }
          return <>
            {savedViews.length > 0 && <select aria-label="保存的视图" value="" onChange={(event) => {
              const view = savedViews.find((item) => item.name === event.target.value)
              if (!view) return
              setQuery(view.query); setBusFilter(view.bus); setQualityFilter(view.quality); setRoleFilter(view.role); setAccessFilter(view.access)
            }}>
              <option value="">视图…</option>
              {savedViews.map((view) => <option key={view.name} value={view.name}>{view.name}</option>)}
            </select>}
            {matched
              ? <button className="secondary-button" onClick={() => saveViews(savedViews.filter((view) => view !== matched))}>删视图</button>
              : <button className="secondary-button" disabled={isDefault} title={isDefault ? "先设置筛选条件" : undefined} onClick={() => {
                  const name = viewName || "视图"
                  saveViews([...savedViews.filter((view) => view.name !== name), { name, query, bus: busFilter, quality: qualityFilter, role: roleFilter, access: accessFilter }])
                }}>存视图</button>}
          </>
        })()}
        <button className="secondary-button" onClick={goChannels}>通道视图</button>
        {isAdmin && <button className="primary-button" disabled={!buses.length || busy} title={!buses.length ? "先接入 Modbus 通道" : undefined} onClick={openCreate}>+ 批量新增</button>}
      </div>

      <div className="point-batchbar">
        <label className="point-check"><input type="checkbox" checked={allVisible} onChange={toggleVisible} /> 当前筛选全选</label>
        <span>已选 {selected.size} / 筛选 {visible.length} / 全部 {rows.length}</span>
        <button className="secondary-button" disabled={!visible.length} onClick={() => downloadCSV(selectedRows.length ? selectedRows : visible)}>导出 CSV</button>
        {isAdmin && <button className="secondary-button" disabled={!selected.size || busy} onClick={openBatchEdit}>批量改缩放/偏移</button>}
        {isAdmin && <button className="secondary-button" disabled={!selected.size || busy} onClick={() => batchDisabled(true)}>批量停用</button>}
        {isAdmin && <button className="secondary-button" disabled={!selected.size || busy} onClick={() => batchDisabled(false)}>批量启用</button>}
        {isAdmin && <button className="danger-button" disabled={!selected.size || busy} onClick={batchDelete}>批量删除</button>}
        {batchNotice && <span className="point-batch-ok">{batchNotice}</span>}
      </div>

      {batchEdit && <div className="point-batch-edit">
        <div><strong>批量修改缩放 / 偏移</strong><span>已选 {selectedRows.length} 个点位；逐点保存，失败时已成功的点位保留新值。</span></div>
        <label>缩放<input aria-label="批量缩放" type="number" step="any" value={batchScale} onChange={(event) => setBatchScale(event.target.value)} /></label>
        <label>偏移<input aria-label="批量偏移" type="number" step="any" value={batchOffset} onChange={(event) => setBatchOffset(event.target.value)} /></label>
        <button className="primary-button" disabled={busy || !selectedRows.length} onClick={applyBatchScaleOffset}>应用到已选</button>
        <button className="secondary-button" disabled={busy} onClick={() => setBatchEdit(false)}>取消</button>
      </div>}

      <div className="point-table-wrap">
        <table className="point-table">
          <thead><tr>
            <th className="check"><input type="checkbox" aria-label="全选当前筛选" checked={allVisible} onChange={toggleVisible} /></th>
            <th>点位名称</th><th>角色</th><th>通道</th><th>站号</th><th>类型</th><th>地址</th><th>格式</th><th>缩放 / 单位</th><th>当前值</th><th>质量</th><th>更新时间</th><th>操作</th>
          </tr></thead>
          <tbody>
            {visible.map((row) => <tr key={row.node.id} className={`${row.disabled ? "disabled" : ""} ${selected.has(row.node.id) ? "selected" : ""} ${openID === row.node.id ? "open" : ""}`} onClick={() => { setOpenID(row.node.id); setEditing(false); setNotice("") }}>
              <td className="check" onClick={(event) => event.stopPropagation()}><input type="checkbox" aria-label={`选择 ${row.name}`} checked={selected.has(row.node.id)} onChange={() => toggle(row.node.id)} /></td>
              <td><strong>{row.name}</strong>{row.readOnly && <span className="point-tag">只读</span>}{row.disabled && <span className="point-tag warn">停用</span>}</td>
              <td><span className={`point-role ${row.role}`}>{roleLabel(row.role)}</span></td>
              <td><span>{row.busName}</span><small>{row.busAddress}</small></td>
              <td>{row.serverID}</td>
              <td>{ioTypeNames[row.ioType] || row.ioType}</td>
              <td>{row.address}</td>
              <td>{row.format}</td>
              <td>{row.scale} / {row.units || "—"}</td>
              <td className={`point-value ${row.quality}`}>{displayValue(row)}</td>
              <td><span className={`point-quality ${row.quality}`}>{row.disabled ? "已停用" : qualityNames[row.quality]}</span><small className="point-quality-reason">{qualityReason(row)}</small></td>
              <td title={row.valueTime}>{timeLabel(row.valueTime)}<small>{ageLabel(row.valueTime)}</small></td>
              <td><button className="manage-button" onClick={(event) => { event.stopPropagation(); setOpenID(row.node.id); setEditing(false); setNotice("") }}>详情</button></td>
            </tr>)}
            {visible.length === 0 && <tr><td colSpan={13}><div className="point-empty">{rows.length ? "没有匹配的点位，调整筛选条件。" : "还没有 Modbus 点位。到通道视图接入设备，或在部署页预检并导入点表。"}</div></td></tr>}
          </tbody>
        </table>
      </div>
    </div>

    {createOpen && <>
      <div className="point-create-backdrop" onClick={() => { if (!busy) setCreateOpen(false) }} />
      <div className="point-create" role="dialog" aria-label="批量新增点位">
        <div className="point-create-head">
          <div><span className="section-kicker">按通道批量新增</span><h2>批量新增点位</h2><p>生成连续地址点位；先看预览，再创建。</p></div>
          <button className="icon-button" aria-label="关闭批量新增" disabled={busy} onClick={() => setCreateOpen(false)}>×</button>
        </div>
        <div className="point-create-grid">
          <label>通道<select value={createChannel} onChange={(event) => setCreateChannel(event.target.value)}>
            {buses.map((bus) => <option key={bus.id} value={bus.id}>{text(bus, "description") || bus.id}</option>)}
          </select></label>
          <label>名称前缀<input value={createPrefix} onChange={(event) => setCreatePrefix(event.target.value)} placeholder="例如：电芯温度-" /></label>
          <label>起始序号<input type="number" min={0} value={createStartIndex} onChange={(event) => setCreateStartIndex(event.target.value)} /></label>
          <label>站号<input type="number" min={1} max={247} value={createServer} onChange={(event) => setCreateServer(event.target.value)} /></label>
          <label>起始地址<input type="number" min={0} value={createAddress} onChange={(event) => setCreateAddress(event.target.value)} /></label>
          <label>数量<input type="number" min={1} max={200} value={createCount} onChange={(event) => setCreateCount(event.target.value)} /></label>
          <label>地址步长<input type="number" min={1} value={createStep} onChange={(event) => setCreateStep(event.target.value)} /></label>
          <label>类型<select value={createType} onChange={(event) => changeCreateType(event.target.value)}>{ioTypeOptions.map((value) => <option key={value} value={value}>{ioTypeNames[value]}</option>)}</select></label>
          <label>数据格式<select value={createFormat} disabled={createType === "modbusCoil" || createType === "modbusDiscreteInput"} onChange={(event) => setCreateFormat(event.target.value)}>{formatOptions.map((value) => <option key={value} value={value}>{value}</option>)}</select></label>
          <label>缩放<input type="number" step="any" value={createScale} onChange={(event) => setCreateScale(event.target.value)} /></label>
          <label>偏移<input type="number" step="any" value={createOffset} onChange={(event) => setCreateOffset(event.target.value)} /></label>
          <label>单位<input value={createUnits} onChange={(event) => setCreateUnits(event.target.value)} /></label>
        </div>
        <div className="point-create-checks">
          <label className="point-check"><input type="checkbox" checked={createReadOnly} disabled={createType === "modbusInputRegister" || createType === "modbusDiscreteInput"} onChange={(event) => setCreateReadOnly(event.target.checked)} /> 只读</label>
          <label className="point-check"><input type="checkbox" checked={createDisabled} onChange={(event) => setCreateDisabled(event.target.checked)} /> 停用</label>
          <span>当前通道 {createPointsInChannel} 个点位</span>
        </div>
        <div className="point-create-preview">
          <strong>预览</strong>
          {createPreview.length ? <span>{createPreview.slice(0, 3).map((point) => `${point.name} @ ${point.address}`).join("、")}{createPreview.length > 3 ? ` … ${createPreview[createPreview.length - 1].name} @ ${createPreview[createPreview.length - 1].address}` : ""}</span> : <span>填写通道、前缀、起始地址和数量后显示预览。</span>}
          {createAddressWarning && <em>{createAddressWarning}</em>}
        </div>
        {createError && <div className="inline-error" role="alert">{createError}</div>}
        <div className="point-create-actions">
          <button className="secondary-button" disabled={busy} onClick={() => setCreateOpen(false)}>取消</button>
          <button className="primary-button" disabled={busy} onClick={() => void createPoints()}>{busy ? (createProgress || "创建中…") : (createPreview.length ? `创建 ${createPreview.length} 个点位` : "创建点位")}</button>
        </div>
      </div>
    </>}

    {openRow && <>
      <div className="point-drawer-backdrop" onClick={() => { setOpenID(""); setEditing(false); setDraft(null) }} />
      <aside className="point-drawer" role="dialog" aria-label={`${openRow.name} 详情`}>
        <div className="point-drawer-head">
          <div><span className="section-kicker">点位详情</span><h2>{openRow.name}</h2></div>
          <button className="icon-button" aria-label="关闭点位详情" onClick={() => { setOpenID(""); setEditing(false); setDraft(null) }}>×</button>
        </div>
        <div className={`point-drawer-live ${openRow.quality}`}>
          <strong>{displayValue(openRow)}</strong>
          <span>{openRow.disabled ? "已停用" : qualityNames[openRow.quality]} · {ageLabel(openRow.valueTime)} · {qualityReason(openRow)}</span>
        </div>
        <PointSparkline token={token} nodeId={openRow.node.id} />
        {openRow.command && <div className="point-command">最近命令：{({ accepted: "已接受", confirmed: "已回读", unknown: "结果未知", failed: "失败" } as Record<string, string>)[openRow.command] || openRow.command}</div>}
        {notice && <div className="success-message" role="status">{notice}</div>}

        {!openRow.readOnly && canOperate && <div className="point-write">
          <strong>下发目标值</strong>
          <div><input aria-label="目标值" type="number" step="any" value={writeValue} onChange={(event) => setWriteValue(Number(event.target.value))} /><button className="primary-button" disabled={busy} onClick={writePoint}>下发</button></div>
          <small>{openRow.valueSet !== undefined ? `当前目标 ${openRow.valueSet}${openRow.units ? ` ${openRow.units}` : ""}` : "该点位还没有目标值"}</small>
        </div>}

        <div className="point-actions">
          <button className="secondary-button" onClick={() => openHistory(openRow.node.id)}>查看历史</button>
          {canOperate && <button className="secondary-button" disabled={busy} onClick={toggleManual}>{openRow.manual ? "恢复自动" : "接管自动"}</button>}
          {isAdmin && !openRow.readOnly && <button className="secondary-button" onClick={() => setSafetyOpen(true)}>安全约束{point(openRow.node, "easyemsSafety")?.text ? "" : " · 未配置"}</button>}
          {isAdmin && !editing && <button className="secondary-button" onClick={startEdit}>编辑点位</button>}
        </div>

        {editing && draft ? <div className="point-edit">
          <label>名称<input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label>
          <div className="point-edit-row">
            <label>站号<input type="number" min={1} max={247} value={draft.serverID} onChange={(event) => setDraft({ ...draft, serverID: Number(event.target.value) })} /></label>
            <label>地址<input type="number" min={0} value={draft.address} onChange={(event) => setDraft({ ...draft, address: Number(event.target.value) })} /></label>
          </div>
          <div className="point-edit-row">
            <label>类型<select value={draft.ioType} onChange={(event) => setDraft({ ...draft, ioType: event.target.value })}>{ioTypeOptions.map((value) => <option key={value} value={value}>{ioTypeNames[value]}</option>)}</select></label>
            <label>数据格式<select value={draft.format} onChange={(event) => setDraft({ ...draft, format: event.target.value })}>{formatOptions.map((value) => <option key={value} value={value}>{value}</option>)}</select></label>
          </div>
          <div className="point-edit-row">
            <label>缩放<input type="number" step="any" value={draft.scale} onChange={(event) => setDraft({ ...draft, scale: Number(event.target.value) })} /></label>
            <label>偏移<input type="number" step="any" value={draft.offset} onChange={(event) => setDraft({ ...draft, offset: Number(event.target.value) })} /></label>
            <label>单位<input value={draft.units} onChange={(event) => setDraft({ ...draft, units: event.target.value })} /></label>
          </div>
          <label className="point-check"><input type="checkbox" checked={draft.readOnly} onChange={(event) => setDraft({ ...draft, readOnly: event.target.checked })} /> 只读</label>
          <label className="point-check"><input type="checkbox" checked={draft.disabled} onChange={(event) => setDraft({ ...draft, disabled: event.target.checked })} /> 停用</label>
          <div className="point-edit-actions"><button className="secondary-button" onClick={() => { setEditing(false); setDraft(null) }}>取消</button><button className="primary-button" disabled={busy} onClick={saveEdit}>保存点位</button></div>
        </div> : <>
          <dl className="point-meta">
            <div><dt>通道</dt><dd>{openRow.busName} · {openRow.busAddress}</dd></div>
            <div><dt>站号 / 地址</dt><dd>{openRow.serverID} / {openRow.address}</dd></div>
            <div><dt>类型 / 格式</dt><dd>{ioTypeNames[openRow.ioType] || openRow.ioType} · {openRow.format}</dd></div>
            <div><dt>缩放 / 偏移 / 单位</dt><dd>{openRow.scale} / {openRow.offset} / {openRow.units || "—"}</dd></div>
            <div><dt>读写</dt><dd>{openRow.readOnly ? "只读" : "可写"} · {openRow.disabled ? "已停用" : "启用"}</dd></div>
            <div><dt>业务 ID</dt><dd className="point-id">{text(openRow.node, "businessId") || openRow.node.id}</dd></div>
          </dl>
        </>}
      </aside>
      {safetyOpen && <SafetyModal token={token} node={openRow.node} nodes={nodes} onClose={() => setSafetyOpen(false)} onSaved={() => { setSafetyOpen(false); setNotice("安全约束已保存") }} />}
    </>}

    {pendingWrite && openRow && <ConfirmDialog
      title={`确认下发「${openRow.name}」`}
      message="目标值会直接写入设备，等待回读确认；失败不会自动重试。"
      rows={[
        { label: "当前值", value: `${displayValue(openRow)}${openRow.units ? ` ${openRow.units}` : ""}` },
        { label: "目标值", value: `${writeValue}${openRow.units ? ` ${openRow.units}` : ""}` },
      ]}
      confirmLabel="确认下发"
      busy={busy}
      onCancel={() => setPendingWrite(false)}
      onConfirm={confirmWrite}
    />}
  </section>
}
