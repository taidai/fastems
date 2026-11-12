import { useCallback, useEffect, useMemo, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { fetchHistory, Node, point, request } from "./api"
import SafetyModal from "./SafetyModal"
import { BatchActionBar, EmsButton, EmsCard, EmsInput, EmsOverlay, EmsRecordList, EmsSelect, EmsSelectOption, EmptyState, FilterToolbar, InlineNotice, MetricStrip, StatusBadge, type EmsTone } from "./components/ems"

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
const qualityTones: Record<Quality, EmsTone> = { normal: "success", stale: "warning", offline: "danger", unknown: "neutral" }
const qualityValueColors: Record<Quality, string> = { normal: "text-ems-ink", stale: "text-ems-amber", offline: "text-ems-danger", unknown: "text-ems-faint" }
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

const savedViewsKey = "fastems-point-views"
type SavedPointView = { name: string; query: string; bus: string; quality: string; access: string; role: string }
const qualityFilterNames: Record<string, string> = { attention: "需处理", normal: "正常", stale: "过期", offline: "离线", unknown: "无值", disabled: "已停用" }

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
  link.download = "fastiot-points.csv"
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
  if (values.length < 2) return <div className="rounded-xl border border-dashed border-ems-line px-3 py-4 text-center text-xs text-ems-muted">最近一小时样本不足</div>
  const width = 260, height = 64, pad = 5
  const min = Math.min(...values), max = Math.max(...values), span = max - min || 1
  const points = values.map((value, index) => `${pad + (index / (values.length - 1)) * (width - pad * 2)},${height - pad - ((value - min) / span) * (height - pad * 2)}`).join(" ")
  return <div className="rounded-xl border border-ems-line bg-white/60 p-2"><svg className="h-20 w-full" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="最近一小时趋势"><polyline points={points} fill="none" stroke="#0e7c7b" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" /><text x={pad} y="10" fontSize="9" fill="#6b7c88">{max.toFixed(1)}</text><text x={pad} y={height - 2} fontSize="9" fill="#6b7c88">{min.toFixed(1)}</text></svg></div>
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
  const [batchWriteOpen, setBatchWriteOpen] = useState(false)
  const [batchWriteValue, setBatchWriteValue] = useState(0)
  const [pendingBatchWrite, setPendingBatchWrite] = useState(false)
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
  const closeCreate = useCallback(() => setCreateOpen(false), [])
  const closeDetail = useCallback(() => { setOpenID(""); setEditing(false); setDraft(null) }, [])
  const closeBatchEdit = useCallback(() => setBatchEdit(false), [])
  const closeBatchWrite = useCallback(() => setBatchWriteOpen(false), [])
  const cancelPendingWrite = useCallback(() => setPendingWrite(false), [])
  const cancelPendingBatchWrite = useCallback(() => setPendingBatchWrite(false), [])

  function showDetail(row: PointRow) {
    setOpenID(row.node.id)
    setEditing(false)
    setNotice("")
  }

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
    if (qualityFilter === "attention" && row.quality === "normal" && !row.disabled) return false
    if (qualityFilter === "disabled" && !row.disabled) return false
    if (qualityFilter !== "all" && qualityFilter !== "disabled" && row.quality !== qualityFilter) return false
    if (!query.trim()) return true
    const needle = query.trim().toLowerCase()
    return [row.name, row.busName, String(row.address), String(row.serverID), row.units, row.format]
      .some((value) => value.toLowerCase().includes(needle))
  }), [rows, busFilter, accessFilter, qualityFilter, roleFilter, query])

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
  const writableSelected = selectedRows.filter((row) => !row.readOnly && !row.disabled)
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
  function openBatchWrite() {
    if (!writableSelected.length) return
    setBatchWriteValue(writableSelected[0].valueSet ?? writableSelected[0].value ?? 0)
    setBatchNotice("")
    setBatchWriteOpen(true)
  }
  function doBatchWrite() {
    if (!writableSelected.length) return
    const value = batchWriteValue
    if (!Number.isFinite(value)) {
      onError("目标值必须是数字")
      return
    }
    if (writableSelected.some((row) => row.ioType === "modbusCoil") && value !== 0 && value !== 1) {
      onError("选中的点位包含线圈，目标值必须是 0 或 1")
      return
    }
    const count = writableSelected.length
    setPendingBatchWrite(false)
    void run(async () => {
      for (const row of writableSelected) {
        await request(token, `/nodes/${row.node.id}/value`, "POST", { value, point_type: "valueSet" })
      }
      setBatchWriteOpen(false)
      setBatchNotice(`已下发 ${value} 到 ${count} 个点位，逐点回读中。`)
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

  const isDefaultView = !query.trim() && !busFilter && qualityFilter === "all" && roleFilter === "all" && accessFilter === "all"
  const currentViewName = [
    query.trim() ? `搜"${query.trim()}"` : "",
    busFilter ? (buses.find((bus) => bus.id === busFilter) ? text(buses.find((bus) => bus.id === busFilter)!, "description") || busFilter : busFilter) : "",
    qualityFilter !== "all" ? qualityFilterNames[qualityFilter] || qualityFilter : "",
    roleFilter !== "all" ? roleNames[roleFilter as PointRole] || roleFilter : "",
    accessFilter !== "all" ? (accessFilter === "writable" ? "可写" : "只读") : "",
  ].filter(Boolean).join("·")
  const matchedView = savedViews.find((view) => view.query === query && view.bus === busFilter && view.quality === qualityFilter && view.role === roleFilter && view.access === accessFilter)
  function saveViews(views: SavedPointView[]) {
    setSavedViews(views)
    localStorage.setItem(savedViewsKey, JSON.stringify(views))
  }

  return <section className="points-page grid min-w-0 gap-4">
    <MetricStrip items={[
      { label: "全部点位", value: counts.total, note: "实时值每 2 秒刷新" },
      { label: "正常", value: counts.normal, tone: "success" },
      { label: "过期", value: counts.stale, tone: counts.stale ? "warning" : "neutral" },
      { label: "离线", value: counts.offline, tone: counts.offline ? "danger" : "neutral" },
      { label: "可写", value: counts.writable, tone: "info" },
      { label: "已选", value: selected.size },
    ]} />

    <EmsCard><IonCardContent className="grid gap-4">
      <div className="min-w-0 [&>div>div:last-child]:basis-full lg:[&>div>div:last-child]:basis-auto [&_.ems-field:first-child]:basis-full lg:[&_.ems-field:first-child]:basis-auto">
      <FilterToolbar trailing={<div className="flex w-full flex-wrap items-center gap-2 sm:w-auto">
        <EmsButton variant="secondary" onClick={goChannels}>通道视图</EmsButton>
        {isAdmin && <EmsButton disabled={!buses.length || busy} title={!buses.length ? "先接入 Modbus 通道" : undefined} onClick={openCreate}>+ 批量新增</EmsButton>}
      </div>}>
        <EmsInput className="min-w-[220px] flex-[2_1_240px]" label="搜索点位" aria-label="搜索点位" placeholder="搜索名称、通道、地址或单位" value={query} onIonInput={(event) => setQuery(event.detail.value || "")} />
        <EmsSelect label="通道" aria-label="过滤通道" value={busFilter} onIonChange={(event) => setBusFilter(event.detail.value)}>
          <EmsSelectOption value="">全部通道</EmsSelectOption>
          {buses.map((bus) => <EmsSelectOption key={bus.id} value={bus.id}>{text(bus, "description") || bus.id}</EmsSelectOption>)}
        </EmsSelect>
        <EmsSelect label="质量" aria-label="过滤质量" value={qualityFilter} onIonChange={(event) => setQualityFilter(event.detail.value)}>
          <EmsSelectOption value="all">全部质量</EmsSelectOption><EmsSelectOption value="attention">需处理</EmsSelectOption><EmsSelectOption value="normal">正常</EmsSelectOption><EmsSelectOption value="stale">过期</EmsSelectOption>
          <EmsSelectOption value="offline">离线</EmsSelectOption><EmsSelectOption value="unknown">无值</EmsSelectOption><EmsSelectOption value="disabled">已停用</EmsSelectOption>
        </EmsSelect>
        <EmsSelect label="角色" aria-label="过滤角色" value={roleFilter} onIonChange={(event) => setRoleFilter(event.detail.value)}>
          <EmsSelectOption value="all">全部角色</EmsSelectOption><EmsSelectOption value="telemetry">遥测</EmsSelectOption><EmsSelectOption value="status">遥信</EmsSelectOption>
          <EmsSelectOption value="energy">电度</EmsSelectOption><EmsSelectOption value="setpoint">定值</EmsSelectOption><EmsSelectOption value="protection">压板</EmsSelectOption>
        </EmsSelect>
        <EmsSelect label="读写" aria-label="过滤读写" value={accessFilter} onIonChange={(event) => setAccessFilter(event.detail.value)}>
          <EmsSelectOption value="all">全部读写</EmsSelectOption><EmsSelectOption value="writable">可写</EmsSelectOption><EmsSelectOption value="readonly">只读</EmsSelectOption>
        </EmsSelect>
        {savedViews.length > 0 && <EmsSelect label="已存视图" aria-label="保存的视图" value="" onIonChange={(event) => {
          const view = savedViews.find((item) => item.name === event.detail.value)
          if (!view) return
          setQuery(view.query); setBusFilter(view.bus); setQualityFilter(view.quality); setRoleFilter(view.role); setAccessFilter(view.access)
        }}>
          <EmsSelectOption value="">视图…</EmsSelectOption>
          {savedViews.map((view) => <EmsSelectOption key={view.name} value={view.name}>{view.name}</EmsSelectOption>)}
        </EmsSelect>}
        <EmsButton variant="secondary" disabled={!matchedView && isDefaultView} title={!matchedView && isDefaultView ? "先设置筛选条件" : undefined} onClick={() => {
          if (matchedView) {
            saveViews(savedViews.filter((view) => view !== matchedView))
          } else {
            const name = currentViewName || "视图"
            saveViews([...savedViews.filter((view) => view.name !== name), { name, query, bus: busFilter, quality: qualityFilter, role: roleFilter, access: accessFilter }])
          }
        }}>{matchedView ? "删视图" : "存视图"}</EmsButton>
      </FilterToolbar>
      </div>

      <div className="[&_.ems-batch-bar]:flex-wrap">
        <BatchActionBar count={selected.size} trailing={<>
          <EmsButton variant="secondary" size="small" disabled={!visible.length} onClick={() => downloadCSV(selectedRows.length ? selectedRows : visible)}>导出 CSV</EmsButton>
          {isAdmin && <EmsButton variant="secondary" size="small" disabled={!selected.size || busy} onClick={openBatchEdit}>批量改缩放/偏移</EmsButton>}
          {isAdmin && <EmsButton variant="secondary" size="small" disabled={!selected.size || busy} onClick={() => batchDisabled(true)}>批量停用</EmsButton>}
          {isAdmin && <EmsButton variant="secondary" size="small" disabled={!selected.size || busy} onClick={() => batchDisabled(false)}>批量启用</EmsButton>}
          {canOperate && <EmsButton size="small" disabled={!writableSelected.length || busy} title={!writableSelected.length ? "先勾选表格左侧选择框，至少选一个可写点位" : undefined} onClick={openBatchWrite}>批量下发</EmsButton>}
          {isAdmin && <EmsButton variant="danger" size="small" disabled={!selected.size || busy} onClick={batchDelete}>批量删除</EmsButton>}
        </>}>
          <label className="!flex !items-center !gap-2 text-xs text-ems-muted"><input className="!h-4 !w-4 !min-h-0 !p-0 accent-ems-accent" type="checkbox" aria-label="全选当前筛选" checked={allVisible} onChange={toggleVisible} /> 当前筛选全选</label>
          <span className="text-xs text-ems-muted">筛选 {visible.length} / 全部 {rows.length}</span>
          {batchNotice && <span className="text-xs font-medium text-ems-mint">{batchNotice}</span>}
        </BatchActionBar>
      </div>

      <div className="min-w-0 [&_.ems-table]:min-w-[1180px]">
        <EmsRecordList rows={visible} keyOf={(row) => row.node.id} onRowClick={showDetail}
          empty={<EmptyState title={rows.length ? "没有匹配的点位" : "还没有 Modbus 点位"} description={rows.length ? "调整筛选条件后再试。" : "到通道视图接入设备，或在部署页预检并导入点表。"} />}
          columns={[
            { key: "select", label: "选择", className: "w-10 text-center", render: (row) => <input className="!h-4 !w-4 !min-h-0 !p-0 accent-ems-accent" type="checkbox" aria-label={`选择 ${row.name}`} checked={selected.has(row.node.id)} onClick={(event) => event.stopPropagation()} onChange={() => toggle(row.node.id)} /> },
            { key: "name", label: "点位名称", render: (row) => <div className="grid gap-1"><strong>{row.name}</strong><div className="flex flex-wrap gap-1">{row.readOnly && <StatusBadge dot={false}>只读</StatusBadge>}{row.disabled && <StatusBadge tone="warning" dot={false}>停用</StatusBadge>}</div></div> },
            { key: "role", label: "角色", render: (row) => <StatusBadge tone="info" dot={false}>{roleLabel(row.role)}</StatusBadge> },
            { key: "bus", label: "通道", render: (row) => <><span>{row.busName}</span><small className="block max-w-44 break-all text-ems-muted">{row.busAddress || "—"}</small></> },
            { key: "server", label: "站号", render: (row) => row.serverID },
            { key: "type", label: "类型", render: (row) => ioTypeNames[row.ioType] || row.ioType },
            { key: "address", label: "地址", render: (row) => row.address },
            { key: "format", label: "格式", render: (row) => row.format },
            { key: "scale", label: "缩放 / 单位", render: (row) => `${row.scale} / ${row.units || "—"}` },
            { key: "value", label: "当前值", render: (row) => <strong className={`whitespace-nowrap tabular-nums ${qualityValueColors[row.quality]}`}>{displayValue(row)}</strong> },
            { key: "quality", label: "质量", render: (row) => <><StatusBadge tone={row.disabled ? "neutral" : qualityTones[row.quality]}>{row.disabled ? "已停用" : qualityNames[row.quality]}</StatusBadge><small className="mt-1 block text-ems-muted">{qualityReason(row)}</small></> },
            { key: "time", label: "更新时间", render: (row) => <><span title={row.valueTime}>{timeLabel(row.valueTime)}</span><small className="block text-ems-muted">{ageLabel(row.valueTime)}</small></> },
            { key: "action", label: "操作", render: (row) => <EmsButton variant="quiet" size="small" onClick={(event) => { event.stopPropagation(); showDetail(row) }}>详情</EmsButton> },
          ]}
          mobileCard={(row) => <div className="grid min-w-0 gap-3">
            <div className="flex items-start justify-between gap-3">
              <div className="flex min-w-0 items-start gap-2">
                <input className="!mt-1 !h-4 !w-4 !min-h-0 !p-0 shrink-0 accent-ems-accent" type="checkbox" aria-label={`选择 ${row.name}`} checked={selected.has(row.node.id)} onClick={(event) => event.stopPropagation()} onChange={() => toggle(row.node.id)} />
                <div className="min-w-0"><strong className="break-words text-sm text-ems-ink">{row.name}</strong><div className="mt-1 flex flex-wrap gap-1">{row.readOnly && <StatusBadge dot={false}>只读</StatusBadge>}{row.disabled && <StatusBadge tone="warning" dot={false}>停用</StatusBadge>}</div></div>
              </div>
              <div className="shrink-0 text-right"><span className="block text-[10px] text-ems-muted">当前值</span><strong className={`text-lg tabular-nums ${qualityValueColors[row.quality]}`}>{displayValue(row)}</strong></div>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2"><StatusBadge tone={row.disabled ? "neutral" : qualityTones[row.quality]}>{row.disabled ? "已停用" : qualityNames[row.quality]}</StatusBadge><span className="text-xs text-ems-muted">{qualityReason(row)} · {ageLabel(row.valueTime)}</span></div>
            <div className="min-w-0 text-xs text-ems-muted"><span className="block">{row.busName} · 站号 {row.serverID} / 地址 {row.address}</span><span className="block break-all">{row.busAddress || "—"}</span></div>
            <div className="flex items-center justify-between gap-2"><StatusBadge tone="info" dot={false}>{roleLabel(row.role)}</StatusBadge><EmsButton variant="secondary" size="small" onClick={(event) => { event.stopPropagation(); showDetail(row) }}>详情</EmsButton></div>
          </div>}
        />
      </div>
    </IonCardContent></EmsCard>

    <EmsOverlay open={batchEdit} title="批量修改缩放 / 偏移" busy={busy} onClose={closeBatchEdit}
      actions={<><EmsButton variant="secondary" disabled={busy} onClick={closeBatchEdit}>取消</EmsButton><EmsButton busy={busy} disabled={!selectedRows.length} onClick={applyBatchScaleOffset}>应用到已选</EmsButton></>}>
      <div className="grid gap-4">
        <InlineNotice>已选 {selectedRows.length} 个点位；逐点保存，失败时已成功的点位保留新值。</InlineNotice>
        <div className="grid gap-3 sm:grid-cols-2">
          <EmsInput label="缩放" aria-label="批量缩放" type="number" step="any" value={batchScale} onIonInput={(event) => setBatchScale(event.detail.value || "")} />
          <EmsInput label="偏移" aria-label="批量偏移" type="number" step="any" value={batchOffset} onIonInput={(event) => setBatchOffset(event.detail.value || "")} />
        </div>
      </div>
    </EmsOverlay>

    <EmsOverlay open={batchWriteOpen && !pendingBatchWrite} title="批量下发目标值" busy={busy} onClose={closeBatchWrite}
      actions={<><EmsButton variant="secondary" disabled={busy} onClick={closeBatchWrite}>取消</EmsButton><EmsButton disabled={busy || !Number.isFinite(batchWriteValue)} onClick={() => setPendingBatchWrite(true)}>确认下发</EmsButton></>}>
      <div className="grid gap-4">
        <InlineNotice tone="warning">已选 {writableSelected.length} 个可写点位；同一目标值逐点下发并等待回读。</InlineNotice>
        <EmsInput label="目标值" aria-label="批量下发目标值" type="number" step="any" value={batchWriteValue} onIonInput={(event) => setBatchWriteValue(Number(event.detail.value))} />
      </div>
    </EmsOverlay>

    <EmsOverlay open={createOpen} title="批量新增点位" busy={busy} onClose={closeCreate}
      actions={<><EmsButton variant="secondary" disabled={busy} onClick={closeCreate}>取消</EmsButton><EmsButton busy={busy} busyLabel={createProgress || "创建中…"} onClick={() => void createPoints()}>{createPreview.length ? `创建 ${createPreview.length} 个点位` : "创建点位"}</EmsButton></>}>
      <div className="grid gap-4">
        <InlineNotice>按通道生成连续地址点位；先看预览，再创建。失败时会回滚本次已创建的点位。</InlineNotice>
        <div className="grid gap-3 sm:grid-cols-2">
          <EmsSelect label="通道" aria-label="新增通道" value={createChannel} onIonChange={(event) => setCreateChannel(event.detail.value)}>
            {buses.map((bus) => <EmsSelectOption key={bus.id} value={bus.id}>{text(bus, "description") || bus.id}</EmsSelectOption>)}
          </EmsSelect>
          <EmsInput label="名称前缀" value={createPrefix} onIonInput={(event) => setCreatePrefix(event.detail.value || "")} placeholder="例如：电芯温度-" />
          <EmsInput label="起始序号" type="number" min={0} value={createStartIndex} onIonInput={(event) => setCreateStartIndex(event.detail.value || "")} />
          <EmsInput label="站号" type="number" min={1} max={247} value={createServer} onIonInput={(event) => setCreateServer(event.detail.value || "")} />
          <EmsInput label="起始地址" type="number" min={0} value={createAddress} onIonInput={(event) => setCreateAddress(event.detail.value || "")} />
          <EmsInput label="数量" type="number" min={1} max={200} value={createCount} onIonInput={(event) => setCreateCount(event.detail.value || "")} />
          <EmsInput label="地址步长" type="number" min={1} value={createStep} onIonInput={(event) => setCreateStep(event.detail.value || "")} />
          <EmsSelect label="类型" value={createType} onIonChange={(event) => changeCreateType(event.detail.value)}>{ioTypeOptions.map((value) => <EmsSelectOption key={value} value={value}>{ioTypeNames[value]}</EmsSelectOption>)}</EmsSelect>
          <EmsSelect label="数据格式" value={createFormat} disabled={createType === "modbusCoil" || createType === "modbusDiscreteInput"} onIonChange={(event) => setCreateFormat(event.detail.value)}>{formatOptions.map((value) => <EmsSelectOption key={value} value={value}>{value}</EmsSelectOption>)}</EmsSelect>
          <EmsInput label="缩放" type="number" step="any" value={createScale} onIonInput={(event) => setCreateScale(event.detail.value || "")} />
          <EmsInput label="偏移" type="number" step="any" value={createOffset} onIonInput={(event) => setCreateOffset(event.detail.value || "")} />
          <EmsInput label="单位" value={createUnits} onIonInput={(event) => setCreateUnits(event.detail.value || "")} />
        </div>
        <div className="flex flex-wrap items-center gap-4 text-xs text-ems-muted">
          <label className="!flex !items-center !gap-2"><input className="!h-4 !w-4 !min-h-0 !p-0 accent-ems-accent" type="checkbox" checked={createReadOnly} disabled={createType === "modbusInputRegister" || createType === "modbusDiscreteInput"} onChange={(event) => setCreateReadOnly(event.target.checked)} /> 只读</label>
          <label className="!flex !items-center !gap-2"><input className="!h-4 !w-4 !min-h-0 !p-0 accent-ems-accent" type="checkbox" checked={createDisabled} onChange={(event) => setCreateDisabled(event.target.checked)} /> 停用</label>
          <span>当前通道 {createPointsInChannel} 个点位</span>
        </div>
        <div className="grid gap-2 rounded-xl border border-ems-line bg-white/60 p-3 text-xs">
          <strong className="text-ems-ink">创建预览</strong>
          {createPreview.length ? <span className="break-all text-ems-muted">{createPreview.slice(0, 3).map((point) => `${point.name} @ ${point.address}`).join("、")}{createPreview.length > 3 ? ` … ${createPreview[createPreview.length - 1].name} @ ${createPreview[createPreview.length - 1].address}` : ""}</span> : <span className="text-ems-muted">填写通道、前缀、起始地址和数量后显示预览。</span>}
        </div>
        {createAddressWarning && <InlineNotice tone="warning">{createAddressWarning}</InlineNotice>}
        {createError && <InlineNotice tone="danger">{createError}</InlineNotice>}
      </div>
    </EmsOverlay>

    <EmsOverlay open={Boolean(openRow) && !safetyOpen && !pendingWrite} variant="drawer" title={openRow ? `${openRow.name} 详情` : "点位详情"} onClose={closeDetail}
      actions={<EmsButton variant="secondary" onClick={closeDetail}>关闭</EmsButton>}>
      {openRow && <div className="grid gap-4">
        <div className="grid gap-2 rounded-xl border border-ems-line bg-white/65 p-4">
          <span className="text-xs text-ems-muted">当前值</span>
          <strong className={`text-3xl tabular-nums ${qualityValueColors[openRow.quality]}`}>{displayValue(openRow)}</strong>
          <div className="flex flex-wrap items-center gap-2"><StatusBadge tone={openRow.disabled ? "neutral" : qualityTones[openRow.quality]}>{openRow.disabled ? "已停用" : qualityNames[openRow.quality]}</StatusBadge><span className="text-xs text-ems-muted">{ageLabel(openRow.valueTime)} · {qualityReason(openRow)}</span></div>
        </div>
        <PointSparkline token={token} nodeId={openRow.node.id} />
        {openRow.command && <InlineNotice>最近命令：{({ accepted: "已接受", confirmed: "已回读", unknown: "结果未知", failed: "失败" } as Record<string, string>)[openRow.command] || openRow.command}</InlineNotice>}
        {notice && <InlineNotice tone="success">{notice}</InlineNotice>}

        {!openRow.readOnly && canOperate && <div className="grid gap-2 rounded-xl border border-ems-line bg-white/50 p-3">
          <strong className="text-sm">下发目标值</strong>
          <div className="flex items-end gap-2"><EmsInput label="目标值" aria-label="目标值" type="number" step="any" value={writeValue} onIonInput={(event) => setWriteValue(Number(event.detail.value))} /><EmsButton className="shrink-0" disabled={busy} onClick={writePoint}>下发</EmsButton></div>
          <small className="text-xs text-ems-muted">{openRow.valueSet !== undefined ? `当前目标 ${openRow.valueSet}${openRow.units ? ` ${openRow.units}` : ""}` : "该点位还没有目标值"}</small>
        </div>}

        <div className="flex flex-wrap gap-2">
          <EmsButton variant="secondary" size="small" onClick={() => openHistory(openRow.node.id)}>查看历史</EmsButton>
          {canOperate && <EmsButton variant="secondary" size="small" disabled={busy} onClick={toggleManual}>{openRow.manual ? "恢复自动" : "接管自动"}</EmsButton>}
          {isAdmin && !openRow.readOnly && <EmsButton variant="secondary" size="small" onClick={() => setSafetyOpen(true)}>安全约束{point(openRow.node, "easyemsSafety")?.text ? "" : " · 未配置"}</EmsButton>}
          {isAdmin && !editing && <EmsButton variant="secondary" size="small" onClick={startEdit}>编辑点位</EmsButton>}
        </div>

        {editing && draft ? <div className="grid gap-3 border-t border-ems-line pt-4">
          <EmsInput label="名称" value={draft.name} onIonInput={(event) => setDraft({ ...draft, name: event.detail.value || "" })} />
          <div className="grid gap-3 sm:grid-cols-2">
            <EmsInput label="站号" type="number" min={1} max={247} value={draft.serverID} onIonInput={(event) => setDraft({ ...draft, serverID: Number(event.detail.value) })} />
            <EmsInput label="地址" type="number" min={0} value={draft.address} onIonInput={(event) => setDraft({ ...draft, address: Number(event.detail.value) })} />
            <EmsSelect label="类型" value={draft.ioType} onIonChange={(event) => setDraft({ ...draft, ioType: event.detail.value })}>{ioTypeOptions.map((value) => <EmsSelectOption key={value} value={value}>{ioTypeNames[value]}</EmsSelectOption>)}</EmsSelect>
            <EmsSelect label="数据格式" value={draft.format} onIonChange={(event) => setDraft({ ...draft, format: event.detail.value })}>{formatOptions.map((value) => <EmsSelectOption key={value} value={value}>{value}</EmsSelectOption>)}</EmsSelect>
            <EmsInput label="缩放" type="number" step="any" value={draft.scale} onIonInput={(event) => setDraft({ ...draft, scale: Number(event.detail.value) })} />
            <EmsInput label="偏移" type="number" step="any" value={draft.offset} onIonInput={(event) => setDraft({ ...draft, offset: Number(event.detail.value) })} />
          </div>
          <EmsInput label="单位" value={draft.units} onIonInput={(event) => setDraft({ ...draft, units: event.detail.value || "" })} />
          <div className="flex flex-wrap gap-4 text-xs text-ems-muted">
            <label className="!flex !items-center !gap-2"><input className="!h-4 !w-4 !min-h-0 !p-0 accent-ems-accent" type="checkbox" checked={draft.readOnly} onChange={(event) => setDraft({ ...draft, readOnly: event.target.checked })} /> 只读</label>
            <label className="!flex !items-center !gap-2"><input className="!h-4 !w-4 !min-h-0 !p-0 accent-ems-accent" type="checkbox" checked={draft.disabled} onChange={(event) => setDraft({ ...draft, disabled: event.target.checked })} /> 停用</label>
          </div>
          <div className="flex justify-end gap-2"><EmsButton variant="secondary" onClick={() => { setEditing(false); setDraft(null) }}>取消</EmsButton><EmsButton busy={busy} onClick={saveEdit}>保存点位</EmsButton></div>
        </div> : <dl className="grid gap-3">
          {[
            ["通道", `${openRow.busName} · ${openRow.busAddress}`],
            ["站号 / 地址", `${openRow.serverID} / ${openRow.address}`],
            ["类型 / 格式", `${ioTypeNames[openRow.ioType] || openRow.ioType} · ${openRow.format}`],
            ["缩放 / 偏移 / 单位", `${openRow.scale} / ${openRow.offset} / ${openRow.units || "—"}`],
            ["读写", `${openRow.readOnly ? "只读" : "可写"} · ${openRow.disabled ? "已停用" : "启用"}`],
          ].map(([label, value]) => <div className="grid grid-cols-[96px_minmax(0,1fr)] gap-3 text-sm sm:grid-cols-[130px_minmax(0,1fr)]" key={label}><dt className="text-ems-muted">{label}</dt><dd className="m-0 break-all text-ems-ink">{value}</dd></div>)}
          <div className="grid grid-cols-[96px_minmax(0,1fr)] gap-3 text-sm sm:grid-cols-[130px_minmax(0,1fr)]"><dt className="text-ems-muted">业务 ID</dt><dd className="m-0 break-all font-mono text-xs text-ems-muted">{text(openRow.node, "businessId") || openRow.node.id}</dd></div>
        </dl>}
      </div>}
    </EmsOverlay>
    {safetyOpen && openRow && <SafetyModal token={token} node={openRow.node} nodes={nodes} onClose={() => setSafetyOpen(false)} onSaved={() => { setSafetyOpen(false); setNotice("安全约束已保存") }} />}

    <EmsOverlay open={pendingWrite && Boolean(openRow)} title={openRow ? `确认下发「${openRow.name}」` : "确认下发"} busy={busy} onClose={cancelPendingWrite}
      actions={<><EmsButton variant="secondary" disabled={busy} onClick={cancelPendingWrite}>取消</EmsButton><EmsButton busy={busy} busyLabel="执行中…" onClick={confirmWrite}>确认下发</EmsButton></>}>
      {openRow && <div className="grid gap-4">
        <InlineNotice tone="warning">目标值会直接写入设备，等待回读确认；失败不会自动重试。</InlineNotice>
        <dl className="grid gap-3 text-sm">
          <div className="flex items-center justify-between gap-3"><dt className="text-ems-muted">当前值</dt><dd className="m-0 font-medium">{displayValue(openRow)}</dd></div>
          <div className="flex items-center justify-between gap-3"><dt className="text-ems-muted">目标值</dt><dd className="m-0 font-semibold text-ems-accent">{writeValue}{openRow.units ? ` ${openRow.units}` : ""}</dd></div>
        </dl>
      </div>}
    </EmsOverlay>

    <EmsOverlay open={pendingBatchWrite} title={`确认批量下发（${writableSelected.length} 个点位）`} busy={busy} onClose={cancelPendingBatchWrite}
      actions={<><EmsButton variant="secondary" disabled={busy} onClick={cancelPendingBatchWrite}>取消</EmsButton><EmsButton busy={busy} busyLabel="执行中…" onClick={doBatchWrite}>确认下发</EmsButton></>}>
      <div className="grid gap-4">
        <InlineNotice tone="warning">同一目标值会直接写入所选点位，逐点下发并等待回读；失败不会自动重试。</InlineNotice>
        <div className="grid gap-2">{writableSelected.map((row) => <div className="flex items-center justify-between gap-3 rounded-lg border border-ems-line bg-white/55 px-3 py-2 text-xs" key={row.node.id}><strong className="break-words">{row.name}</strong><span className="shrink-0 tabular-nums text-ems-muted">{row.value === undefined ? "—" : row.value} → <b className="text-ems-accent">{batchWriteValue}{row.units ? ` ${row.units}` : ""}</b></span></div>)}</div>
      </div>
    </EmsOverlay>
  </section>
}
