export type Point = {
  type: string
  key: string
  time: string
  value?: number
  text?: string
  tombstone?: number
  origin?: string
}

export type Node = {
  id: string
  type: string
  parent: string
  points: Point[]
}

export type Condition = {
  point_id: string
  operator: string
  value: number
  kind?: "number" | "bool" | "enum" | "time"
  text?: string
  start?: string
  end?: string
  weekday_mask?: number
}
export type Action = { action_id: string; value: number }
export type Row = { row_id: string; conditions: Condition[]; actions: Action[]; hold_ms?: number; cooldown_ms?: number }
export type Target = { node_id: string; point_type?: string; key?: string }
export type Scenario = {
  scenario_id: string
  name?: string
  inputs?: Record<string, number>
  text_inputs?: Record<string, string>
  at?: string
  expect_row?: string
  expect_actions?: Action[]
}
export type ScenarioResult = {
  scenario_id: string
  name?: string
  passed: boolean
  row_id?: string
  actions?: { action_id: string; value: number; blocked?: boolean; reason?: string }[]
  reason?: string
  detail?: string
}
export type Document = {
  table: { rows: Row[] }
  bindings: { points: Record<string, Target>; actions: Record<string, Target> }
  scenarios?: Scenario[]
}
export type Snapshot = {
  user: { user_id: string; policy: { admin: boolean; role: string } }
  roots: string[]
  nodes: Node[]
}
export type TimeCheck = { start?: string; end?: string; weekday_mask?: number; satisfied: boolean }
export type Evaluation = { row_id?: string; actions?: Action[]; time_checks?: TimeCheck[] }

export class ApiError extends Error {
  constructor(message: string, public status: number) {
    super(message)
  }
}

export async function login(email: string, password: string) {
  const response = await fetch("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  })
  if (!response.ok) {
    const result = await response.json().catch(() => ({ error: "无法连接应用" }))
    throw new ApiError(response.status === 403 ? "账号或密码不正确" : result.error || "无法连接应用", response.status)
  }
  return (await response.json()) as { token: string }
}

export async function request<T>(token: string, path: string, method = "GET", body?: unknown): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) {
    const result = await response.json().catch(() => ({ error: `请求失败 (${response.status})` }))
    throw new ApiError(result.error || `请求失败 (${response.status})`, response.status)
  }
  return (await response.json()) as T
}

export async function importCSV(token: string, path: string, csv: string) {
  const response = await fetch(`/api${path}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "text/csv" },
    body: csv,
  })
  if (!response.ok) {
    const result = await response.json().catch(() => ({ error: "CSV 导入失败" }))
    throw new ApiError(result.error || "CSV 导入失败", response.status)
  }
  return (await response.json()) as { imported?: number; preview?: boolean; count?: number }
}

export async function downloadCSV(token: string, path: string, filename = "easyems-modbus-points.csv") {
  const response = await fetch(`/api${path}`, { headers: { Authorization: `Bearer ${token}` } })
  if (!response.ok) throw new ApiError("CSV 导出失败", response.status)
  const url = URL.createObjectURL(await response.blob())
  const link = document.createElement("a")
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

export function point(node: Node, type: string) {
  return node.points.find((item) => item.type === type && (item.tombstone || 0) % 2 === 0)
}

export function nameOf(node: Node) {
  return point(node, "description")?.text || node.id
}

export function businessId(node: Node) {
  return point(node, "businessId")?.text || node.id
}

export type EmsAlarm = {
  id: string
  severity: "info" | "warning" | "critical"
  title: string
  detail: string
  device_id?: string
  point_id?: string
  active: boolean
  state: "active" | "acknowledged" | "resolved" | "closed"
  created_at: string
  updated_at: string
  acked_by?: string
  ack_note?: string
  maintenance_until?: string
  maintenance_reason?: string
  maintained_by?: string
}

export async function fetchAlarms(token: string) {
  return request<{ alarms: EmsAlarm[] }>(token, "/ems/alarms")
}

export async function ackAlarm(token: string, id: string, note = "") {
  return request<EmsAlarm>(token, `/ems/alarms/${encodeURIComponent(id)}/ack`, "POST", { note })
}

export async function closeAlarm(token: string, id: string) {
  return request<EmsAlarm>(token, `/ems/alarms/${encodeURIComponent(id)}/close`, "POST", {})
}

export async function maintainAlarm(token: string, id: string, until: string, reason: string) {
  return request<EmsAlarm>(token, `/ems/alarms/${encodeURIComponent(id)}/maintenance`, "POST", { until, reason })
}

export async function releaseAlarm(token: string, id: string) {
  return request<EmsAlarm>(token, `/ems/alarms/${encodeURIComponent(id)}/maintenance`, "DELETE")
}

export type HistoryResponse = { node_id: string; type: string; points: Point[] }

export async function fetchHistory(token: string, nodeId: string, start: Date, end: Date) {
  const query = new URLSearchParams({
    node_id: nodeId,
    type: "value",
    start: start.toISOString(),
    end: end.toISOString(),
    limit: "2000",
  })
  return request<HistoryResponse>(token, `/history?${query.toString()}`)
}

export async function downloadHistory(token: string, nodeId: string, start: Date, end: Date) {
  const query = new URLSearchParams({
    node_id: nodeId,
    type: "value",
    start: start.toISOString(),
    end: end.toISOString(),
  })
  const response = await fetch(`/api/history/export?${query.toString()}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!response.ok) throw new ApiError("历史导出失败", response.status)
  const url = URL.createObjectURL(await response.blob())
  const link = document.createElement("a")
  link.href = url
  link.download = "easyems-history.csv"
  link.click()
  URL.revokeObjectURL(url)
}

export type ScreenConfig = {
  title: string
  columns: number
  point_ids: string[]
}

export async function fetchScreen(token: string, nodeId: string) {
  return request<ScreenConfig>(token, `/ems/screen/${nodeId}`)
}

export async function saveScreen(token: string, nodeId: string, config: ScreenConfig) {
  return request<{ saved: boolean }>(token, `/ems/screen/${nodeId}`, "PUT", config)
}

export type AllocatorMember = {
  id: string
  target: Target
  min_power?: number
  max_charge: number
  max_discharge: number
  available_charge?: Target
  available_discharge?: Target
  readback?: Target
  fault?: Target
}

export type AllocatorConfig = {
  grace_ms?: number
  timeout_ms?: number
  members: AllocatorMember[]
}

export async function fetchAllocator(token: string, nodeId: string) {
  return request<AllocatorConfig>(token, `/ems/allocators/${nodeId}`)
}

export async function saveAllocator(token: string, nodeId: string, config: AllocatorConfig) {
  return request<{ saved: boolean }>(token, `/ems/allocators/${nodeId}`, "PUT", config)
}
