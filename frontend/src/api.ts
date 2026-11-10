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
  user: { user_id: string; policy: { admin: boolean; role: string; actions?: string[] } }
  roots: string[]
  nodes: Node[]
  session_timeout_seconds?: number
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
    const result = await response.json().catch(() => ({ error: "" }))
    const friendly = response.status === 403 ? "账号或密码不正确"
      : response.status === 409 ? "该账号同时登录的会话数已达上限，请稍后再试或退出其他会话"
      : response.status === 503 ? "认证服务暂不可用，请稍后重试"
      : ""
    throw new ApiError(friendly || "登录失败，请稍后重试", response.status)
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
    const result = await response.json().catch(() => ({ error: "" }))
    throw new ApiError(errorMessage(result.error, response.status), response.status)
  }
  return (await response.json()) as T
}

// 服务端错误码 → 面向值班员的业务文案；未知码按状态给出可操作提示。
const errorTexts: Record<string, string> = {
  admin_required: "需要管理员权限",
  forbidden: "当前账号无权执行此操作",
  not_found: "对象不存在或已删除",
  requires_energy_unit: "只能对能源设备执行此操作",
  requires_plant: "只能对电站执行此操作",
  energy_unit_not_found: "能源设备不存在",
  plant_not_found: "电站不存在",
  no_rule: "还没有规则，请先配置并保存草稿",
  rule_id_required: "规则缺少标识",
  invalid_rule: "规则内容不合法",
  invalid_members: "成员配置不合法",
  description_required: "请填写名称",
  rated_power_required: "请填写大于 0 的额定功率",
  meter_point_required: "请选择 PCC 主表点位",
  save_failed: "保存失败，请稍后重试",
  create_failed: "创建失败，请稍后重试",
  list_failed: "读取失败，请稍后刷新",
  invalid_login: "账号或密码不正确",
  authentication_unavailable: "认证服务暂不可用，请稍后重试",
  session_limit_reached: "该账号同时登录的会话数已达上限",
  capability_load_failed: "读取设备能力失败",
  plant_resolve_failed: "无法确定所属电站，请先创建电站",
  channel_busy: "该地址正被运行中的通道占用",
}

function errorMessage(code: string, status: number): string {
  if (code && errorTexts[code]) return errorTexts[code]
  if (status === 401) return "登录已失效，请重新登录"
  if (status === 403) return "当前账号无权执行此操作"
  if (status === 404) return "对象不存在或已删除"
  if (status >= 500) return "服务暂时不可用，请稍后重试"
  return "操作未完成，请稍后重试"
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
  root_cause?: string
  group_id?: string
  count?: number
  last_raised_at?: string
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

export type EmsStateValue = {
  device_id: string
  point_id: string
  value: { kind: string; number?: number; bool?: boolean; enum?: string }
  quality: string
  source_time: string
  received_time: string
}

export type EmsHistorySample = {
  value: { kind: string; number?: number; bool?: boolean; enum?: string }
  quality: string
  source_time: string
}

export async function fetchEmsState(token: string) {
  return request<{ values: EmsStateValue[] }>(token, "/ems/state")
}

export async function fetchEmsHistory(token: string, deviceId: string, pointId: string, start: Date, end: Date) {
  const query = new URLSearchParams({
    device_id: deviceId,
    point_id: pointId,
    start: start.toISOString(),
    end: end.toISOString(),
    limit: "5000",
  })
  return request<{ device_id: string; point_id: string; samples: EmsHistorySample[] }>(token, `/ems/history?${query.toString()}`)
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

export type MemberConstraint = {
  kind: string
  target: Target
  operator: string
  limit: number
  direction?: "charge" | "discharge" | "both"
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
  priority?: number
  weight?: number
  constraints?: MemberConstraint[]
}

export type AllocatorConfig = {
  grace_ms?: number
  timeout_ms?: number
  deadband_kw?: number
  members: AllocatorMember[]
}

export async function fetchAllocator(token: string, nodeId: string) {
  return request<AllocatorConfig>(token, `/ems/allocators/${nodeId}`)
}

export async function saveAllocator(token: string, nodeId: string, config: AllocatorConfig) {
  return request<{ saved: boolean }>(token, `/ems/allocators/${nodeId}`, "PUT", config)
}

// 能力级调度（领域模型重构）：规则条件引用 EnergyUnit 能力，动作是功率请求。
export type CapabilityCondition = {
  ref: { energy_unit_id: string; field: string }
  operator: string
  value: number
}

export type CapabilityAction = {
  energy_unit_id: string
  request_kw: number
}

export type CapabilityRule = {
  id: string
  state?: string
  published_at?: string
  conditions: CapabilityCondition[]
  actions: CapabilityAction[]
}

export type EnergyMember = {
  energy_unit_id: string
  min_power?: number
  priority?: number
  weight?: number
}

export async function fetchEnergyRule(token: string, energyUnitId: string) {
  return request<CapabilityRule>(token, `/energy-units/${energyUnitId}/rule`)
}

export type EnergyUnitSummary = { id: string; asset_code: string; description: string }

export async function fetchEnergyUnits(token: string) {
  return request<{ units: EnergyUnitSummary[] }>(token, `/energy-units`)
}

export async function createEnergyUnit(token: string, input: { asset_code?: string; description: string; rated_power_kw: number; energy_type?: string }) {
  return request<{ id: string; asset_code: string; plant_id: string }>(token, `/energy-units`, "POST", input)
}

export type EnergyDevice = { id: string; asset_code: string; description: string; rated_power_kw: number; power_setpoint_id: string }

export async function fetchEnergyDevices(token: string, energyUnitId: string) {
  return request<{ devices: EnergyDevice[] }>(token, `/energy-units/${energyUnitId}/devices`)
}

export async function createEnergyDevice(token: string, energyUnitId: string, input: { asset_code?: string; description: string; rated_power_kw: number; power_setpoint_id: string }) {
  return request<{ id: string; asset_code: string }>(token, `/energy-units/${energyUnitId}/devices`, "POST", input)
}

// 电站策略（防逆流/需量）：PCC 反馈绑定。
export type PlantSummary = { id: string; asset_code: string; description: string }
export type PCCBinding = { MeterPointID: string; Sign: number; MaxAgeMS: number; UpperLimitKW: number; LowerLimitKW: number }

export async function fetchPlants(token: string) {
  return request<{ plants: PlantSummary[] }>(token, `/plants`)
}

export async function fetchPlantPCC(token: string, plantId: string) {
  return request<{ binding: PCCBinding; configured: boolean }>(token, `/plants/${plantId}/pcc`)
}

export async function savePlantPCC(token: string, plantId: string, binding: PCCBinding) {
  return request<{ saved: boolean }>(token, `/plants/${plantId}/pcc`, "PUT", binding)
}

export type TariffPeriod = { StartMin: number; EndMin: number; Kind: string; Price: number }
export type PlantStrategy = { peak_valley_power_kw: number; periods: TariffPeriod[] }

export async function fetchPlantStrategy(token: string, plantId: string) {
  return request<{ strategy: PlantStrategy; configured: boolean }>(token, `/plants/${plantId}/strategy`)
}

export async function savePlantStrategy(token: string, plantId: string, strategy: PlantStrategy) {
  return request<{ saved: boolean }>(token, `/plants/${plantId}/strategy`, "PUT", strategy)
}

export async function saveEnergyRule(token: string, energyUnitId: string, rule: CapabilityRule) {
  return request<{ saved: boolean }>(token, `/energy-units/${energyUnitId}/rule`, "PUT", rule)
}

export async function publishEnergyRule(token: string, energyUnitId: string) {
  return request<{ saved: boolean }>(token, `/energy-units/${energyUnitId}/rule/publish`, "POST", {})
}

export async function simulateEnergyRule(token: string, energyUnitId: string, rule: CapabilityRule, values: Record<string, Record<string, number>>) {
  return request<{ actions: CapabilityAction[] }>(token, `/energy-units/${energyUnitId}/rule/simulate`, "POST", { rule, values })
}

export async function fetchEnergyAllocator(token: string, energyUnitId: string) {
  return request<{ members: EnergyMember[] }>(token, `/energy-units/${energyUnitId}/allocator`)
}

export async function saveEnergyAllocator(token: string, energyUnitId: string, members: EnergyMember[]) {
  return request<{ saved: boolean }>(token, `/energy-units/${energyUnitId}/allocator`, "PUT", { members })
}

export type AllocShare = {
  member_id: string
  capacity: number
  setpoint: number
  actual?: number
  confirmed?: boolean
  excluded: boolean
  held: boolean
  reason: string
}

export type AllocationSnapshot = {
  at: string
  requested: number
  delivered: number
  shortfall: number
  status: string
  reason?: string
  shares: AllocShare[]
}

export async function simulateAllocator(token: string, nodeId: string, requestValue: number) {
  return request<{ requested: number; delivered: number; shortfall: number; shares: AllocShare[]; notes?: string[] }>(
    token, `/ems/allocators/${nodeId}/simulate`, "POST", { request: requestValue })
}

export async function fetchAllocatorHistory(token: string, nodeId: string) {
  return request<{ history: AllocationSnapshot[] }>(token, `/ems/allocators/${nodeId}/history`)
}

export type RowDiff = {
  row_id: string
  kind: "added" | "removed" | "changed"
  from?: string
  to?: string
  detail?: string[]
}

export type DependencyNode = { id: string; kind: "input" | "output" | "row"; label: string }
export type DependencyEdge = { from: string; to: string; kind: "condition" | "action" }
export type DependencyGraph = { nodes: DependencyNode[]; edges: DependencyEdge[] }

export async function diffRule(token: string, ruleId: string, from: string, to: string) {
  return request<{ diffs: RowDiff[] }>(token, `/rules/${ruleId}/diff`, "POST", { from, to })
}

export async function replayRule(token: string, ruleId: string, versionId: string, inputs: Record<string, number>, textInputs: Record<string, string>, at?: string) {
  return request<Evaluation>(token, `/rules/${ruleId}/replay`, "POST", {
    version_id: versionId, inputs, text_inputs: textInputs, at: at || undefined,
  })
}

export async function fetchRuleDeps(token: string, ruleId: string, document: Document) {
  return request<DependencyGraph>(token, `/rules/${ruleId}/deps`, "POST", { document })
}

export const permissionActions: { key: string; label: string }[] = [
  { key: "configure", label: "配置（通道/设备/点位/规则/画面）" },
  { key: "write_value", label: "写值（控制写入）" },
  { key: "command", label: "命令执行" },
  { key: "rule_publish", label: "规则发布" },
  { key: "alarm_ack", label: "报警确认" },
  { key: "alarm_maintain", label: "报警维护" },
  { key: "delete", label: "删除" },
  { key: "export", label: "导出" },
  { key: "account_manage", label: "账号管理" },
  { key: "audit_view", label: "审计查看" },
  { key: "site_settings", label: "站点设置" },
]

export type UserAccount = {
  id: string
  email: string
  policy: { admin: boolean; role: string; root_node_ids?: string[]; actions?: string[] }
}

export async function listUsers(token: string) {
  return request<{ users: UserAccount[]; actions: string[] }>(token, "/users")
}

export async function updateUserPolicy(token: string, id: string, role: string, actions: string[]) {
  return request<{ saved: boolean }>(token, `/users/${encodeURIComponent(id)}`, "PUT", { role, actions })
}

export async function resetUserPassword(token: string, id: string, password: string) {
  return request<{ reset: boolean }>(token, `/users/${encodeURIComponent(id)}/password`, "POST", { password })
}
