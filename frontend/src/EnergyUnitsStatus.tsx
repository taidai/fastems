import { useMemo, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsSectionHeader, InlineNotice, StatusBadge, type EmsTone } from "./components/ems"
import { nameOf, Node, point, sendEnergyUnitEvent, setEnergyUnitMode } from "./api"

// EnergyUnitsStatus 用领域模型的实时数据展示"每柜现在在做什么"：
// 运行状态（cabinetState）、额定/可用功率、最近一次调度的请求/下发/缺口，
// 并给有权限的操作者提供启停、检修与自动/人工切换。

type DispatchLast = { request_kw: number; delivered_kw: number; shortfall_kw: number; at: string; source?: string; stage?: string }

const stateMeta: Record<string, { label: string; tone: EmsTone }> = {
  running: { label: "运行", tone: "success" },
  starting: { label: "启动中", tone: "busy" },
  stopping: { label: "停机中", tone: "busy" },
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

function availableKW(node: Node): { charge: number | null; discharge: number | null } {
  const text = point(node, "capability")?.text
  if (!text) return { charge: null, discharge: null }
  try {
    const cap = JSON.parse(text) as { power_max_kw?: number; charge_max_kw?: number; discharge_max_kw?: number }
    const fallback = cap.power_max_kw ?? null
    return { charge: cap.charge_max_kw ?? fallback, discharge: cap.discharge_max_kw ?? fallback }
  } catch {
    return { charge: null, discharge: null }
  }
}

export default function EnergyUnitsStatus({ nodes, onOpen, token, canOperate, canConfigure }: {
  nodes: Node[]
  onOpen: () => void
  token?: string
  canOperate?: boolean
  canConfigure?: boolean
}) {
  const [pending, setPending] = useState<{ id: string; action: string; mode?: "auto" | "manual" } | null>(null)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<{ tone: "success" | "warning" | "danger"; text: string } | null>(null)
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

  async function act(unit: Node, action: string, mode?: "auto" | "manual") {
    if (!token) return
    setBusy(true)
    setNotice(null)
    try {
      if (mode) {
        await setEnergyUnitMode(token, unit.id, mode)
        setNotice({ tone: "success", text: mode === "manual" ? "已切换到人工模式，自动调度停止，显式恢复后才会继续。" : "已恢复自动调度。" })
      } else {
        const result = await sendEnergyUnitEvent(token, unit.id, action, crypto.randomUUID())
        setNotice({ tone: "success", text: `指令 ${action} 已提交（${result.request_id.slice(0, 8)}），等待设备回读确认。` })
      }
      setPending(null)
    } catch (error) {
      setNotice({ tone: "danger", text: error instanceof Error ? error.message : "操作失败" })
    } finally {
      setBusy(false)
    }
  }

  const pendingLabel = (target: { action: string; mode?: "auto" | "manual" }) => target.action === "mode"
    ? (target.mode === "manual" ? "人工接管" : "恢复自动")
    : ({ start: "启动", stop: "停机", enter_maintenance: "进入检修", exit_maintenance: "退出检修", fault_clear: "确认故障解除" }[target.action] || target.action)

  return <EmsCard>
    <IonCardContent className="grid gap-4">
      <EmsSectionHeader kicker="能源设备运行" title="每柜当前状态"
        description={`${stats.running} 台运行中${stats.attention > 0 ? ` · ${stats.attention} 台需处理` : " · 全部正常"}`}
        actions={canConfigure ? <EmsButton variant="secondary" onClick={onOpen}>调度配置</EmsButton> : undefined} />
      {notice && <InlineNotice tone={notice.tone}>{notice.text}</InlineNotice>}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {units.map((unit) => {
          const state = point(unit, "cabinetState")?.text || "running"
          const meta = stateMeta[state] || { label: state, tone: "neutral" as EmsTone }
          const rated = point(unit, "ratedPowerKW")?.text
          const available = availableKW(unit)
          const manual = (point(unit, "easyemsManual")?.value ?? point(unit, "easyemsManual")?.text) ? "人工" : "自动"
          const last = parseLast(unit)
          const canStart = state === "standby" || state === "stopped"
          const canStop = state === "running" || state === "standby" || state === "starting"
          const inMaintenance = state === "maintenance"
          return <div className="grid gap-3 rounded-xl border border-ems-line bg-white/70 p-3" key={unit.id}>
            <div className="flex items-center justify-between gap-2">
              <strong className="text-sm text-ems-ink">{nameOf(unit)}</strong>
              <div className="flex items-center gap-2">
                <StatusBadge tone={manual === "人工" ? "warning" : "neutral"}>{manual}</StatusBadge>
                <StatusBadge tone={meta.tone}>{meta.label}</StatusBadge>
              </div>
            </div>
            <div className="flex flex-wrap gap-5">
              <div><span className="mr-2 text-xs text-ems-muted">额定</span><b className="text-lg text-ems-ink">{rated ? `${rated} kW` : "—"}</b></div>
              <div><span className="mr-2 text-xs text-ems-muted">充</span><b className="text-lg text-ems-ink">{available.charge !== null ? `${available.charge} kW` : "—"}</b></div>
              <div><span className="mr-2 text-xs text-ems-muted">放</span><b className="text-lg text-ems-ink">{available.discharge !== null ? `${available.discharge} kW` : "—"}</b></div>
            </div>
            {last
              ? <div className="text-xs text-ems-muted">最近调度：请求 {last.request_kw} kW · 提交 {last.delivered_kw} kW
                  {last.shortfall_kw > 0 && <span className="ml-1 font-medium text-ems-amber">缺口 {last.shortfall_kw} kW</span>}
                </div>
              : <div className="text-xs text-ems-faint">暂无调度记录</div>}
            {canOperate && token && <div className="grid gap-2 border-t border-ems-line pt-2">
              {pending?.id === unit.id
                ? <div className="flex flex-wrap items-center gap-2">
                    <span className="text-xs text-ems-muted">确认对 {nameOf(unit)} 执行“{pendingLabel(pending)}”？</span>
                    <EmsButton size="small" busy={busy} busyLabel="提交中…" onClick={() => void act(unit, pending.action, pending.mode)}>确认</EmsButton>
                    <EmsButton size="small" variant="quiet" disabled={busy} onClick={() => setPending(null)}>取消</EmsButton>
                  </div>
                : <div className="flex flex-wrap gap-2">
                    {canStart && <EmsButton size="small" disabled={busy} onClick={() => setPending({ id: unit.id, action: "start" })}>启动</EmsButton>}
                    {canStop && <EmsButton size="small" variant="secondary" disabled={busy} onClick={() => setPending({ id: unit.id, action: "stop" })}>停机</EmsButton>}
                    {state !== "fault" && <EmsButton size="small" variant="secondary" disabled={busy} onClick={() => setPending({ id: unit.id, action: inMaintenance ? "exit_maintenance" : "enter_maintenance" })}>{inMaintenance ? "退出检修" : "进入检修"}</EmsButton>}
                    {state === "fault" && <EmsButton size="small" variant="danger" disabled={busy} onClick={() => setPending({ id: unit.id, action: "fault_clear" })}>确认故障解除</EmsButton>}
                    <EmsButton size="small" variant="quiet" disabled={busy} onClick={() => setPending({ id: unit.id, action: "mode", mode: manual === "人工" ? "auto" : "manual" })}>
                      {manual === "人工" ? "恢复自动" : "人工接管"}
                    </EmsButton>
                  </div>}
              <p className="m-0 text-xs text-ems-faint">启停指令须设备回读确认后才改变状态；人工接管后自动目标停止，需显式恢复。</p>
            </div>}
          </div>
        })}
      </div>
    </IonCardContent>
  </EmsCard>
}
