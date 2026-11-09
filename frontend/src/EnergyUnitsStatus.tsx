import { useMemo } from "react"
import { nameOf, Node, point } from "./api"

// EnergyUnitsStatus 用领域模型的实时数据展示"每柜现在在做什么"：
// 运行状态（cabinetState）、额定/可用功率、最近一次调度的请求/下发/缺口。
// 这是值班员视角的首屏核心——不再只看点位设备树。

type DispatchLast = { request_kw: number; delivered_kw: number; shortfall_kw: number; at: string }

const stateMeta: Record<string, { label: string; tone: string }> = {
  running: { label: "运行", tone: "ok" },
  starting: { label: "启动中", tone: "warn" },
  standby: { label: "待机", tone: "muted" },
  stopped: { label: "停机", tone: "muted" },
  fault: { label: "故障", tone: "bad" },
  maintenance: { label: "检修", tone: "warn" },
}

function parseLast(node: Node): DispatchLast | null {
  const text = point(node, "easyemsDispatchLog")?.text
  if (!text) return null
  try {
    const records = JSON.parse(text) as DispatchLast[]
    return records.length ? records[records.length - 1] : null
  } catch {
    return null
  }
}

function availableKW(node: Node): number | null {
  const text = point(node, "capability")?.text
  if (!text) return null
  try {
    const cap = JSON.parse(text) as { power_max_kw?: number }
    return cap.power_max_kw ?? null
  } catch {
    return null
  }
}

export default function EnergyUnitsStatus({ nodes, onOpen }: {
  nodes: Node[]
  onOpen: () => void
}) {
  const units = useMemo(() => nodes.filter((node) => node.type === "energyUnit"), [nodes])
  const stats = useMemo(() => {
    let running = 0, attention = 0
    for (const unit of units) {
      const state = point(unit, "cabinetState")?.text || "running"
      if (state === "fault" || state === "maintenance") attention++
      else if (state === "running") running++
    }
    return { running, attention }
  }, [units])
  if (units.length === 0) return null

  return <section className="panel energy-status-panel">
    <div className="panel-heading"><div><span className="section-kicker">储能柜运行</span><h2>每柜当前状态</h2>
      <p className="energy-status-summary">{stats.running} 台运行中{stats.attention > 0 ? ` · ${stats.attention} 台需处理` : " · 全部正常"}</p>
    </div>
      <button className="secondary-button" onClick={onOpen}>调度配置</button>
    </div>
    <div className="energy-status-grid">
      {units.map((unit) => {
        const state = point(unit, "cabinetState")?.text || "running"
        const meta = stateMeta[state] || { label: state, tone: "muted" }
        const rated = point(unit, "ratedPowerKW")?.text
        const available = availableKW(unit)
        const last = parseLast(unit)
        return <div className="energy-status-card" key={unit.id}>
          <div className="energy-status-head">
            <strong>{nameOf(unit)}</strong>
            <span className={`energy-status-badge ${meta.tone}`}>{meta.label}</span>
          </div>
          <div className="energy-status-figures">
            <div><span>额定</span><b>{rated ? `${rated} kW` : "—"}</b></div>
            <div><span>可用</span><b>{available !== null ? `${available} kW` : "—"}</b></div>
          </div>
          {last
            ? <div className="energy-status-dispatch">
                最近调度：请求 {last.request_kw} kW · 下发 {last.delivered_kw} kW
                {last.shortfall_kw > 0 && <span className="energy-status-shortfall">缺口 {last.shortfall_kw} kW</span>}
              </div>
            : <div className="energy-status-dispatch muted">暂无调度记录</div>}
        </div>
      })}
    </div>
  </section>
}
