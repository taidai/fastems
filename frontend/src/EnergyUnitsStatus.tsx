import { useMemo } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsSectionHeader, StatusBadge, type EmsTone } from "./components/ems"
import { nameOf, Node, point } from "./api"

// EnergyUnitsStatus 用领域模型的实时数据展示"每柜现在在做什么"：
// 运行状态（cabinetState）、额定/可用功率、最近一次调度的请求/下发/缺口。
// 这是值班员视角的首屏核心——不再只看点位设备树。

type DispatchLast = { request_kw: number; delivered_kw: number; shortfall_kw: number; at: string }

const stateMeta: Record<string, { label: string; tone: EmsTone }> = {
  running: { label: "运行", tone: "success" },
  starting: { label: "启动中", tone: "busy" },
  standby: { label: "待机", tone: "neutral" },
  stopped: { label: "停机", tone: "neutral" },
  fault: { label: "故障", tone: "danger" },
  maintenance: { label: "检修", tone: "warning" },
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

  return <EmsCard>
    <IonCardContent className="grid gap-4">
      <EmsSectionHeader kicker="能源设备运行" title="每柜当前状态"
        description={`${stats.running} 台运行中${stats.attention > 0 ? ` · ${stats.attention} 台需处理` : " · 全部正常"}`}
        actions={<EmsButton variant="secondary" onClick={onOpen}>调度配置</EmsButton>} />
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {units.map((unit) => {
          const state = point(unit, "cabinetState")?.text || "running"
          const meta = stateMeta[state] || { label: state, tone: "neutral" as EmsTone }
          const rated = point(unit, "ratedPowerKW")?.text
          const available = availableKW(unit)
          const last = parseLast(unit)
          return <div className="grid gap-3 rounded-xl border border-ems-line bg-white/70 p-3" key={unit.id}>
            <div className="flex items-center justify-between gap-2">
              <strong className="text-sm text-ems-ink">{nameOf(unit)}</strong>
              <StatusBadge tone={meta.tone}>{meta.label}</StatusBadge>
            </div>
            <div className="flex gap-6">
              <div><span className="mr-2 text-xs text-ems-muted">额定</span><b className="text-lg text-ems-ink">{rated ? `${rated} kW` : "—"}</b></div>
              <div><span className="mr-2 text-xs text-ems-muted">可用</span><b className="text-lg text-ems-ink">{available !== null ? `${available} kW` : "—"}</b></div>
            </div>
            {last
              ? <div className="text-xs text-ems-muted">最近调度：请求 {last.request_kw} kW · 下发 {last.delivered_kw} kW
                  {last.shortfall_kw > 0 && <span className="ml-1 font-medium text-ems-amber">缺口 {last.shortfall_kw} kW</span>}
                </div>
              : <div className="text-xs text-ems-faint">暂无调度记录</div>}
          </div>
        })}
      </div>
    </IonCardContent>
  </EmsCard>
}
