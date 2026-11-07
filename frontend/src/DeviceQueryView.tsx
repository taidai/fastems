import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react"
import { downloadHistory, EmsAlarm, fetchAlarms, nameOf, Node, point } from "./api"
import HistoryView from "./HistoryView"

type Tab = "overview" | "realtime" | "alarms" | "history" | "report"

type Row = {
  node: Node
  value?: number
  text: string
  units: string
  time: string
  quality: "normal" | "stale" | "missing"
}

function timeOf(value?: string) {
  if (!value) return "—"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString("zh-CN", { hour12: false })
}

function pointRow(node: Node): Row {
  const value = point(node, "value")
  const age = value?.time ? Date.now() - new Date(value.time).getTime() : NaN
  const numeric = value?.value
  if (value === undefined || !value.time) return { node, text: "—", units: point(node, "units")?.text || "", time: "—", quality: "missing" }
  return {
    node,
    value: numeric,
    text: numeric === undefined ? "—" : String(numeric),
    units: point(node, "units")?.text || "",
    time: timeOf(value.time),
    quality: Number.isFinite(age) && age <= 15000 && numeric !== undefined ? "normal" : "stale",
  }
}

export default function DeviceQueryView({ token, nodes, roots, groups, onError }: {
  token: string
  nodes: Node[]
  roots: string[]
  groups: Node[]
  onError: (message: string) => void
}) {
  const [selectedGroup, setSelectedGroup] = useState(roots[0] || "")
  const [tab, setTab] = useState<Tab>("overview")
  const [query, setQuery] = useState("")
  const [alarms, setAlarms] = useState<EmsAlarm[]>([])
  const [selectedPoint, setSelectedPoint] = useState("")
  const groupNodes = useMemo(() => groups.filter((node) => roots.includes(node.id) || node.type === "group"), [groups, roots])

  const children = useMemo(() => {
    const map = new Map<string, Node[]>()
    for (const node of nodes) map.set(node.parent, [...(map.get(node.parent) || []), node])
    return map
  }, [nodes])

  const descendants = useCallback((id: string): Node[] => {
    const result: Node[] = []
    const visit = (parent: string) => {
      for (const node of children.get(parent) || []) {
        result.push(node)
        visit(node.id)
      }
    }
    visit(id)
    return result
  }, [children])

  const selected = groupNodes.find((node) => node.id === selectedGroup)
  const selectedNodes = selected ? descendants(selected.id) : []
  const selectedPoints = selectedNodes.filter((node) => node.type === "variable" || node.type === "modbusIo")
  const rows = selectedPoints.map(pointRow)
  const ids = new Set([selected?.id, ...selectedNodes.map((node) => node.id)].filter(Boolean) as string[])
  const scopedAlarms = alarms.filter((alarm) => alarm.device_id && ids.has(alarm.device_id))
  const activeAlarms = scopedAlarms.filter((alarm) => alarm.active)
  const normal = rows.filter((row) => row.quality === "normal" && !point(row.node, "disabled")?.value).length
  const stale = rows.filter((row) => row.quality === "stale").length
  const missing = rows.filter((row) => row.quality === "missing").length

  useEffect(() => {
    if (!selectedGroup && roots[0]) setSelectedGroup(roots[0])
  }, [roots, selectedGroup])

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const result = await fetchAlarms(token)
        if (!cancelled) setAlarms(result.alarms || [])
      } catch {
        // The query page remains usable even if the alarm count is unavailable.
      }
    }
    void load()
    const timer = window.setInterval(() => void load(), 5000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [token])

  const tree = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const matches = (node: Node) => !needle || nameOf(node).toLowerCase().includes(needle)
    const childrenOf = (id: string) => groupNodes.filter((node) => node.parent === id)
    const render = (node: Node, depth: number): ReactElement[] => {
      const list = childrenOf(node.id).flatMap((child) => render(child, depth + 1))
      const points = descendants(node.id).filter((item) => item.type === "variable" || item.type === "modbusIo")
      const own = points.length
      const hits = points.filter(matches).length
      if (!matches(node) && list.length === 0 && hits === 0) return []
      return [<div key={node.id} className="query-tree-group" style={{ paddingLeft: `${depth * 14}px` }}>
        <button className={`query-tree-node ${selectedGroup === node.id ? "active" : ""}`} onClick={() => setSelectedGroup(node.id)}>
          <span>{nameOf(node)}</span><small>{own} 点位</small>
        </button>
        {list}
      </div>]
    }
    return roots.flatMap((id) => {
      const node = groupNodes.find((item) => item.id === id)
      return node ? render(node, 0) : []
    })
  }, [roots, groupNodes, query, selectedGroup, descendants])

  const overviewRows = rows.filter((row) => row.quality === "normal").slice(0, 6)
  const reportRows = rows

  return <section className="device-query-page">
    <aside className="panel query-tree">
      <div className="query-tree-head"><strong>站点 / 设备</strong><span>{groupNodes.length} 组</span></div>
      <input aria-label="搜索站点或设备" placeholder="搜索站点 / 设备" value={query} onChange={(event) => setQuery(event.target.value)} />
      <div className="query-tree-body">{tree}{tree.length === 0 && <div className="empty-state"><span>没有匹配的设备</span></div>}</div>
    </aside>

    <div className="query-main">
      <div className="query-tabs" role="group" aria-label="数据查询">
        {([["overview", "概览"], ["realtime", "实时数据"], ["alarms", "告警事件"], ["history", "历史趋势"], ["report", "数据报表"]] as const).map(([key, label]) =>
          <button key={key} className={tab === key ? "active" : ""} onClick={() => setTab(key)}>{label}</button>)}
      </div>

      {tab === "overview" && <div className="query-overview">
        <div className="query-summary">
          <div><strong>{rows.length}</strong><span>点位</span></div>
          <div><strong>{normal}</strong><span>正常</span></div>
          <div className={stale ? "warn" : ""}><strong>{stale}</strong><span>过期</span></div>
          <div className={missing ? "bad" : ""}><strong>{missing}</strong><span>无值</span></div>
          <div className={activeAlarms.length ? "bad" : ""}><strong>{activeAlarms.length}</strong><span>活动告警</span></div>
        </div>
        <div className="query-overview-grid">
          <div className="panel query-live-panel">
            <div className="query-panel-head"><div><span className="section-kicker">实时概览</span><h2>{selected ? nameOf(selected) : "—"}</h2></div><button className="secondary-button" onClick={() => setTab("realtime")}>全部实时数据</button></div>
            <div className="query-live-list">
              {overviewRows.map((row) => <button key={row.node.id} className="query-live-row" onClick={() => { setSelectedPoint(row.node.id); setTab("history") }}>
                <span>{nameOf(row.node)}</span><strong>{row.text}<small>{row.units}</small></strong><em className={row.quality}>{row.quality === "normal" ? "正常" : row.quality === "stale" ? "过期" : "无值"}</em>
              </button>)}
              {overviewRows.length === 0 && <div className="empty-state"><span>该站点暂无实时点位，或点表尚未接入。</span></div>}
            </div>
          </div>
          <div className="panel query-alarm-panel">
            <div className="query-panel-head"><div><span className="section-kicker">告警事件</span><h2>活动告警</h2></div><button className="secondary-button" onClick={() => setTab("alarms")}>查看全部</button></div>
            {activeAlarms.slice(0, 5).map((alarm) => <div key={alarm.id} className="query-alarm-row"><span className={`alarm-type-severity ${alarm.severity}`}>{alarm.severity === "critical" ? "严重" : alarm.severity === "warning" ? "重要" : "提示"}</span><strong>{alarm.title}</strong><small>{timeOf(alarm.created_at)}</small></div>)}
            {activeAlarms.length === 0 && <div className="empty-state"><span>当前没有活动告警。</span></div>}
          </div>
        </div>
      </div>}

      {tab === "realtime" && <div className="panel query-table-panel">
        <div className="query-panel-head"><div><span className="section-kicker">只读实时值</span><h2>{selected ? nameOf(selected) : "实时数据"}</h2></div><span className="query-note">{rows.length} 个点位</span></div>
        <div className="query-table-wrap">
          <table className="query-table">
            <thead><tr><th>点位</th><th>实时值</th><th>单位</th><th>质量</th><th>更新时间</th><th>来源</th></tr></thead>
            <tbody>{rows.map((row) => <tr key={row.node.id} onClick={() => { setSelectedPoint(row.node.id); setTab("history") }}>
              <td>{nameOf(row.node)}</td><td>{row.text}</td><td>{row.units || "—"}</td>
              <td><span className={`query-quality ${row.quality}`}>{row.quality === "normal" ? "正常" : row.quality === "stale" ? "过期" : "无值"}</span></td>
              <td>{row.time}</td><td>{(() => { const parent = nodes.find((node) => node.id === row.node.parent); return parent ? nameOf(parent) : "—" })()}</td>
            </tr>)}</tbody>
          </table>
          {rows.length === 0 && <div className="empty-state"><span>没有可查询的点位。</span></div>}
        </div>
      </div>}

      {tab === "alarms" && <div className="panel query-table-panel">
        <div className="query-panel-head"><div><span className="section-kicker">站点范围</span><h2>告警事件</h2></div><span className="query-note">{scopedAlarms.length} 条记录</span></div>
        <div className="query-alarm-list">
          {scopedAlarms.map((alarm) => <div key={alarm.id} className="query-alarm-card">
            <span className={`alarm-type-severity ${alarm.severity}`}>{alarm.severity === "critical" ? "严重" : alarm.severity === "warning" ? "重要" : "提示"}</span>
            <div><strong>{alarm.title}</strong><p>{alarm.detail || "没有更多说明"}</p></div>
            <span>{alarm.active ? "活动" : alarm.state === "resolved" ? "已恢复" : "已关闭"}</span><small>{timeOf(alarm.created_at)}</small>
          </div>)}
          {scopedAlarms.length === 0 && <div className="empty-state"><strong>该站点没有告警记录</strong><span>告警会在通道离线、命令异常、规则异常或自定义告警类型触发时出现。</span></div>}
        </div>
      </div>}

      {tab === "history" && <div className="query-history">
        {selectedPoints.length ? <HistoryView key={selectedPoint || selectedPoints[0].id} token={token} points={selectedPoints} initialPoint={selectedPoint || selectedPoints[0].id} onError={onError} /> :
          <div className="panel empty-state large"><strong>没有可查询的点位</strong><span>先在“通道”的端口配置或点位表接入点位。</span></div>}
      </div>}

      {tab === "report" && <div className="panel query-table-panel">
        <div className="query-panel-head"><div><span className="section-kicker">当前快照</span><h2>数据报表</h2></div><span className="query-note">导出为最近 24 小时历史 CSV</span></div>
        <div className="query-table-wrap">
          <table className="query-table">
            <thead><tr><th>点位</th><th>当前值</th><th>单位</th><th>质量</th><th>更新时间</th><th>导出</th></tr></thead>
            <tbody>{reportRows.map((row) => <tr key={row.node.id}>
              <td>{nameOf(row.node)}</td><td>{row.text}</td><td>{row.units || "—"}</td>
              <td><span className={`query-quality ${row.quality}`}>{row.quality === "normal" ? "正常" : row.quality === "stale" ? "过期" : "无值"}</span></td>
              <td>{row.time}</td>
              <td><button className="secondary-button" onClick={() => {
                const end = new Date(); const start = new Date(end.getTime() - 24 * 3600 * 1000)
                void downloadHistory(token, row.node.id, start, end).catch((cause) => onError(cause instanceof Error ? cause.message : "导出失败"))
              }}>导出 CSV</button></td>
            </tr>)}</tbody>
          </table>
          {reportRows.length === 0 && <div className="empty-state"><span>没有可导出的点位。</span></div>}
        </div>
      </div>}
    </div>
  </section>
}
