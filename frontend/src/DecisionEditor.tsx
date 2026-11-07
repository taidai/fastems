import { useEffect, useMemo, useRef, useState } from "react"
import { IonToggle } from "@ionic/react"
import { writable } from "./writable"
import {
  Action,
  businessId,
  Condition,
  DependencyGraph,
  diffRule,
  Document,
  Evaluation,
  fetchRuleDeps,
  nameOf,
  Node,
  point,
  replayRule,
  request,
  Row,
  RowDiff,
  Scenario,
  ScenarioResult,
  TimeCheck,
} from "./api"
import SafetyModal from "./SafetyModal"
import ConfirmDialog from "./ConfirmDialog"

type Props = {
  token: string
  rule: Node
  nodes: Node[]
  onRefresh: () => Promise<void>
  onError: (message: string) => void
  onSelect: (id: string) => void
  readOnly: boolean
}

type RuleVersion = { id: string; published_at: string; document: Document }

type StrategyTemplate = {
  name: string
  description: string
  rows: Omit<Row, "row_id">[]
}

// Strategy templates give a one-click starting point for common storage
// policies. Their point/action ids are placeholders the operator rebinds to the
// station's real points through the binding panel.
const STRATEGY_TEMPLATES: StrategyTemplate[] = [
  {
    name: "峰谷套利",
    description: "峰段放电、谷段充电，默认待机",
    rows: [
      { conditions: [{ point_id: "", operator: "", value: 0, kind: "time", start: "08:00", end: "22:00", weekday_mask: 0 }], actions: [{ action_id: "ess_power", value: 100 }] },
      { conditions: [{ point_id: "", operator: "", value: 0, kind: "time", start: "22:00", end: "08:00", weekday_mask: 0 }], actions: [{ action_id: "ess_power", value: -100 }] },
      { conditions: [], actions: [{ action_id: "ess_power", value: 0 }] },
    ],
  },
  {
    name: "SOC 保护",
    description: "SOC 过低停放、过高停充",
    rows: [
      { conditions: [{ point_id: "soc", operator: "lt", value: 20 }], actions: [{ action_id: "ess_power", value: 0 }] },
      { conditions: [{ point_id: "soc", operator: "gt", value: 90 }], actions: [{ action_id: "ess_power", value: 0 }] },
      { conditions: [], actions: [{ action_id: "ess_power", value: 0 }] },
    ],
  },
  {
    name: "需量削峰",
    description: "电网需量超限时放电削峰",
    rows: [
      { conditions: [{ point_id: "grid_import", operator: "gt", value: 100 }], actions: [{ action_id: "ess_power", value: 50 }] },
      { conditions: [], actions: [{ action_id: "ess_power", value: 0 }] },
    ],
  },
]

const checkStateLabels: Record<string, string> = {
  fired: "已触发",
  "already-fired": "已执行",
  "waiting-hold": "持续确认中",
  cooldown: "冷却中",
  "no-match": "未命中",
}

type LastCheck = {
  at: string
  row: string
  state: string
  inputs: { input: string; value: number; text?: string; kind?: string; quality: string; source_time: string }[]
  actions: { output: string; requested: number; applied: number; blocked: boolean; reason: string; status?: string }[]
}

function parseLastCheck(rule: Node): LastCheck | null {
  const text = point(rule, "lastCheck")?.text
  if (!text) return null
  try {
    const parsed = JSON.parse(text)
    return {
      at: parsed.at || "",
      row: parsed.row || "",
      state: parsed.state || "",
      inputs: Array.isArray(parsed.inputs) ? parsed.inputs : [],
      actions: Array.isArray(parsed.actions) ? parsed.actions : [],
    }
  } catch {
    return null
  }
}

const operators = [
  ["gt", "大于"], ["ge", "大于或等于"], ["eq", "等于"],
  ["ne", "不等于"], ["le", "小于或等于"], ["lt", "小于"],
]
const switchOperators = [["eq", "等于"], ["ne", "不等于"]]

function operatorsFor(kind?: string) {
  return kind === "bool" || kind === "enum" ? switchOperators : operators
}

const operatorNames = new Map<string, string>([...operators, ...switchOperators] as [string, string][])
function operatorLabel(op: string) {
  return operatorNames.get(op) || op
}

function conditionValueText(condition: Condition) {
  if (condition.kind === "enum") return condition.text || "—"
  if (condition.kind === "bool") return condition.value ? "是" : "否"
  return String(condition.value)
}

// Weekday bits follow Go's time.Weekday: bit 0 = Sunday ... bit 6 = Saturday.
const weekdayOptions: [string, number][] = [
  ["一", 1], ["二", 2], ["三", 3], ["四", 4], ["五", 5], ["六", 6], ["日", 0],
]

function weekdayText(mask?: number) {
  if (!mask || mask === 127) return "每天"
  return weekdayOptions.filter(([, bit]) => mask & (1 << bit)).map(([label]) => `周${label}`).join("、")
}

function timeWindowText(start?: string, end?: string) {
  if (!start || !end) return "全天"
  return start > end ? `${start}–次日${end}` : `${start}–${end}`
}

function timeConditionText(condition: { start?: string; end?: string; weekday_mask?: number }) {
  return `${weekdayText(condition.weekday_mask)} ${timeWindowText(condition.start, condition.end)}`
}

function timeCheckText(check: TimeCheck) {
  return `时间条件 ${timeConditionText(check)}：${check.satisfied ? "满足" : "不满足"}`
}

// The engine reports a time condition as the synthetic input "time:start-end/mask".
function timeInputLabel(id: string) {
  const body = id.slice("time:".length)
  const slash = body.lastIndexOf("/")
  const [start, end] = body.slice(0, slash).split("-")
  return `时间条件 ${timeConditionText({ start, end, weekday_mask: Number(body.slice(slash + 1)) })}`
}

function NumberInput({ value, onChange, label }: { value: number; onChange: (value: number) => void; label: string }) {
  const [text, setText] = useState(String(value))
  const [focused, setFocused] = useState(false)
  useEffect(() => {
    if (!focused) setText(String(value))
  }, [value, focused])
  function change(raw: string) {
    setText(raw)
    const trimmed = raw.trim()
    if (trimmed === "" || trimmed === "-" || trimmed === "." || trimmed === "-." || trimmed.endsWith(".")) return
    const parsed = Number(trimmed)
    if (Number.isFinite(parsed)) onChange(parsed)
  }
  function blur() {
    setFocused(false)
    const parsed = Number(text)
    if (text.trim() === "" || !Number.isFinite(parsed)) {
      setText(String(value))
      return
    }
    setText(String(parsed))
    if (parsed !== value) onChange(parsed)
  }
  return <input
    aria-label={label}
    type="text"
    inputMode="decimal"
    value={text}
    onFocus={() => setFocused(true)}
    onChange={(event) => change(event.target.value)}
    onBlur={blur}
  />
}

function bindingsFor(nodes: Node[], existing?: Document["bindings"]): Document["bindings"] {
  const points: Document["bindings"]["points"] = { ...existing?.points }
  const actions: Document["bindings"]["actions"] = { ...existing?.actions }
  nodes.filter((node) => node.type === "variable" || node.type === "modbusIo").forEach((node) => {
    const id = businessId(node)
    points[id] ||= { node_id: node.id, point_type: "value" }
    actions[id] ||= { node_id: node.id, point_type: "valueSet" }
  })
  nodes.filter((node) => node.type === "group" && point(node, "easyemsAllocator")?.text).forEach((node) => {
    const id = businessId(node)
    actions[id] ||= { node_id: node.id, point_type: "valueSet" }
  })
  return { points, actions }
}

function initialDocument(rule: Node, nodes: Node[]): Document {
  let document: Document = { table: { rows: [] }, bindings: bindingsFor(nodes) }
  try {
    const draft = point(rule, "draft")?.text
    if (draft) document = JSON.parse(draft)
  } catch {
    // The server reports malformed persisted documents; the editor starts empty.
  }
  document.bindings = bindingsFor(nodes, document.bindings)
  document.table ||= { rows: [] }
  document.table.rows ||= []
  return document
}

export default function DecisionEditor({ token, rule, nodes, onRefresh, onError, onSelect, readOnly }: Props) {
  const available = useMemo(
    () => nodes.filter((node) => node.type === "variable" || node.type === "modbusIo"),
    [nodes],
  )
  const outputs = useMemo(() => available.filter(writable), [available])
  const allocatorOutputs = useMemo(
    () => nodes.filter((node) => node.type === "group" && point(node, "easyemsAllocator")?.text),
    [nodes],
  )
  const targetOutputs = useMemo(() => [...outputs, ...allocatorOutputs], [outputs, allocatorOutputs])
  const [document, setDocument] = useState<Document>(() => initialDocument(rule, nodes))
  const [busy, setBusy] = useState("")
  const [dirty, setDirty] = useState(false)
  const [result, setResult] = useState<Evaluation | null>(null)
  const [newName, setNewName] = useState(nameOf(rule))
  const [showRename, setShowRename] = useState(false)
  const [showVersions, setShowVersions] = useState(false)
  const [safetyNode, setSafetyNode] = useState<Node | null>(null)
  const [confirmAction, setConfirmAction] = useState<"publish" | "delete" | null>(null)
  const [viewMode, setViewMode] = useState<"table" | "cards">(() => localStorage.getItem("easyems-rule-view") === "table" ? "table" : "cards")
  const [inputs, setInputs] = useState<Record<string, number>>(() =>
    Object.fromEntries(available.map((node) => [businessId(node), point(node, "value")?.value || 0])),
  )
  const [textInputs, setTextInputs] = useState<Record<string, string>>(() =>
    Object.fromEntries(available.map((node) => [businessId(node), point(node, "value")?.text || ""])),
  )
  const [simAt, setSimAt] = useState("")
  const [scenarioName, setScenarioName] = useState("")
  const [scenarioExpect, setScenarioExpect] = useState("")
  const [scenarioResults, setScenarioResults] = useState<Record<string, ScenarioResult>>({})
  const [showStrategy, setShowStrategy] = useState(false)
  const [templateName, setTemplateName] = useState("")
  const [diffFrom, setDiffFrom] = useState("applied")
  const [diffTo, setDiffTo] = useState("draft")
  const [diffResult, setDiffResult] = useState<RowDiff[] | null>(null)
  const [replayVersion, setReplayVersion] = useState("applied")
  const [replayResult, setReplayResult] = useState<Evaluation | null>(null)
  const [deps, setDeps] = useState<DependencyGraph | null>(null)
  const rowsEnd = useRef<HTMLDivElement>(null)
  const simulationRef = useRef<HTMLDivElement>(null)
  const resultRef = useRef<HTMLDivElement>(null)
  const rows = document.table.rows
  const enabled = (point(rule, "enabled")?.value || 0) !== 0
  const published = Boolean(point(rule, "applied")?.text)
  const runtimeError = point(rule, "error")?.text
  const lastMatch = point(rule, "lastMatch")
  const lastMatchIndex = rows.findIndex((row) => row.row_id === lastMatch?.text)
  const lastMatchLabel = lastMatchIndex >= 0 ? `第 ${lastMatchIndex + 1} 行` : lastMatch?.text || "无"
  const versions = rule.points.filter((item) => item.type === "history" && (item.tombstone || 0) % 2 === 0).map((item) => {
    try { return JSON.parse(item.text || "") as RuleVersion } catch { return null }
  }).filter((item): item is RuleVersion => Boolean(item)).sort((a, b) => b.published_at.localeCompare(a.published_at))
  let appliedRows = 0
  let versionDifferent = false
  try {
    const applied = JSON.parse(point(rule, "applied")?.text || "") as Document
    appliedRows = applied.table.rows.length
    versionDifferent = JSON.stringify({ table: applied.table, bindings: applied.bindings }) !== JSON.stringify({ table: document.table, bindings: document.bindings })
  } catch { /* An unpublished rule has no comparison target. */ }

  function setRows(next: Row[]) {
    setDocument({ ...document, table: { rows: next } })
    setDirty(true)
    setResult(null)
    setScenarioResults({})
  }
  function updateRow(index: number, row: Row) {
    setRows(rows.map((current, i) => (i === index ? row : current)))
  }
  function move(index: number, direction: -1 | 1) {
    const target = index + direction
    if (target < 0 || target >= rows.length) return
    const next = [...rows]
    ;[next[index], next[target]] = [next[target], next[index]]
    setRows(next)
  }
  function addRow(useDefault = false) {
    const first = available[0]
    setRows([...rows, {
      row_id: crypto.randomUUID(),
      conditions: !useDefault && first ? [{ point_id: businessId(first), operator: "gt", value: 0 }] : [],
      actions: [],
    }])
    window.setTimeout(() => rowsEnd.current?.scrollIntoView({ block: "nearest" }), 0)
  }
  function duplicateRow(index: number) {
    const source = rows[index]
    if (!source) return
    const copy: Row = {
      ...source,
      row_id: crypto.randomUUID(),
      conditions: source.conditions.map((condition) => ({ ...condition })),
      actions: source.actions.map((action) => ({ ...action })),
    }
    const next = [...rows]
    next.splice(index + 1, 0, copy)
    setRows(next)
  }
  function updateCondition(rowIndex: number, conditionIndex: number, patch: Partial<Condition>) {
    const row = rows[rowIndex]
    updateRow(rowIndex, {
      ...row,
      conditions: row.conditions.map((condition, i) => i === conditionIndex ? { ...condition, ...patch } : condition),
    })
  }
  function updateAction(rowIndex: number, actionIndex: number, patch: Partial<Action>) {
    const row = rows[rowIndex]
    updateRow(rowIndex, {
      ...row,
      actions: row.actions.map((action, i) => i === actionIndex ? { ...action, ...patch } : action),
    })
  }
  function displayName(id: string) {
    const node = available.find((item) => businessId(item) === id)
    return node ? nameOf(node) : id
  }
  function rebind(kind: "points" | "actions", id: string, nodeID: string) {
    const targets = { ...document.bindings[kind] }
    targets[id] = { node_id: nodeID, point_type: kind === "points" ? "value" : "valueSet" }
    setDocument({ ...document, bindings: { ...document.bindings, [kind]: targets } })
    setDirty(true)
    setResult(null)
  }
  useEffect(() => {
    if (result) resultRef.current?.scrollIntoView({ block: "nearest" })
  }, [result])
  function locate(rowID?: string) {
    if (!rowID) return
    window.document.getElementById(`rule-row-${rowID}`)?.scrollIntoView({ block: "center" })
  }
  function useLiveInputs() {
    setInputs(Object.fromEntries(available.map((node) => [businessId(node), point(node, "value")?.value || 0])))
    setTextInputs(Object.fromEntries(available.map((node) => [businessId(node), point(node, "value")?.text || ""])))
  }
  async function run(kind: "save" | "publish" | "test" | "enable", enabledValue = false) {
    setBusy(kind)
    onError("")
    const current = { ...document, bindings: bindingsFor(available, document.bindings) }
    try {
      if (kind === "save" || kind === "publish") {
        await request(token, `/rules/${rule.id}/${kind === "save" ? "draft" : "publish"}`, kind === "save" ? "PUT" : "POST", current)
        setDocument(current)
        setDirty(false)
      } else if (kind === "test") {
        setResult(await request<Evaluation>(token, `/rules/${rule.id}/test`, "POST", {
          document: current, inputs,
          text_inputs: Object.fromEntries(Object.entries(textInputs).filter(([, text]) => text !== "")),
          at: simAt || undefined,
        }))
      } else {
        await request(token, `/rules/${rule.id}/enable`, "POST", { enabled: enabledValue })
      }
      await onRefresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "操作失败")
    } finally {
      setBusy("")
    }
  }

  async function manage(kind: "rename" | "copy" | "delete" | "restore", version?: RuleVersion) {
    setBusy(kind)
    onError("")
    try {
      if (kind === "rename") {
        await request(token, `/nodes/${rule.id}`, "PATCH", { description: newName.trim() })
        setShowRename(false)
      } else if (kind === "copy") {
        const result = await request<{ id: string }>(token, `/rules/${rule.id}/copy`, "POST", { description: `${nameOf(rule)} 副本` })
        await onRefresh()
        onSelect(result.id)
        return
      } else if (kind === "delete") {
        await request(token, `/nodes/${rule.id}`, "DELETE", { parent: rule.parent })
        onSelect("")
      } else if (version) {
        await request(token, `/rules/${rule.id}/restore`, "POST", { version_id: version.id })
        setDocument(version.document)
        setDirty(false)
      }
      await onRefresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "操作失败")
    } finally {
      setBusy("")
    }
  }

  const referencedInputs = [...new Set(rows.flatMap((row) => row.conditions.filter((condition) => condition.kind !== "time").map((condition) => condition.point_id)))]
  const referencedActions = [...new Set(rows.flatMap((row) => row.actions.map((action) => action.action_id)))]
  const lastCheck = useMemo(() => parseLastCheck(rule), [rule])
  const scenarios = document.scenarios || []

  function rowLabel(rowID?: string) {
    if (!rowID) return "不命中任何行"
    const index = rows.findIndex((row) => row.row_id === rowID)
    return index >= 0 ? `第 ${index + 1} 行` : "已删除的行"
  }
  function setScenarios(next: Scenario[]) {
    setDocument({ ...document, scenarios: next })
    setDirty(true)
    setScenarioResults({})
  }
  function addScenario() {
    const row = rows.find((item) => item.row_id === scenarioExpect)
    const scenario: Scenario = {
      scenario_id: crypto.randomUUID(),
      name: scenarioName.trim() || `场景 ${scenarios.length + 1}`,
      inputs: Object.fromEntries(referencedInputs.map((id) => [id, inputs[id] ?? 0])),
      text_inputs: Object.fromEntries(referencedInputs.map((id) => [id, textInputs[id] || ""]).filter(([, text]) => text !== "")),
      at: simAt || undefined,
      expect_row: scenarioExpect || undefined,
      expect_actions: row && row.actions.length ? row.actions.map((action) => ({ ...action })) : undefined,
    }
    setScenarios([...scenarios, scenario])
    setScenarioName("")
  }
  function loadScenario(scenario: Scenario) {
    setInputs({ ...inputs, ...scenario.inputs })
    setTextInputs({ ...textInputs, ...scenario.text_inputs })
    setSimAt(scenario.at || "")
  }
  async function runScenarios() {
    setBusy("scenarios")
    onError("")
    try {
      const current = { ...document, bindings: bindingsFor(available, document.bindings) }
      const response = await request<{ results: ScenarioResult[] }>(token, `/rules/${rule.id}/scenarios`, "POST", { document: current })
      setScenarioResults(Object.fromEntries(response.results.map((item) => [item.scenario_id, item])))
    } catch (error) {
      onError(error instanceof Error ? error.message : "操作失败")
    } finally {
      setBusy("")
    }
  }
  function scenarioVerdictText(scenario: Scenario, result: ScenarioResult) {
    if (result.passed) return `通过 · 实际${rowLabel(result.row_id)}`
    if (result.reason === "row_mismatch") return `失败 · 期望${rowLabel(scenario.expect_row)}，实际${rowLabel(result.row_id)}`
    if (result.reason === "action_mismatch") return `失败 · 输出与期望不一致（实际 ${result.actions?.map((action) => `${displayName(action.action_id)}=${action.value}`).join("、") || "无输出"}）`
    return `失败 · ${result.detail || "无法模拟"}`
  }

  function applyTemplate(template: StrategyTemplate) {
    setRows(template.rows.map((row) => ({
      ...row,
      row_id: crypto.randomUUID(),
      conditions: row.conditions.map((condition) => ({ ...condition })),
      actions: row.actions.map((action) => ({ ...action })),
    })))
  }
  function applySelectedTemplate() {
    const template = STRATEGY_TEMPLATES.find((item) => item.name === templateName)
    if (template) applyTemplate(template)
  }
  async function runDiff() {
    setBusy("diff")
    onError("")
    try {
      setDiffResult((await diffRule(token, rule.id, diffFrom, diffTo)).diffs)
    } catch (error) {
      onError(error instanceof Error ? error.message : "对比失败")
    } finally {
      setBusy("")
    }
  }
  async function runReplay() {
    setBusy("replay")
    onError("")
    try {
      setReplayResult(await replayRule(token, rule.id, replayVersion, inputs, Object.fromEntries(Object.entries(textInputs).filter(([, text]) => text !== "")), simAt || undefined))
    } catch (error) {
      onError(error instanceof Error ? error.message : "回放失败")
    } finally {
      setBusy("")
    }
  }
  async function runDeps() {
    setBusy("deps")
    onError("")
    try {
      setDeps(await fetchRuleDeps(token, rule.id, { ...document, bindings: bindingsFor(available, document.bindings) }))
    } catch (error) {
      onError(error instanceof Error ? error.message : "依赖图生成失败")
    } finally {
      setBusy("")
    }
  }

  return <section className="rule-editor">
    <div className="editor-heading">
      <div>
        <span className="section-kicker">有序决策表</span>
        <h2>{nameOf(rule)}</h2>
        <p>同一行的条件必须同时满足。从上到下，命中首行即停止。</p>
      </div>
      <div className="rule-state">
        <span className={dirty ? "state-dot amber" : "state-dot"}>{dirty ? "有未保存修改" : "草稿已保存"}</span>
        <span>{published ? "已有发布版本" : "尚未发布"}</span>
        <span>{rows.length} 行 · {referencedInputs.length} 个输入</span>
      </div>
    </div>

    {!readOnly && <div className="rule-management">
      {showRename && <form onSubmit={(event) => { event.preventDefault(); void manage("rename") }}><input autoFocus aria-label="规则名称" value={newName} onChange={(event) => setNewName(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape") setShowRename(false) }} required /><button className="secondary-button" disabled={busy !== ""}>保存名称</button></form>}
      <button className="text-button" onClick={() => { setNewName(nameOf(rule)); setShowRename(!showRename) }}>{showRename ? "取消重命名" : "重命名"}</button>
      <button className="text-button" disabled={busy !== ""} onClick={() => void manage("copy")}>复制</button>
      <button className="text-button" onClick={() => setShowVersions(!showVersions)}>版本与历史</button>
      <button className="text-button" onClick={() => setShowStrategy(!showStrategy)}>策略工具</button>
      <button className="text-button danger-text" disabled={busy !== ""} onClick={() => setConfirmAction("delete")}>删除</button>
    </div>}
    <div className="editor-view-switch" role="group" aria-label="编辑视图">
      <button className={viewMode === "table" ? "active" : ""} onClick={() => { setViewMode("table"); localStorage.setItem("easyems-rule-view", "table") }}>表格视图</button>
      <button className={viewMode === "cards" ? "active" : ""} onClick={() => { setViewMode("cards"); localStorage.setItem("easyems-rule-view", "cards") }}>详细视图</button>
    </div>
    {viewMode === "table" && <div className="rule-table-wrap">
      <table className="rule-table">
        <thead><tr><th>#</th><th>条件（全部满足）</th><th>执行动作</th><th>持续（秒）</th><th>冷却（秒）</th><th>操作</th></tr></thead>
        <tbody>
          {rows.map((row, rowIndex) => <tr key={row.row_id} className={result?.row_id === row.row_id ? "sim-hit" : ""} title="点击行编辑条件和动作"
            onClick={(event) => {
              if ((event.target as HTMLElement).closest("input,button,select,label")) return
              setViewMode("cards")
              localStorage.setItem("easyems-rule-view", "cards")
              window.setTimeout(() => locate(row.row_id), 0)
            }}>
            <td>{rowIndex + 1}</td>
            <td><div className="rule-chips">
              {row.conditions.length === 0 ? <span className="rule-chip default">默认行</span> : row.conditions.map((condition, index) => <span className="rule-chip" key={index}>{condition.kind === "time" ? `时间 ${timeConditionText(condition)}` : `${displayName(condition.point_id)} ${operatorLabel(condition.operator)} ${conditionValueText(condition)}`}</span>)}
            </div></td>
            <td><div className="rule-chips">
              {row.actions.length === 0 ? <span className="rule-chip muted">不执行动作</span> : row.actions.map((action, index) => <span className="rule-chip" key={index}>{displayName(action.action_id)} = {action.value}</span>)}
            </div></td>
            <td><input aria-label="持续确认秒" type="number" min="0" step="0.1" disabled={readOnly} value={Math.round((row.hold_ms || 0) / 100) / 10} onChange={(event) => updateRow(rowIndex, { ...row, hold_ms: Math.max(0, Math.round((Number(event.target.value) || 0) * 1000)) })} /></td>
            <td><input aria-label="冷却秒" type="number" min="0" step="0.1" disabled={readOnly} value={Math.round((row.cooldown_ms || 0) / 100) / 10} onChange={(event) => updateRow(rowIndex, { ...row, cooldown_ms: Math.max(0, Math.round((Number(event.target.value) || 0) * 1000)) })} /></td>
            <td className="rule-table-actions">
              <button title="上移" disabled={rowIndex === 0 || readOnly} onClick={() => move(rowIndex, -1)}>↑</button>
              <button title="下移" disabled={rowIndex === rows.length - 1 || readOnly} onClick={() => move(rowIndex, 1)}>↓</button>
              <button onClick={() => { setViewMode("cards"); window.setTimeout(() => locate(row.row_id), 0) }}>编辑</button>
              <button disabled={readOnly} onClick={() => duplicateRow(rowIndex)}>复制</button>
              <button className="danger-text" disabled={readOnly} onClick={() => setRows(rows.filter((_, i) => i !== rowIndex))}>删除</button>
            </td>
          </tr>)}
          {rows.length === 0 && <tr><td colSpan={6}><div className="empty-state">还没有规则，先添加一行。</div></td></tr>}
        </tbody>
      </table>
    </div>}
    {showVersions && !readOnly && <div className="version-panel">
      {published && <div className="version-compare"><strong>草稿与运行版本</strong><span>当前草稿 {rows.length} 行；运行版本 {appliedRows} 行</span><span>{versionDifferent ? "存在规则内容差异" : "规则内容一致"}</span></div>}
      {versions.length === 0 ? <p>首次发布后，再次发布会在这里保存历史版本。</p> : versions.map((version) => <div className="version-line" key={version.id}><div><strong>{new Date(version.published_at).toLocaleString("zh-CN")}</strong><span>{version.document.table.rows.length} 行</span></div><button className="secondary-button" disabled={busy !== ""} onClick={() => void manage("restore", version)}>恢复此版本</button></div>)}
    </div>}

    {showStrategy && !readOnly && <div className="strategy-panel">
      <div className="version-compare"><strong>策略模板</strong><span>套用预置储能策略，再按本机点位重新绑定输入与输出。</span></div>
      <div className="strategy-row">
        <select aria-label="策略模板" value={templateName} onChange={(event) => setTemplateName(event.target.value)}>
          <option value="">选择模板…</option>
          {STRATEGY_TEMPLATES.map((template) => <option key={template.name} value={template.name}>{template.name} · {template.description}</option>)}
        </select>
        <button className="secondary-button" disabled={!templateName || busy !== ""} onClick={applySelectedTemplate}>套用（替换当前规则行）</button>
      </div>

      <div className="version-compare"><strong>版本对比</strong><span>逐行比较两个版本。</span></div>
      <div className="strategy-row">
        <label>从
          <select aria-label="对比源" value={diffFrom} onChange={(event) => setDiffFrom(event.target.value)}>
            <option value="applied">运行版本</option>
            <option value="draft">草稿</option>
            {versions.map((version) => <option key={version.id} value={version.id}>{new Date(version.published_at).toLocaleString("zh-CN", { hour12: false })}</option>)}
          </select>
        </label>
        <label>到
          <select aria-label="对比目标" value={diffTo} onChange={(event) => setDiffTo(event.target.value)}>
            <option value="draft">草稿</option>
            <option value="applied">运行版本</option>
            {versions.map((version) => <option key={version.id} value={version.id}>{new Date(version.published_at).toLocaleString("zh-CN", { hour12: false })}</option>)}
          </select>
        </label>
        <button className="secondary-button" disabled={busy !== ""} onClick={() => void runDiff()}>{busy === "diff" ? "对比中…" : "对比"}</button>
      </div>
      {diffResult && <div className="strategy-result">
        {diffResult.length === 0 ? <span>两个版本内容一致。</span> : diffResult.map((diff) => (
          <div className="diff-line" key={diff.row_id}>
            <strong>{diff.kind === "added" ? "新增" : diff.kind === "removed" ? "删除" : "修改"}</strong>
            {diff.from && <span>原：{diff.from}</span>}
            {diff.to && <span>新：{diff.to}</span>}
            {diff.detail?.map((item, i) => <span key={i}>{item}</span>)}
          </div>
        ))}
      </div>}

      <div className="version-compare"><strong>历史回放</strong><span>对某版本在某时刻模拟，绝不写设备。</span></div>
      <div className="strategy-row">
        <label>版本
          <select aria-label="回放版本" value={replayVersion} onChange={(event) => setReplayVersion(event.target.value)}>
            <option value="applied">运行版本</option>
            {versions.map((version) => <option key={version.id} value={version.id}>{new Date(version.published_at).toLocaleString("zh-CN", { hour12: false })}</option>)}
          </select>
        </label>
        <label>时刻
          <input aria-label="回放时刻" type="datetime-local" value={simAt} onChange={(event) => setSimAt(event.target.value)} />
        </label>
        <button className="secondary-button" disabled={busy !== ""} onClick={() => void runReplay()}>{busy === "replay" ? "回放中…" : "回放"}</button>
      </div>
      {replayResult && <div className="strategy-result" role="status">
        <span>{replayResult.row_id ? `命中第 ${rows.findIndex((row) => row.row_id === replayResult.row_id) + 1} 行` : "没有命中规则"}</span>
        <span>{replayResult.actions?.length ? replayResult.actions.map((action) => `${displayName(action.action_id)} 设为 ${action.value}`).join("；") : "不执行任何操作"}</span>
      </div>}

      <div className="version-compare"><strong>依赖图</strong><span>输入点位、规则行与输出点位的引用关系。</span></div>
      <div className="strategy-row">
        <button className="secondary-button" disabled={busy !== ""} onClick={() => void runDeps()}>{busy === "deps" ? "生成中…" : "生成依赖图"}</button>
      </div>
      {deps && <div className="strategy-result">
        <div className="deps-nodes">
          {deps.nodes.map((node) => <span key={node.id} className={`dep-node dep-${node.kind}`}>{node.label}</span>)}
        </div>
        <div className="deps-edges">
          {deps.edges.map((edge, i) => <span key={i}>{edge.from.replace(/^(input|output|row):/, "")} → {edge.to.replace(/^(input|output|row):/, "")}</span>)}
        </div>
      </div>}
    </div>}

    <fieldset className={`editor-fields ${viewMode === "table" ? "table-mode" : ""}`} disabled={readOnly}>
    <div className="rows">
      {rows.map((row, rowIndex) => <article id={`rule-row-${row.row_id}`} className={`decision-row ${result?.row_id === row.row_id ? "sim-hit" : ""}`} key={row.row_id}>
        <div className="row-order">
          <strong>{rowIndex + 1}</strong>
          <button aria-label="上移规则" title="上移" disabled={rowIndex === 0} onClick={() => move(rowIndex, -1)}>↑</button>
          <button aria-label="下移规则" title="下移" disabled={rowIndex === rows.length - 1} onClick={() => move(rowIndex, 1)}>↓</button>
          {lastMatch?.text === row.row_id && <span className="row-hit" title="最近一次运行时命中">命中</span>}
        </div>
        <div className="row-main">
          {row.conditions.length === 0 && rowIndex < rows.length - 1 && <div className="row-warning" role="status">默认行会命中所有输入，下面的规则不会执行。</div>}
          <div className="logic-block">
            <div className="logic-title"><span>当以下条件同时满足</span><button className="text-button" disabled={!available.length} title={!available.length ? "没有可用输入点位，先到设备页添加" : undefined} onClick={() => {
              if (!available[0]) return
              updateRow(rowIndex, { ...row, conditions: [...row.conditions, { point_id: businessId(available[0]), operator: "gt", value: 0 }] })
            }}>+ 条件</button></div>
            {row.conditions.length === 0 && <div className="default-row">默认行：以上规则均未命中时执行</div>}
            {row.conditions.map((condition, conditionIndex) => {
              const kind = condition.kind || "number"
              return <div className="logic-line condition-line" key={conditionIndex}>
                {kind !== "time" && <select aria-label="输入点位" value={condition.point_id} onChange={(event) => updateCondition(rowIndex, conditionIndex, { point_id: event.target.value })}>
                  {available.map((node) => <option key={node.id} value={businessId(node)}>{nameOf(node)}</option>)}
                </select>}
                <select aria-label="条件类型" value={kind} onChange={(event) => {
                  const next = event.target.value as "number" | "bool" | "enum" | "time"
                  if (next === "time") {
                    updateCondition(rowIndex, conditionIndex, {
                      kind: "time", point_id: "", operator: "", value: 0, text: "",
                      start: condition.start || "08:00", end: condition.end || "20:00",
                      weekday_mask: condition.weekday_mask || 0,
                    })
                    return
                  }
                  updateCondition(rowIndex, conditionIndex, {
                    kind: next,
                    point_id: condition.point_id || (available[0] ? businessId(available[0]) : ""),
                    operator: next === "number" ? (kind === "time" ? "gt" : condition.operator) : "eq",
                    value: next === "bool" ? 0 : condition.value,
                    text: next === "enum" ? (condition.text || "") : "",
                    start: "", end: "", weekday_mask: 0,
                  })
                }}>
                  <option value="number">数值</option>
                  <option value="bool">开关</option>
                  <option value="enum">枚举</option>
                  <option value="time">时间</option>
                </select>
                {kind === "time"
                  ? <>
                    <input aria-label="开始时间" type="time" value={condition.start || ""} onChange={(event) => updateCondition(rowIndex, conditionIndex, { start: event.target.value })} />
                    <span className="set-label">至</span>
                    <input aria-label="结束时间" type="time" value={condition.end || ""} onChange={(event) => updateCondition(rowIndex, conditionIndex, { end: event.target.value })} />
                    <span className="weekday-picks" role="group" aria-label="星期">
                      {weekdayOptions.map(([label, bit]) => <label key={bit}>
                        <input type="checkbox" checked={Boolean((condition.weekday_mask || 0) & (1 << bit))} onChange={(event) => {
                          const mask = condition.weekday_mask || 0
                          updateCondition(rowIndex, conditionIndex, { weekday_mask: event.target.checked ? mask | (1 << bit) : mask & ~(1 << bit) })
                        }} />{label}
                      </label>)}
                    </span>
                  </>
                  : <>
                    <select aria-label="比较逻辑" value={condition.operator} onChange={(event) => updateCondition(rowIndex, conditionIndex, { operator: event.target.value })}>
                      {operatorsFor(kind).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                    </select>
                    {kind === "bool"
                      ? <select aria-label="条件值" value={condition.value ? 1 : 0} onChange={(event) => updateCondition(rowIndex, conditionIndex, { value: Number(event.target.value) })}>
                        <option value={1}>是</option><option value={0}>否</option>
                      </select>
                      : kind === "enum"
                        ? <input aria-label="枚举值" value={condition.text || ""} placeholder="如 grid / 故障" onChange={(event) => updateCondition(rowIndex, conditionIndex, { text: event.target.value })} />
                        : <NumberInput label="条件值" value={condition.value} onChange={(value) => updateCondition(rowIndex, conditionIndex, { value })} />}
                  </>}
                <button className="icon-button" aria-label="删除条件" onClick={() => updateRow(rowIndex, { ...row, conditions: row.conditions.filter((_, i) => i !== conditionIndex) })}>×</button>
                {kind === "time" && <small className="time-hint">站点本地时间；开始晚于结束表示跨午夜（{timeConditionText(condition)}）；不勾选星期即每天。</small>}
              </div>
            })}
          </div>
          <div className="logic-block output-block">
            <div className="logic-title"><span>按顺序执行</span><button className="text-button" disabled={!targetOutputs.length} title={!targetOutputs.length ? "没有可写输出点位，先到设备页添加" : undefined} onClick={() => {
              if (!targetOutputs[0]) return
              updateRow(rowIndex, { ...row, actions: [...row.actions, { action_id: businessId(targetOutputs[0]), value: 1 }] })
            }}>+ 输出</button></div>
            {row.actions.length === 0 && <div className="no-action">{targetOutputs.length === 0 ? "暂无可写点位或分配器；请先添加可写输出。" : "不执行任何操作"}</div>}
            {row.actions.map((action, actionIndex) => <div className="logic-line" key={actionIndex}>
              <select aria-label="输出点位" value={action.action_id} onChange={(event) => updateAction(rowIndex, actionIndex, { action_id: event.target.value })}>
                {targetOutputs.map((node) => <option key={node.id} value={businessId(node)}>{nameOf(node)}{node.type === "group" ? "（分配器）" : ""}{(point(node, "easyemsManual")?.value || 0) !== 0 ? "（人工接管）" : ""}</option>)}
              </select>
              <span className="set-label">设为</span>
              <NumberInput label="输出值" value={action.value} onChange={(value) => updateAction(rowIndex, actionIndex, { value })} />
              {!readOnly && (() => {
                const target = available.find((node) => businessId(node) === action.action_id)
                return target ? <button className={`safety-link ${point(target, "easyemsSafety")?.text ? "configured" : "unconfigured"}`} title={point(target, "easyemsSafety")?.text ? "已配置安全约束" : "未配置安全约束"} onClick={() => setSafetyNode(target)}>安全</button> : null
              })()}
              <button className="icon-button" aria-label="删除输出" onClick={() => updateRow(rowIndex, { ...row, actions: row.actions.filter((_, i) => i !== actionIndex) })}>×</button>
            </div>)}
          </div>
          <div className="row-timing">
            <label>持续确认
              <input aria-label="持续确认秒" type="number" min="0" step="0.1" value={Math.round((row.hold_ms || 0) / 100) / 10}
                onChange={(event) => updateRow(rowIndex, { ...row, hold_ms: Math.max(0, Math.round((Number(event.target.value) || 0) * 1000)) })} /> 秒
            </label>
            <label>冷却
              <input aria-label="冷却秒" type="number" min="0" step="0.1" value={Math.round((row.cooldown_ms || 0) / 100) / 10}
                onChange={(event) => updateRow(rowIndex, { ...row, cooldown_ms: Math.max(0, Math.round((Number(event.target.value) || 0) * 1000)) })} /> 秒
            </label>
            <small>条件持续满足后才触发；触发后冷却期内不重复执行。0 表示不等待 / 不冷却。</small>
          </div>
        </div>
        <div className="row-actions">
          <button className="text-button" onClick={() => duplicateRow(rowIndex)}>复制行</button>
          <button className="delete-row" onClick={() => setRows(rows.filter((_, i) => i !== rowIndex))}>删除行</button>
        </div>
      </article>)}
      {rows.length === 0 && <div className="empty-state">
        <strong>{available.length ? "还没有规则" : "先接入点位，再写规则"}</strong>
        <span>{available.length ? "添加一行，选择输入条件和输出动作。" : "当前没有可用的输入点位。先到设备页接入 Modbus 或添加变量，再回来配置。"}</span>
      </div>}
      <div ref={rowsEnd} />
    </div>

    {(referencedInputs.length > 0 || referencedActions.length > 0) && <details className="binding-panel">
      <summary>点位来源 / 换品牌 · {referencedInputs.length} 输入 / {referencedActions.length} 输出</summary>
      <p>只更换数据来源或执行目标，业务 ID 与上面的规则不变。保存草稿不影响运行，发布后生效。</p>
      <div className="binding-grid">
        {referencedInputs.map((id) => <label key={`input-${id}`}>{displayName(id)} · 输入来源<select value={document.bindings.points[id]?.node_id || ""} onChange={(event) => rebind("points", id, event.target.value)}>
          {available.map((node) => <option key={node.id} value={node.id}>{nameOf(node)}</option>)}
        </select></label>)}
        {referencedActions.map((id) => <label key={`output-${id}`}>{displayName(id)} · 输出目标<select value={document.bindings.actions[id]?.node_id || ""} onChange={(event) => rebind("actions", id, event.target.value)}>
          {targetOutputs.map((node) => <option key={node.id} value={node.id}>{nameOf(node)}{node.type === "group" ? "（分配器）" : ""}</option>)}
        </select></label>)}
      </div>
    </details>}
    </fieldset>

    {!readOnly && <div className="add-row-actions"><button onClick={() => addRow()}>+ 添加规则行</button><button onClick={() => addRow(true)}>+ 默认不动作行</button></div>}

    {lastCheck && <details className="last-check">
      <summary>
        最近一次求值 · {lastCheck.at ? new Date(lastCheck.at).toLocaleTimeString("zh-CN", { hour12: false }) : "—"}
        {" · "}{checkStateLabels[lastCheck.state] || lastCheck.state || "未知状态"}
        {" · "}{rows.findIndex((row) => row.row_id === lastCheck.row) >= 0 ? `命中第 ${rows.findIndex((row) => row.row_id === lastCheck.row) + 1} 行` : "未命中行"}
      </summary>
      <div className="last-check-grid">
        <div>
          <b>输入</b>
          {lastCheck.inputs.length === 0 && <span>无输入</span>}
          {lastCheck.inputs.map((input, index) => <span key={`${input.input}-${index}`}>
            {input.input.startsWith("time:")
              ? <>{timeInputLabel(input.input)} = {input.value ? "满足" : "不满足"}</>
              : <>{displayName(input.input)} = {input.kind === "enum" ? (input.text || "—") : input.value} <em>{input.quality}</em></>}
            {input.source_time ? ` · ${new Date(input.source_time).toLocaleTimeString("zh-CN", { hour12: false })}` : ""}
          </span>)}
        </div>
        <div>
          <b>动作</b>
          {lastCheck.actions.length === 0 && <span>不执行动作</span>}
          {lastCheck.actions.map((action, index) => <span key={`${action.output}-${index}`}>
            {displayName(action.output)} {action.requested} → {action.applied} · {action.blocked ? "已拦截" : action.status === "confirmed" ? "已回读" : action.status === "unknown" ? "结果未知" : action.status === "failed" ? "失败" : action.reason?.startsWith("ramping") ? "爬坡中" : "已下发"}
            {action.reason ? `（${action.reason.startsWith("ramping") ? "正在接近目标" : action.reason}）` : ""}
          </span>)}
        </div>
      </div>
    </details>}

    <div className={`simulation-panel ${result ? "has-result" : ""}`} ref={simulationRef}>
      <div><span className="section-kicker">不会操作设备</span><h3>用当前草稿模拟</h3></div>
      <div className="simulation-inputs">
        {referencedInputs.map((id) => {
          const kind = rows.flatMap((row) => row.conditions).find((condition) => condition.point_id === id && condition.kind && condition.kind !== "number")?.kind || "number"
          return <label key={id}>{displayName(id)}
            {kind === "bool"
              ? <select value={inputs[id] ? 1 : 0} onChange={(event) => setInputs({ ...inputs, [id]: Number(event.target.value) })}><option value={1}>是</option><option value={0}>否</option></select>
              : kind === "enum"
                ? <input value={textInputs[id] || ""} placeholder="枚举值" onChange={(event) => setTextInputs({ ...textInputs, [id]: event.target.value })} />
                : <NumberInput label={displayName(id)} value={inputs[id] ?? 0} onChange={(value) => setInputs({ ...inputs, [id]: value })} />}
          </label>
        })}
        {referencedInputs.length === 0 && <span>当前只有默认行，可直接模拟。</span>}
        {referencedInputs.length > 0 && <button type="button" className="text-button" onClick={useLiveInputs}>用当前值</button>}
        <label>模拟时刻
          <input aria-label="模拟时刻" type="datetime-local" value={simAt} onChange={(event) => setSimAt(event.target.value)} />
        </label>
        {simAt && <button type="button" className="text-button" onClick={() => setSimAt("")}>用现在</button>}
        <button className="secondary-button" disabled={busy !== ""} onClick={() => run("test")}>{busy === "test" ? "计算中…" : "开始模拟"}</button>
      </div>
      {result && <div className="simulation-result" role="status" ref={resultRef}>
        {result.row_id ? <>命中第 {rows.findIndex((row) => row.row_id === result.row_id) + 1} 行 <button type="button" className="inline-link" onClick={() => locate(result.row_id)}>定位该行</button></> : "没有命中规则"}
        <span>{result.actions?.length ? result.actions.map((action) => `${displayName(action.action_id)} 设为 ${action.value}`).join("；") : "不执行任何操作"}</span>
        {result.time_checks?.length ? <span>{result.time_checks.map(timeCheckText).join("；")}</span> : null}
      </div>}

      <div className="scenario-panel">
        <div className="logic-title"><span>场景测试集 · 存在草稿里，不影响发布版本</span></div>
        {!readOnly && <div className="scenario-form">
          <input aria-label="场景名称" placeholder="场景名称" value={scenarioName} onChange={(event) => setScenarioName(event.target.value)} />
          <label>期望结果
            <select aria-label="期望结果" value={scenarioExpect} onChange={(event) => setScenarioExpect(event.target.value)}>
              <option value="">不命中任何行</option>
              {rows.map((row, index) => <option key={row.row_id} value={row.row_id}>第 {index + 1} 行</option>)}
            </select>
          </label>
          <button type="button" className="secondary-button" onClick={addScenario}>把当前输入和时刻存为场景</button>
        </div>}
        {scenarios.length === 0 && <small>先在上面设好输入与模拟时刻，再保存为场景；一键全部运行可回归检查整张表。</small>}
        {scenarios.length > 0 && <>
          <div className="scenario-list">
            {scenarios.map((scenario) => {
              const result = scenarioResults[scenario.scenario_id]
              return <div className="scenario-line" key={scenario.scenario_id}>
                <div className="scenario-main">
                  <strong>{scenario.name || "未命名场景"}</strong>
                  <span>{[
                    ...Object.entries(scenario.inputs || {}).map(([id, value]) => `${displayName(id)}=${scenario.text_inputs?.[id] ?? value}`),
                    ...(scenario.at ? [`时刻 ${scenario.at.replace("T", " ")}`] : []),
                  ].join("、") || "无输入"}</span>
                  <span>期望：{rowLabel(scenario.expect_row)}{scenario.expect_actions?.length ? `；${scenario.expect_actions.map((action) => `${displayName(action.action_id)}=${action.value}`).join("、")}` : ""}</span>
                </div>
                {result && <span className={result.passed ? "scenario-pass" : "scenario-fail"} role="status">{scenarioVerdictText(scenario, result)}</span>}
                <span className="scenario-actions">
                  <button type="button" className="text-button" onClick={() => loadScenario(scenario)}>载入</button>
                  {!readOnly && <button type="button" className="text-button danger-text" onClick={() => setScenarios(scenarios.filter((item) => item.scenario_id !== scenario.scenario_id))}>删除</button>}
                </span>
              </div>
            })}
          </div>
          <div className="scenario-run">
            <button type="button" className="secondary-button" disabled={busy !== ""} onClick={() => void runScenarios()}>{busy === "scenarios" ? "运行中…" : "运行全部场景"}</button>
            {Object.keys(scenarioResults).length > 0 && <span role="status">{scenarios.filter((scenario) => scenarioResults[scenario.scenario_id]?.passed).length}/{scenarios.length} 通过 · 模拟运行，不会操作设备</span>}
          </div>
        </>}
      </div>
    </div>

    {runtimeError && <div className="inline-error">运行异常：{runtimeError}</div>}
    {safetyNode && <SafetyModal token={token} node={safetyNode} nodes={nodes} onClose={() => setSafetyNode(null)} onSaved={() => { setSafetyNode(null); void onRefresh() }} />}

    <div className="publish-bar">
      <div>
        <span>自动运行</span>
        <IonToggle checked={enabled} disabled={readOnly || busy !== "" || (!enabled && !published)} onIonChange={(event) => run("enable", event.detail.checked)} />
        {!published && <small>发布后才能启用</small>}
      </div>
      <span className="last-match">最近命中：{lastMatchIndex >= 0 && lastMatch?.text ? <button type="button" className="inline-link" onClick={() => locate(lastMatch.text)}>{lastMatchLabel}</button> : lastMatchLabel}</span>
      <div className="action-buttons">
        <button className="text-button" onClick={() => simulationRef.current?.scrollIntoView({ block: "center" })}>模拟</button>
        <button className="secondary-button" disabled={readOnly || busy !== ""} onClick={() => run("save")}>{busy === "save" ? "保存中…" : "保存草稿"}</button>
        <button className="primary-button" disabled={readOnly || busy !== "" || rows.length === 0} onClick={() => setConfirmAction("publish")}>{busy === "publish" ? "发布中…" : "发布当前规则"}</button>
      </div>
    </div>

    {confirmAction && <ConfirmDialog
      title={confirmAction === "publish" ? "确认发布当前规则" : "确认删除规则"}
      message={confirmAction === "publish" ? "发布后规则会按当前草稿运行；请确认条件、目标和顺序已经检查。" : "删除后规则及其版本将从业务树移除，不能撤销。"}
      confirmLabel={confirmAction === "publish" ? "确认发布" : "确认删除"}
      danger={confirmAction === "delete"}
      busy={busy !== ""}
      rows={[{ label: "规则", value: nameOf(rule) }, { label: "行数", value: `${rows.length}` }]}
      onCancel={() => setConfirmAction(null)}
      onConfirm={() => {
        const action = confirmAction
        setConfirmAction(null)
        if (action === "publish") void run("publish")
        else void manage("delete")
      }}
    />}
  </section>
}
