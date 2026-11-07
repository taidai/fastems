import { useCallback, useEffect, useMemo, useState } from "react"
import { downloadHistory, fetchHistory, nameOf, Node, point, Point, request } from "./api"

const ranges = [
  { key: "1h", label: "1 小时", hours: 1 },
  { key: "24h", label: "24 小时", hours: 24 },
  { key: "7d", label: "7 天", hours: 24 * 7 },
]

function Trend({ values }: { values: number[] }) {
  if (values.length < 2) return <div className="trend-empty">这段时间内没有足够的数据点</div>
  const width = 760
  const height = 240
  const pad = 28
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const step = (width - pad * 2) / (values.length - 1)
  const coords = values.map((value, index) => ({
    x: pad + index * step,
    y: height - pad - ((value - min) / span) * (height - pad * 2),
  }))
  const last = coords[coords.length - 1]
  return <svg className="trend-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="历史趋势">
    {[0.25, 0.5, 0.75].map((ratio) => <line key={ratio} x1={pad} x2={width - pad} y1={pad + ratio * (height - pad * 2)} y2={pad + ratio * (height - pad * 2)} className="trend-grid" />)}
    <polyline points={coords.map(({ x, y }) => `${x},${y}`).join(" ")} className="trend-line" />
    <circle cx={last.x} cy={last.y} r="4.5" className="trend-dot" />
    <text x={pad} y={pad - 8} className="trend-label">{max.toFixed(1)}</text>
    <text x={pad} y={height - pad + 16} className="trend-label">{min.toFixed(1)}</text>
  </svg>
}

function recentPoint(points: Node[]) {
  const ranked = points.map((node) => {
    const time = point(node, "value")?.time || ""
    return { id: node.id, time }
  }).filter((item) => item.time).sort((a, b) => b.time.localeCompare(a.time))
  return ranked[0]?.id || points[0]?.id || ""
}

export default function HistoryView({ token, points, initialPoint, onError, isAdmin = false }: {
  token: string
  points: Node[]
  initialPoint?: string
  onError: (message: string) => void
  isAdmin?: boolean
}) {
  const [selected, setSelected] = useState(() => initialPoint && points.some((item) => item.id === initialPoint) ? initialPoint : recentPoint(points))
  const [range, setRange] = useState("24h")
  const [interval, setInterval] = useState("raw")
  const [samples, setSamples] = useState<Point[]>([])
  const [loading, setLoading] = useState(false)
  const [limitDraft, setLimitDraft] = useState(20000)
  const [limitBusy, setLimitBusy] = useState(false)
  const [limitNotice, setLimitNotice] = useState("")

  const hours = ranges.find((item) => item.key === range)?.hours || 24
  const node = points.find((item) => item.id === selected)
  const units = node ? point(node, "units")?.text || "" : ""

  const bounds = useMemo(() => {
    const end = new Date()
    const start = new Date(end.getTime() - hours * 3600 * 1000)
    return { start, end }
  }, [hours])

  const load = useCallback(async () => {
    if (!selected) {
      setSamples([])
      return
    }
    setLoading(true)
    try {
      const result = await fetchHistory(token, selected, bounds.start, bounds.end)
      setSamples(result.points || [])
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

  const displaySamples = useMemo(() => {
    if (interval === "raw") return samples
    const size = interval === "5m" ? 5 * 60_000 : 60 * 60_000
    const buckets = new Map<number, { sum: number; count: number; last: Point }>()
    for (const sample of samples) {
      const value = Number(sample.value)
      if (!Number.isFinite(value)) continue
      const at = new Date(sample.time).getTime()
      if (!Number.isFinite(at)) continue
      const key = Math.floor(at / size) * size
      const current = buckets.get(key)
      if (current) { current.sum += value; current.count += 1; current.last = sample }
      else buckets.set(key, { sum: value, count: 1, last: sample })
    }
    return [...buckets.entries()].sort((a, b) => a[0] - b[0]).map(([time, bucket]) => ({ ...bucket.last, time: new Date(time).toISOString(), value: bucket.sum / bucket.count }))
  }, [interval, samples])

  const values = displaySamples
    .map((sample) => Number(sample.value))
    .filter((value) => Number.isFinite(value))
  const latest = values.length ? values[values.length - 1] : undefined

  if (points.length === 0) {
    return <div className="panel empty-state large">
      <strong>还没有可查询的点位</strong>
      <span>先在工作台接入 Modbus 设备或添加测试变量，历史与趋势会从这里读取。</span>
    </div>
  }

  return <section className="history-page">
    <div className="panel history-toolbar">
      <label>设备点位
        <select value={selected} onChange={(event) => setSelected(event.target.value)}>
          {points.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}
        </select>
      </label>
      <div className="range-buttons" role="group" aria-label="时间范围">
        {ranges.map((item) => <button key={item.key} className={range === item.key ? "active" : ""} onClick={() => setRange(item.key)}>{item.label}</button>)}
      </div>
      <label>聚合
        <select value={interval} onChange={(event) => setInterval(event.target.value)}>
          <option value="raw">原始</option><option value="5m">5 分钟平均</option><option value="1h">1 小时平均</option>
        </select>
      </label>
      <button className="secondary-button" disabled={!selected} onClick={() => void downloadHistory(token, selected, bounds.start, bounds.end).catch((cause) => onError(cause instanceof Error ? cause.message : "导出失败"))}>导出 CSV</button>
      {isAdmin && <div className="history-limit"><span>每个点位保留</span><input aria-label="历史保留条数" type="number" min={1000} max={200000} step={1000} value={limitDraft} onChange={(event) => setLimitDraft(Number(event.target.value))} /><span>条</span><button className="secondary-button" disabled={limitBusy} onClick={() => void saveLimit()}>{limitBusy ? "保存中…" : "保存"}</button>{limitNotice && <small>{limitNotice}</small>}</div>}
      <span className="history-hint">类型 value · 单次最多 2000 点 · 聚合只影响显示</span>
    </div>

    <div className="panel trend-panel">
      <div className="trend-head">
        <div><span className="section-kicker">最近变化</span><h2>{node ? nameOf(node) : "选择点位"}</h2></div>
        <div className="trend-value">{latest !== undefined ? latest : "—"}<small>{units}</small></div>
      </div>
      {loading ? <div className="trend-empty">正在读取历史…</div> : <Trend values={values} />}
      <p className="trend-note">缺失段不插值；数据来自内嵌历史，按点位保留上限裁剪。过期或无效的数据不会冒充实时值。</p>
    </div>
  </section>
}
