import { useEffect, useState } from "react"
import { writable } from "./writable"
import ConfirmDialog from "./ConfirmDialog"
import {
  EmsAlarm,
  fetchAlarms,
  fetchHistory,
  fetchScreen,
  nameOf,
  Node,
  point,
  request,
  saveScreen,
  ScreenConfig,
  Snapshot,
} from "./api"

type Run = (action: () => Promise<unknown>) => Promise<boolean>

function TileTrend({ token, nodeId }: { token: string; nodeId: string }) {
  const [values, setValues] = useState<number[]>([])
  useEffect(() => {
    let cancelled = false
    const end = new Date()
    const start = new Date(end.getTime() - 3600 * 1000)
    fetchHistory(token, nodeId, start, end)
      .then((result) => {
        if (cancelled) return
        const series = (result.points || [])
          .map((item) => Number(item.value))
          .filter((value) => Number.isFinite(value))
          .slice(-60)
        setValues(series)
      })
      .catch(() => { /* 趋势不可用不影响卡片其它信息 */ })
    return () => { cancelled = true }
  }, [token, nodeId])

  if (values.length < 2) return <div className="tile-trend-empty">最近一小时暂无趋势</div>
  const width = 220
  const height = 48
  const min = Math.min(...values)
  const max = Math.max(...values)
  const span = max - min || 1
  const step = width / (values.length - 1)
  const points = values.map((value, index) => `${index * step},${height - 4 - ((value - min) / span) * (height - 8)}`).join(" ")
  return <svg className="tile-trend" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" role="img" aria-label="最近一小时趋势">
    <polyline points={points} className="tile-trend-line" />
  </svg>
}

export default function ScreenView({ token, snapshot, points, groups, isAdmin, canOperate, busy, run, onError, openHistory, openAlarms }: {
  token: string
  snapshot: Snapshot
  points: Node[]
  groups: Node[]
  isAdmin: boolean
  canOperate: boolean
  busy: boolean
  run: Run
  onError: (message: string) => void
  openHistory: (nodeId: string) => void
  openAlarms: () => void
}) {
  const [nodeID, setNodeID] = useState(snapshot.roots[0] || "")
  const [config, setConfig] = useState<ScreenConfig | null>(null)
  const [draft, setDraft] = useState<ScreenConfig | null>(null)
  const [editing, setEditing] = useState(false)
  const [alarmCount, setAlarmCount] = useState(0)
  const [topAlarm, setTopAlarm] = useState<EmsAlarm | null>(null)
  const [values, setValues] = useState<Record<string, number>>({})
  const [pendingWrite, setPendingWrite] = useState<{ node: Node; value: number } | null>(null)

  const selected = groups.find((group) => group.id === nodeID)
  const saved = selected ? point(selected, "easyemsScreen") : undefined

  useEffect(() => {
    let cancelled = false
    setConfig(null)
    if (!saved) {
      const fallback: ScreenConfig = {
        title: `${selected ? nameOf(selected) : "运行"}画面`,
        columns: 3,
        point_ids: points.slice(0, 4).map((item) => item.id),
      }
      setConfig(fallback)
      setDraft(fallback)
      return
    }
    fetchScreen(token, nodeID)
      .then((result) => {
        if (cancelled) return
        setConfig(result)
        setDraft(result)
      })
      .catch((cause) => {
        if (cancelled) return
        onError(cause instanceof Error ? cause.message : "画面读取失败")
      })
    return () => { cancelled = true }
    // points 每 2 秒刷新，不参与依赖，避免覆盖正在编辑的内容
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, nodeID, saved?.text])

  useEffect(() => {
    const load = async () => {
      try {
        const result = await fetchAlarms(token)
        const active = (result.alarms || []).filter((alarm) => alarm.active)
        setAlarmCount(active.length)
        setTopAlarm(active[0] || null)
      } catch {
        // 报警摘要失败不阻塞画面；错误由报警页负责展示
      }
    }
    void load()
    const timer = window.setInterval(() => void load(), 5000)
    return () => window.clearInterval(timer)
  }, [token])

  const tiles = config ? config.point_ids.map((id) => points.find((item) => item.id === id)).filter((item): item is Node => Boolean(item)) : []

  function togglePoint(id: string, checked: boolean) {
    if (!draft) return
    const next = checked ? [...draft.point_ids, id] : draft.point_ids.filter((item) => item !== id)
    setDraft({ ...draft, point_ids: next })
  }

  async function save() {
    if (!draft) return
    const ok = await run(async () => {
      await saveScreen(token, nodeID, draft)
      setConfig(draft)
      setEditing(false)
    })
    if (!ok) return
  }

  return <section className="screen-page">
    <div className="panel screen-toolbar">
      <label>画面所属节点
        <select value={nodeID} onChange={(event) => { setNodeID(event.target.value); setEditing(false) }}>
          {groups.map((group) => <option key={group.id} value={group.id}>{snapshot.roots.includes(group.id) ? "整个设备树" : nameOf(group)}</option>)}
        </select>
      </label>
      {isAdmin && <button className="secondary-button" onClick={() => { setEditing(!editing); setDraft(config) }}>{editing ? "退出编辑" : "配置画面"}</button>}
      <span className="screen-hint">配置保存在业务节点上，跟随节点权限</span>
    </div>

    {alarmCount > 0 && <button className="panel screen-alarm" onClick={openAlarms}>
      <strong>{alarmCount}</strong><span>项活动报警</span>
      <em>{topAlarm?.title}</em><span aria-hidden="true">查看 →</span>
    </button>}

    {editing && draft && <div className="panel screen-editor">
      <div className="form-row">
        <label>画面标题<input value={draft.title} onChange={(event) => setDraft({ ...draft, title: event.target.value })} /></label>
        <label>列数<select value={draft.columns} onChange={(event) => setDraft({ ...draft, columns: Number(event.target.value) })}>
          <option value={2}>2 列</option><option value={3}>3 列</option><option value={4}>4 列</option>
        </select></label>
      </div>
      <div className="screen-point-picker">
        {points.map((item) => <label className="screen-point-option" key={item.id}>
          <input type="checkbox" checked={draft.point_ids.includes(item.id)} onChange={(event) => togglePoint(item.id, event.target.checked)} />
          <span>{nameOf(item)}</span>
        </label>)}
        {points.length === 0 && <p className="form-note">还没有可选点位，请先接入设备或添加测试变量。</p>}
      </div>
      <div className="screen-editor-actions">
        <button className="secondary-button" onClick={() => { setEditing(false); setDraft(config) }}>取消</button>
        <button className="primary-button" disabled={busy} onClick={() => void save()}>保存画面</button>
      </div>
    </div>}

    {config && <div className="screen-grid" style={{ gridTemplateColumns: `repeat(${config.columns}, minmax(0, 1fr))` }}>
      {tiles.map((node) => {
        const current = point(node, "value")
        const units = point(node, "units")?.text || ""
        const value = values[node.id] ?? current?.value
        const updated = current?.time ? new Date(current.time) : null
        const stale = !updated || Date.now() - updated.getTime() > 15000
        return <article className="panel screen-tile" key={node.id}>
          <div className="screen-tile-head"><strong>{config.title && tiles.length === 1 ? config.title : nameOf(node)}</strong><span className={`screen-quality ${stale ? "stale" : ""}`}>{stale ? "数据过期" : "正常"}</span></div>
          <div className="screen-value">{value ?? "—"}<small>{units}</small></div>
          <div className="screen-meta">{updated ? updated.toLocaleTimeString("zh-CN", { hour12: false }) : "尚未采集"}</div>
          <TileTrend token={token} nodeId={node.id} />
          <div className="screen-tile-actions">
            <button className="secondary-button" onClick={() => openHistory(node.id)}>历史</button>
            {writable(node) && canOperate && <div className="screen-control">
              <input aria-label={`${nameOf(node)}目标值`} type="number" value={values[node.id] ?? current?.value ?? 0} onChange={(event) => setValues({ ...values, [node.id]: Number(event.target.value) })} />
              <button className="secondary-button" disabled={busy} onClick={() => setPendingWrite({ node, value: values[node.id] ?? current?.value ?? 0 })}>下发</button>
            </div>}
          </div>
        </article>
      })}
    </div>}

    {config && tiles.length === 0 && !editing && <div className="panel empty-state large">
      <strong>这张画面还没有点位</strong>
      <span>{isAdmin ? "点击“配置画面”选择要展示的点位。" : "请管理员配置要展示的点位。"}</span>
    </div>}

    {pendingWrite && <ConfirmDialog
      title={`确认下发「${nameOf(pendingWrite.node)}」`}
      message="目标值会直接写入设备，等待回读确认；失败不会自动重试。"
      rows={[
        { label: "当前值", value: `${point(pendingWrite.node, "value")?.value ?? "—"}${point(pendingWrite.node, "units")?.text ? ` ${point(pendingWrite.node, "units")?.text}` : ""}` },
        { label: "目标值", value: `${pendingWrite.value}${point(pendingWrite.node, "units")?.text ? ` ${point(pendingWrite.node, "units")?.text}` : ""}` },
      ]}
      confirmLabel="确认下发"
      busy={busy}
      onCancel={() => setPendingWrite(null)}
      onConfirm={() => {
        const target = pendingWrite
        setPendingWrite(null)
        void run(() => request(token, `/nodes/${target.node.id}/value`, "POST", { value: target.value, point_type: "valueSet" }))
      }}
    />}
  </section>
}
