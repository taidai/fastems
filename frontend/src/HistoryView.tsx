import { useCallback, useEffect, useMemo, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { downloadHistory, EmsAlarm, fetchAlarms, fetchEmsHistory, fetchEmsState, fetchHistory, nameOf, Node, point, Point, request } from "./api"
import { EmsButton, EmsCard, EmsInput, EmsSectionHeader, EmsSelect, EmsSelectOption, EmptyState, InlineNotice, SegmentedTabs } from "./components/ems"

const ranges = [
  { key: "1h", label: "1 小时", hours: 1 },
  { key: "24h", label: "24 小时", hours: 24 },
  { key: "7d", label: "7 天", hours: 24 * 7 },
]

const seriesColors = ["#2f6f9f", "#c07a1f", "#2d8f78", "#b14f6f", "#6b4fa0", "#5f8f3f", "#8a5a2b", "#3f7f7f"]
const severityColors: Record<string, string> = { critical: "#b14f6f", warning: "#b07a2a", info: "#2f6f9f" }

type SavedQuery = { name: string; points: string[]; range: string; interval: string }
type QualityCounts = { good: number; stale: number; invalid: number; none: number }
type TrendSeries = { id: string; name: string; color: string; units: string; latestValue?: number; samples: { time: number; value: number }[] }
type AlarmMark = { at: number; severity: string; title: string }

const savedQueryKey = "fastiot-history-queries"

function loadSavedQueries(): SavedQuery[] {
  try { return JSON.parse(localStorage.getItem(savedQueryKey) || "[]") as SavedQuery[] } catch { return [] }
}

function recentPoint(points: Node[]) {
  const ranked = points.map((node) => {
    const time = point(node, "value")?.time || ""
    return { id: node.id, time }
  }).filter((item) => item.time).sort((a, b) => b.time.localeCompare(a.time))
  return ranked[0]?.id || points[0]?.id || ""
}

function aggregate(samples: Point[], interval: string): { time: number; value: number }[] {
  const raw = samples
    .map((sample) => ({ time: new Date(sample.time).getTime(), value: Number(sample.value) }))
    .filter((item) => Number.isFinite(item.time) && Number.isFinite(item.value))
  if (interval === "raw") return raw
  const size = interval === "5m" ? 5 * 60_000 : 60 * 60_000
  const buckets = new Map<number, { sum: number; count: number }>()
  for (const item of raw) {
    const key = Math.floor(item.time / size) * size
    const bucket = buckets.get(key)
    if (bucket) { bucket.sum += item.value; bucket.count += 1 }
    else buckets.set(key, { sum: item.value, count: 1 })
  }
  return [...buckets.entries()].sort((a, b) => a[0] - b[0]).map(([time, bucket]) => ({ time, value: bucket.sum / bucket.count }))
}

function Trend({ series, alarms, start, end }: { series: TrendSeries[]; alarms: AlarmMark[]; start: number; end: number }) {
  const width = 760
  const height = 260
  const pad = 30
  const plotW = width - pad * 2
  const plotH = height - pad * 2
  const values = series.flatMap((item) => item.samples.map((sample) => sample.value))
  const min = values.length ? Math.min(...values) : 0
  const max = values.length ? Math.max(...values) : 1
  const span = max - min || 1
  const domain = end > start ? end - start : 1
  const xOf = (time: number) => pad + ((time - start) / domain) * plotW
  const yOf = (value: number) => height - pad - ((value - min) / span) * plotH
  const lines = series.map((item) => ({
    ...item,
    coords: item.samples
      .filter((sample) => sample.time >= start && sample.time <= end)
      .map((sample) => ({ x: xOf(sample.time), y: yOf(sample.value) })),
  }))
  const marks = alarms.filter((alarm) => alarm.at >= start && alarm.at <= end)

  return <div className="trend-wrap">
    <div className="trend-legend" role="list" aria-label="对比图例">
      {series.map((item) => <span key={item.id} className="trend-legend-item" role="listitem">
        <i style={{ background: item.color }} />
        <b>{item.name}</b>
        <em>{item.latestValue === undefined ? "无数据" : `${item.latestValue}${item.units}`}</em>
      </span>)}
    </div>
    {values.length < 2 ? <div className="trend-empty">这段时间内没有足够的数据点</div> : <svg className="trend-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="历史趋势">
      {[0.25, 0.5, 0.75].map((ratio) => <line key={ratio} x1={pad} x2={width - pad} y1={pad + ratio * plotH} y2={pad + ratio * plotH} className="trend-grid" />)}
      {marks.map((alarm) => {
        const x = xOf(alarm.at)
        return <line key={`${alarm.at}-${alarm.title}`} x1={x} x2={x} y1={pad} y2={height - pad} className="trend-alarm" style={{ stroke: severityColors[alarm.severity] || severityColors.info }}>
          <title>{alarm.title} · {new Date(alarm.at).toLocaleString("zh-CN", { hour12: false })}</title>
        </line>
      })}
      {lines.map((item) => <polyline key={item.id} points={item.coords.map(({ x, y }) => `${x},${y}`).join(" ")} className="trend-line" style={{ stroke: item.color }} />)}
      <text x={pad} y={pad - 8} className="trend-label">{max.toFixed(1)}</text>
      <text x={pad} y={height - pad + 16} className="trend-label">{min.toFixed(1)}</text>
    </svg>}
    {marks.length > 0 && <p className="trend-alarm-note">图上竖线为所选时间段内与所选点位相关的报警事件，悬停查看详情。</p>}
  </div>
}

export default function HistoryView({ token, points, initialPoint, onError, isAdmin = false }: {
  token: string
  points: Node[]
  initialPoint?: string
  onError: (message: string) => void
  isAdmin?: boolean
}) {
  const [selected, setSelected] = useState<string[]>(() => {
    const first = initialPoint && points.some((item) => item.id === initialPoint) ? initialPoint : recentPoint(points)
    return first ? [first] : []
  })
  const [range, setRange] = useState("24h")
  const [interval, setInterval] = useState("raw")
  const [seriesData, setSeriesData] = useState<Record<string, Point[]>>({})
  const [loading, setLoading] = useState(false)
  const [alarms, setAlarms] = useState<EmsAlarm[]>([])
  const [showAlarms, setShowAlarms] = useState(true)
  const [quality, setQuality] = useState<Record<string, QualityCounts>>({})
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>(loadSavedQueries)
  const [queryName, setQueryName] = useState("")
  const [limitDraft, setLimitDraft] = useState(20000)
  const [limitBusy, setLimitBusy] = useState(false)
  const [limitNotice, setLimitNotice] = useState("")

  const hours = ranges.find((item) => item.key === range)?.hours || 24

  const bounds = useMemo(() => {
    const end = new Date()
    const start = new Date(end.getTime() - hours * 3600 * 1000)
    return { start, end }
  }, [hours])

  const load = useCallback(async () => {
    if (selected.length === 0) {
      setSeriesData({})
      return
    }
    setLoading(true)
    try {
      const entries = await Promise.all(selected.map(async (id) => {
        const result = await fetchHistory(token, id, bounds.start, bounds.end)
        return [id, result.points || []] as const
      }))
      setSeriesData(Object.fromEntries(entries))
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "历史读取失败")
    } finally {
      setLoading(false)
    }
  }, [token, selected, bounds, onError])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    let cancelled = false
    const loadAlarms = async () => {
      try {
        const result = await fetchAlarms(token)
        if (!cancelled) setAlarms(result.alarms || [])
      } catch {
        // 报警叠加是补充信息，失败不阻塞趋势。
      }
    }
    void loadAlarms()
    const timer = window.setInterval(() => void loadAlarms(), 5000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [token])

  useEffect(() => {
    let cancelled = false
    const loadQuality = async () => {
      try {
        const state = await fetchEmsState(token)
        const deviceByPoint = new Map((state.values || []).map((item) => [item.point_id, item.device_id]))
        const counts: Record<string, QualityCounts> = {}
        await Promise.all(selected.map(async (id) => {
          const deviceId = deviceByPoint.get(id)
          if (!deviceId) return
          try {
            const result = await fetchEmsHistory(token, deviceId, id, bounds.start, bounds.end)
            const bucket: QualityCounts = { good: 0, stale: 0, invalid: 0, none: 0 }
            for (const sample of result.samples || []) {
              if (sample.quality === "good") bucket.good += 1
              else if (sample.quality === "stale") bucket.stale += 1
              else if (sample.quality === "invalid") bucket.invalid += 1
              else bucket.none += 1
            }
            counts[id] = bucket
          } catch {
            // 单个点位质量缺失不影响其他点位。
          }
        }))
        if (!cancelled) setQuality(counts)
      } catch {
        // 质量摘要只覆盖已接入运行时的点位，失败不阻塞趋势。
      }
    }
    void loadQuality()
    return () => { cancelled = true }
  }, [token, selected, bounds])

  useEffect(() => {
    if (!isAdmin) return
    void request<{ limit: number }>(token, "/history-settings").then((result) => setLimitDraft(result.limit)).catch(() => {})
  }, [token, isAdmin])

  async function saveLimit() {
    setLimitBusy(true)
    setLimitNotice("")
    try {
      const result = await request<{ limit: number }>(token, "/history-settings", "PUT", { limit: limitDraft })
      setLimitDraft(result.limit)
      setLimitNotice("已保存；运行历史立即生效，持久历史重启后生效")
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "历史保留保存失败")
    } finally {
      setLimitBusy(false)
    }
  }

  const series = useMemo<TrendSeries[]>(() => selected.map((id, index) => {
    const node = points.find((item) => item.id === id)
    const samples = aggregate(seriesData[id] || [], interval)
    return {
      id,
      name: node ? nameOf(node) : id,
      color: seriesColors[index % seriesColors.length],
      units: node ? point(node, "units")?.text || "" : "",
      latestValue: samples.length ? samples[samples.length - 1].value : undefined,
      samples,
    }
  }), [selected, points, seriesData, interval])

  const qualityTotals = useMemo(() => {
    const totals: QualityCounts = { good: 0, stale: 0, invalid: 0, none: 0 }
    for (const id of selected) {
      const bucket = quality[id]
      if (!bucket) continue
      totals.good += bucket.good
      totals.stale += bucket.stale
      totals.invalid += bucket.invalid
      totals.none += bucket.none
    }
    return totals
  }, [selected, quality])

  const qualityEntries = [
    { key: "good", label: "正常", value: qualityTotals.good },
    { key: "stale", label: "过期", value: qualityTotals.stale },
    { key: "invalid", label: "无效", value: qualityTotals.invalid },
    { key: "none", label: "无值", value: qualityTotals.none },
  ]
  const qualityTotal = qualityEntries.reduce((sum, entry) => sum + entry.value, 0)

  const alarmMarks = useMemo<AlarmMark[]>(() => {
    if (!showAlarms) return []
    const selectedSet = new Set(selected)
    return alarms
      .filter((alarm) => {
        const at = new Date(alarm.created_at).getTime()
        if (!Number.isFinite(at) || at < bounds.start.getTime() || at > bounds.end.getTime()) return false
        return selectedSet.has(alarm.point_id || alarm.device_id || "")
      })
      .map((alarm) => ({ at: new Date(alarm.created_at).getTime(), severity: alarm.severity, title: alarm.title }))
  }, [showAlarms, alarms, selected, bounds])

  const primary = series[0]
  const title = selected.length <= 1 ? (primary?.name || "选择点位") : `对比 ${selected.length} 个点位`

  function addPoint(id: string) {
    if (!id || selected.includes(id)) return
    setSelected([...selected, id])
  }

  function removePoint(id: string) {
    if (selected.length <= 1) return
    setSelected(selected.filter((item) => item !== id))
  }

  const saveQueries = (list: SavedQuery[]) => {
    setSavedQueries(list)
    localStorage.setItem(savedQueryKey, JSON.stringify(list))
  }

  const defaultQueryName = useMemo(() => {
    const parts = [
      ...selected.map((id) => points.find((item) => item.id === id) ? nameOf(points.find((item) => item.id === id)!) : id),
      ranges.find((item) => item.key === range)?.label || range,
      interval === "raw" ? "原始" : interval === "5m" ? "5 分钟" : "1 小时",
    ]
    return parts.filter(Boolean).join("·")
  }, [selected, points, range, interval])

  const matchedQuery = savedQueries.find((query) =>
    query.points.join("\u0000") === selected.join("\u0000") && query.range === range && query.interval === interval)

  function applyQuery(name: string) {
    const query = savedQueries.find((item) => item.name === name)
    if (!query) return
    setSelected(query.points.filter((id) => points.some((item) => item.id === id)))
    setRange(query.range)
    setInterval(query.interval)
    setQueryName(query.name)
  }

  function saveQuery() {
    const name = queryName.trim() || defaultQueryName
    saveQueries([...savedQueries.filter((query) => query.name !== name), { name, points: selected, range, interval }])
    setQueryName(name)
  }

  if (points.length === 0) {
    return <section className="history-page">
      <EmptyState title="还没有可查询的点位" description="先在工作台接入 Modbus 设备或添加测试变量，历史与趋势会从这里读取。" />
    </section>
  }

  return <section className="history-page min-w-0">
    <EmsCard className="!block !p-0"><IonCardContent className="grid min-w-0 gap-4">
      <div className="flex min-w-0 flex-wrap items-end gap-3">
        <div className="w-full min-w-0 sm:w-56"><EmsSelect label="对比点位" aria-label="添加对比点位" value="" onIonChange={(event) => addPoint(event.detail.value)}>
          <EmsSelectOption value="">＋ 添加点位…</EmsSelectOption>
          {points.map((item) => <EmsSelectOption key={item.id} value={item.id}>{nameOf(item)}</EmsSelectOption>)}
        </EmsSelect></div>
        <div className="history-series min-w-0 flex-1" role="list" aria-label="已选对比点位">
          {selected.map((id) => {
            const node = points.find((item) => item.id === id)
            const color = seriesColors[selected.indexOf(id) % seriesColors.length]
            return <span key={id} className="history-series-chip min-w-0 max-w-full" role="listitem">
              <i style={{ background: color }} />
              <b className="min-w-0 break-words">{node ? nameOf(node) : id}</b>
              <button aria-label={`移除 ${node ? nameOf(node) : id}`} disabled={selected.length <= 1} onClick={() => removePoint(id)}>×</button>
            </span>
          })}
        </div>
        <EmsButton variant="secondary" size="small" disabled={selected.length === 0} onClick={() => void downloadHistory(token, selected[0], bounds.start, bounds.end).catch((cause) => onError(cause instanceof Error ? cause.message : "导出失败"))}>导出 CSV</EmsButton>
      </div>

      <div className="flex min-w-0 flex-wrap items-end gap-3">
        <div className="grid min-w-0 gap-1"><span className="ems-field-label text-ems-muted">时间范围</span><SegmentedTabs value={range} onChange={setRange} ariaLabel="时间范围" options={ranges.map((item) => ({ value: item.key, label: item.label }))} /></div>
        <div className="w-full min-w-0 sm:w-40"><EmsSelect label="聚合" aria-label="聚合" value={interval} onIonChange={(event) => setInterval(event.detail.value)}>
          <EmsSelectOption value="raw">原始</EmsSelectOption><EmsSelectOption value="5m">5 分钟平均</EmsSelectOption><EmsSelectOption value="1h">1 小时平均</EmsSelectOption>
        </EmsSelect></div>
        <label className="history-alarm-toggle min-h-10"><input type="checkbox" checked={showAlarms} onChange={(event) => setShowAlarms(event.target.checked)} />叠加报警</label>
      </div>

      {isAdmin && <div className="history-limit flex flex-wrap items-end gap-2">
        <div className="w-40"><EmsInput label="每个点位保留" aria-label="历史保留条数" type="number" min="1000" max="200000" step="1000" value={String(limitDraft)} onIonInput={(event) => setLimitDraft(Number(event.detail.value))} /></div>
        <span className="pb-3 text-xs text-ems-muted">条</span>
        <EmsButton variant="secondary" size="small" busy={limitBusy} busyLabel="保存中…" onClick={() => void saveLimit()}>保存</EmsButton>
        {limitNotice && <small className="w-full !whitespace-normal">{limitNotice}</small>}
      </div>}

      <div className="flex min-w-0 flex-wrap items-end gap-2">
        {savedQueries.length > 0 && <div className="w-full min-w-0 sm:w-56"><EmsSelect label="已保存查询" aria-label="保存的查询" value="" onIonChange={(event) => applyQuery(event.detail.value)}>
          <EmsSelectOption value="">查询/模板…</EmsSelectOption>
          {savedQueries.map((query) => <EmsSelectOption key={query.name} value={query.name}>{query.name}</EmsSelectOption>)}
        </EmsSelect></div>}
        <div className="w-full min-w-0 sm:w-56"><EmsInput label="查询名称" aria-label="查询名称" value={queryName} onIonInput={(event) => setQueryName(event.detail.value || "")} placeholder="查询/模板名称" /></div>
        {matchedQuery
          ? <EmsButton variant="secondary" size="small" onClick={() => saveQueries(savedQueries.filter((query) => query !== matchedQuery))}>删查询</EmsButton>
          : <EmsButton variant="secondary" size="small" disabled={selected.length === 0} onClick={saveQuery}>保存查询</EmsButton>}
      </div>
      <span className="history-hint !m-0">类型 value · 单次最多 2000 点 · 聚合只影响显示</span>
    </IonCardContent></EmsCard>

    <EmsCard className="trend-panel min-w-0">
      <EmsSectionHeader kicker="最近变化" title={title} actions={selected.length === 1 && primary ? <div className="trend-value">{primary.latestValue ?? "—"}<small>{primary.units}</small></div> : undefined} />
      <div className="trend-body mt-4">
        <div className="trend-main">
          {loading ? <InlineNotice tone="busy">正在读取历史…</InlineNotice> : <Trend series={series} alarms={alarmMarks} start={bounds.start.getTime()} end={bounds.end.getTime()} />}
        </div>
        <aside className="quality-summary min-w-0" aria-label="数据质量摘要">
          <EmsSectionHeader kicker="数据质量" title="所选时间段" />
          {qualityTotal === 0
            ? <p className="quality-empty">所选点位暂无运行质量样本。</p>
            : <ul className="quality-bars">
              {qualityEntries.map((entry) => {
                const ratio = qualityTotal ? Math.round((entry.value / qualityTotal) * 100) : 0
                return <li key={entry.key} className={`quality-row ${entry.key}`}>
                  <span className="quality-name">{entry.label}</span>
                  <span className="quality-track"><i style={{ width: `${ratio}%` }} /></span>
                  <span className="quality-count">{entry.value}<em>{ratio}%</em></span>
                </li>
              })}
              <li className="quality-total">共 {qualityTotal} 个样本</li>
            </ul>}
          <p className="quality-note">质量来自接入运行时的 Modbus 点位；测试变量不经运行时质量判定。</p>
        </aside>
      </div>
      <div className="trend-note mt-3"><InlineNotice tone="info">缺失段不插值；数据来自内嵌历史，按点位保留上限裁剪。过期或无效的数据不会冒充实时值。</InlineNotice></div>
    </EmsCard>
  </section>
}
