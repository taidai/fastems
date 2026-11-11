import { useCallback, useEffect, useMemo, useState } from "react"
import { ApiError, nameOf, point, request, type Node } from "./api"
import { EmsButton, EmsCard, EmsSectionHeader, InlineNotice, SegmentedTabs, StatusBadge, type EmsTone } from "./components/ems"
import DeployView from "./DeployView"
import acceptance from "./data/acceptance-status.json"
import type { Page } from "./navigation"

// 交付工作台：左侧 7 步、右侧当前步骤摘要 + 三个页签。
// 门槛只读并自动刷新；预检是手动快照，页面上把"实时状态"和"最近一次预检"分开显示。

type Gate = { key: string; title: string; status: string; detail: string; object?: string; next_step?: string }
type PreflightReport = {
  id: string
  generated_at: string
  operator?: string
  status: string
  summary: Record<string, number>
  gates: Gate[]
}
type StepKey = "site" | "channels" | "points" | "model" | "rules" | "commission" | "preflight"
type Tab = "gates" | "acceptance" | "versions" | "tools"
type VersionSummary = { id: string; site_id: string; site_name: string; created_at: string; operator: string; note?: string; preflight_status?: string; nodes: number }
type DemandWindow = { available: boolean; demand_kw: number; valid_bins: number; missing_bins?: string[] }
type PlantDemand = { plant_id: string; plant_name: string; meter_point_id?: string; upper_limit_kw?: number; window: DemandWindow; reason?: string }
type DemandResponse = { site: string; plants: PlantDemand[] }
type ProfileSummary = { id: string; vendor: string; model: string; points: number; commands: number; verified?: boolean }
type ApplyPoint = { name: string; point_id?: string; verdict: string; reason?: string }
type ApplyResult = { server: number; profile_id: string; mapped: number; exact: number; scaled: number; missing: string[]; blocked: string[]; points: ApplyPoint[] }
type ApplyResponse = { profile_id: string; dry_run: boolean; results: ApplyResult[]; applied: number }
type ReuseItem = { id: string; type: string; name: string; reason?: string }
type ReusePreview = { source: string; reuse: ReuseItem[]; change: ReuseItem[]; confirm: ReuseItem[]; counts: Record<string, number> }
type ReuseResult = { site_id: string; created: number; reuse: ReuseItem[]; change: ReuseItem[]; confirm: ReuseItem[]; counts: Record<string, number> }

const gateTones: Record<string, EmsTone> = { pass: "success", warn: "warning", fail: "danger" }
const gateNames: Record<string, string> = { pass: "通过", warn: "注意", fail: "阻断" }
const acceptanceTones: Record<string, EmsTone> = { implemented: "success", partial: "warning", unimplemented: "neutral" }
const acceptanceNames: Record<string, string> = { implemented: "已实现", partial: "部分", unimplemented: "未实现" }

const steps: { key: StepKey; label: string; page?: Page }[] = [
  { key: "site", label: "站点" },
  { key: "channels", label: "通道", page: "channels" },
  { key: "points", label: "点位与映射", page: "points" },
  { key: "model", label: "建模", page: "energyUnits" },
  { key: "rules", label: "策略与试算", page: "rules" },
  { key: "commission", label: "联调", page: "ledger" },
  { key: "preflight", label: "预检" },
]

const clock = (value?: string) => {
  if (!value) return "—"
  const time = new Date(value)
  return Number.isFinite(time.getTime()) ? time.toLocaleString("zh-CN", { hour12: false }) : "—"
}

export default function DeliveryWorkbench({ token, nodes, canConfigure, goPage, onError }: {
  token: string
  nodes: Node[]
  canConfigure: boolean
  goPage: (page: Page) => void
  onError: (message: string) => void
}) {
  const [gates, setGates] = useState<PreflightReport | null>(null)
  const [latest, setLatest] = useState<PreflightReport | null>(null)
  const [ledger, setLedger] = useState<{ verified: boolean }[]>([])
  const [tab, setTab] = useState<Tab>("gates")
  const [step, setStep] = useState<StepKey>("site")
  const [busy, setBusy] = useState(false)
  const [refreshedAt, setRefreshedAt] = useState<Date | null>(null)
  const [reuseSource, setReuseSource] = useState("")
  const [reuseName, setReuseName] = useState("")
  const [reusePreview, setReusePreview] = useState<ReusePreview | null>(null)
  const [reuseResult, setReuseResult] = useState<ReuseResult | null>(null)
  const [reuseBusy, setReuseBusy] = useState(false)
  const [versions, setVersions] = useState<VersionSummary[]>([])
  const [publishNote, setPublishNote] = useState("")
  const [versionBusy, setVersionBusy] = useState(false)
  const [versionMessage, setVersionMessage] = useState("")
  const [planSlots, setPlanSlots] = useState<(number | null)[]>(() => Array(96).fill(null))
  const [planLoaded, setPlanLoaded] = useState(false)
  const [planBusy, setPlanBusy] = useState(false)
  const [planMessage, setPlanMessage] = useState("")
  const [demand, setDemand] = useState<DemandResponse | null>(null)
  const [profiles, setProfiles] = useState<ProfileSummary[]>([])
  const [applyChannel, setApplyChannel] = useState("")
  const [applyProfileID, setApplyProfileID] = useState("")
  const [applyServers, setApplyServers] = useState("1")
  const [applyResult, setApplyResult] = useState<ApplyResponse | null>(null)
  const [applyBusy, setApplyBusy] = useState(false)
  const [applyMessage, setApplyMessage] = useState("")

  const loadGates = useCallback(async () => {
    try {
      setGates(await request<PreflightReport>(token, "/delivery/gates"))
      setRefreshedAt(new Date())
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 404) return
      onError(cause instanceof Error ? cause.message : "门槛读取失败")
    }
  }, [token, onError])

  const loadLatest = useCallback(async () => {
    try {
      setLatest(await request<PreflightReport>(token, "/delivery/preflight"))
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 404) {
        setLatest(null)
        return
      }
      onError(cause instanceof Error ? cause.message : "预检记录读取失败")
    }
  }, [token, onError])

  const loadLedger = useCallback(async () => {
    try {
      const result = await request<{ devices: { verified: boolean }[] }>(token, "/device-ledger")
      setLedger(result.devices || [])
    } catch {
      // 台账读取失败不阻塞工作台；步骤 6 会显示为未完成。
    }
  }, [token])

  useEffect(() => {
    void loadGates()
    void loadLatest()
    void loadLedger()
    const timer = window.setInterval(() => {
      void loadGates()
      void loadLedger()
    }, 10000)
    return () => window.clearInterval(timer)
  }, [loadGates, loadLatest, loadLedger])

  const runPreflight = useCallback(async () => {
    setBusy(true)
    try {
      const report = await request<PreflightReport>(token, "/delivery/preflight", "POST")
      setLatest(report)
      await loadGates()
      setTab("gates")
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "预检失败")
    } finally {
      setBusy(false)
    }
  }, [token, loadGates, onError])

  const sites = useMemo(() => nodes.filter((node) => node.type === "site"), [nodes])
  const channels = useMemo(() => nodes.filter((node) => node.type === "modbus"), [nodes])
  const currentSite = sites[0]?.id || ""

  const loadVersions = useCallback(async () => {
    if (!currentSite) return
    try {
      const result = await request<{ versions: VersionSummary[] }>(token, `/projects/versions?site=${encodeURIComponent(currentSite)}`)
      setVersions(result.versions || [])
    } catch {
      // 没有版本或读取失败时保持空列表，不阻塞工作台。
    }
  }, [token, currentSite])

  useEffect(() => {
    void loadVersions()
  }, [loadVersions])

  const publishProject = useCallback(async () => {
    if (!currentSite) return
    setVersionBusy(true)
    try {
      const published = await request<VersionSummary>(token, "/projects/publish", "POST", { site: currentSite, note: publishNote.trim() })
      setVersionMessage(`已发布 ${published.id}（${published.nodes} 个节点）`)
      setPublishNote("")
      await loadVersions()
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "发布失败")
    } finally {
      setVersionBusy(false)
    }
  }, [token, currentSite, publishNote, loadVersions, onError])

  const rollbackProject = useCallback(async (version: string) => {
    if (!currentSite) return
    setVersionBusy(true)
    try {
      const result = await request<{ version: string; updated: number; created: number; extra: number }>(token, "/projects/rollback", "POST", { site: currentSite, version })
      setVersionMessage(`已回退到 ${result.version}：更新 ${result.updated}、创建 ${result.created}、保留多余 ${result.extra}；历史未回滚`)
      await loadVersions()
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "回退失败")
    } finally {
      setVersionBusy(false)
    }
  }, [token, currentSite, loadVersions, onError])

  useEffect(() => {
    if (step !== "rules" || !currentSite) return
    void (async () => {
      try {
        const plan = await request<{ slots: (number | null)[] }>(token, `/projects/plan96?site=${encodeURIComponent(currentSite)}`)
        setPlanSlots(Array.isArray(plan.slots) && plan.slots.length === 96 ? plan.slots : Array(96).fill(null))
        setPlanLoaded(true)
      } catch {
        setPlanLoaded(false)
      }
      try {
        setDemand(await request<DemandResponse>(token, `/demand?site=${encodeURIComponent(currentSite)}`))
      } catch {
        setDemand(null)
      }
    })()
  }, [step, currentSite, token])

  useEffect(() => {
    if (step !== "points") return
    if (!applyChannel && channels.length > 0) setApplyChannel(channels[0].id)
    if (profiles.length > 0) return
    void (async () => {
      try {
        const result = await request<{ profiles: ProfileSummary[] }>(token, "/ems/profiles")
        setProfiles(result.profiles || [])
      } catch {
        setProfiles([])
      }
    })()
  }, [step, applyChannel, channels, profiles.length, token])

  const parseServers = (text: string): number[] => {
    const servers: number[] = []
    for (const part of text.split(",")) {
      const match = part.trim().match(/^(\d+)(?:-(\d+))?$/)
      if (!match) continue
      const start = Number(match[1])
      const end = match[2] ? Number(match[2]) : start
      for (let server = Math.min(start, end); server <= Math.max(start, end) && servers.length < 247; server++) servers.push(server)
    }
    return Array.from(new Set(servers))
  }

  const runApply = useCallback(async (dryRun: boolean) => {
    const servers = parseServers(applyServers)
    if (!applyChannel || !applyProfileID || servers.length === 0) {
      setApplyMessage("先选择通道、Profile，并填写站号（例如 1-4,6）")
      return
    }
    setApplyBusy(true)
    try {
      const result = await request<ApplyResponse>(token, `/modbus/${encodeURIComponent(applyChannel)}/units/apply`, "POST", {
        profile_id: applyProfileID, servers, dry_run: dryRun,
      })
      setApplyResult(result)
      setApplyMessage(dryRun ? "预览完成；确认无误后再点应用" : `已应用到 ${result.applied} 个站号`)
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "Profile 应用失败")
    } finally {
      setApplyBusy(false)
    }
  }, [token, applyChannel, applyProfileID, applyServers, onError])

  const savePlan = useCallback(async () => {
    if (!currentSite) return
    setPlanBusy(true)
    try {
      await request(token, "/projects/plan96", "PUT", { site: currentSite, slots: planSlots })
      setPlanMessage("计划已保存")
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "计划保存失败")
    } finally {
      setPlanBusy(false)
    }
  }, [token, currentSite, planSlots, onError])

  const loadReusePreview = useCallback(async () => {
    if (!reuseSource) return
    setReuseBusy(true)
    try {
      setReusePreview(await request<ReusePreview>(token, `/projects/reuse/preview?source=${encodeURIComponent(reuseSource)}`))
      setReuseResult(null)
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "复用预览失败")
    } finally {
      setReuseBusy(false)
    }
  }, [token, reuseSource, onError])

  const createFromSource = useCallback(async () => {
    if (!reuseSource || !reuseName.trim()) return
    setReuseBusy(true)
    try {
      const result = await request<ReuseResult>(token, "/projects/reuse", "POST", { source: reuseSource, name: reuseName.trim() })
      setReuseResult(result)
      setReusePreview(null)
      setReuseName("")
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "创建新站失败")
    } finally {
      setReuseBusy(false)
    }
  }, [token, reuseSource, reuseName, onError])

  const counts = useMemo(() => {
    const units = nodes.filter((node) => node.type === "energyUnit").length
    const rules = nodes.filter((node) => node.type === "decisionTable")
    const enabledRules = rules.filter((node) => (point(node, "enabled")?.value || 0) !== 0).length
    const points = nodes.filter((node) => node.type === "variable" || node.type === "modbusIo").length
    const channels = nodes.filter((node) => node.type === "modbus").length
    const verified = ledger.filter((device) => device.verified).length
    return { units, rules: rules.length, enabledRules, points, channels, devices: ledger.length, verified }
  }, [nodes, ledger])

  const gateStatus = useCallback((key: string) => gates?.gates.find((gate) => gate.key === key)?.status || "warn", [gates])

  const stepState = useMemo(() => {
    const done: Record<StepKey, boolean> = {
      site: true,
      channels: gateStatus("channels") !== "fail",
      points: gateStatus("points") !== "fail",
      model: counts.units > 0,
      rules: gateStatus("rules") !== "fail",
      commission: counts.verified > 0,
      preflight: latest?.status === "pass",
    }
    const current = steps.find((item) => !done[item.key])?.key || "preflight"
    return { done, current }
  }, [counts, gateStatus, latest])

  const current = steps.find((item) => item.key === step) || steps[0]

  const summary = () => {
    switch (step) {
      case "channels":
        return { text: `${counts.channels} 条通道`, detail: gates?.gates.find((gate) => gate.key === "channels")?.detail || "读取中…" }
      case "points":
        return { text: `${counts.points} 个点位`, detail: gates?.gates.find((gate) => gate.key === "points")?.detail || "读取中…" }
      case "model":
        return { text: `${counts.units} 个 EnergyUnit`, detail: "站点 → 电站 → 资源集群 → 能源单元；调度以最小受控单元为单位" }
      case "rules":
        return { text: `${counts.rules} 条决策表 · 启用 ${counts.enabledRules}`, detail: "决策表、场景试算与硬优先级仲裁" }
      case "commission":
        return { text: `${counts.devices} 台设备 · 已验证 ${counts.verified}`, detail: "逐台核对点位映射与回读，验证记录进入台账" }
      case "preflight":
        return { text: latest ? `${gateNames[latest.status] || latest.status} · ${clock(latest.generated_at)}` : "尚未预检", detail: latest ? `操作者 ${latest.operator || "—"} · 快照 ${latest.id}` : "运行一次预检，生成可追溯的投运快照" }
      default:
        return { text: `${counts.units} 个单元 · ${counts.points} 个点位`, detail: "站点、电站、资源集群、能源单元与设备的统一模型" }
    }
  }
  const currentSummary = summary()

  const acceptanceCounts = useMemo(() => {
    const result: Record<string, number> = { implemented: 0, partial: 0, unimplemented: 0 }
    for (const row of acceptance.rows) result[row.status] = (result[row.status] || 0) + 1
    return result
  }, [])

  return <section className="grid gap-3">
    <div className="grid items-start gap-4 lg:grid-cols-[232px_1fr]">
      <div className="grid gap-1 rounded-xl border border-ems-line bg-white/60 p-2">
        {steps.map((item, index) => {
          const done = stepState.done[item.key]
          const isCurrent = stepState.current === item.key
          return <button key={item.key} onClick={() => setStep(item.key)}
            className={`flex items-center gap-2 rounded-lg px-2 py-2 text-left text-sm ${step === item.key ? "bg-white shadow-sm" : "hover:bg-white/70"}`}>
            <span className={`grid h-6 w-6 flex-none place-items-center rounded-full border text-xs ${done ? "border-emerald-500 text-emerald-600" : isCurrent ? "border-sky-500 text-sky-600" : "border-slate-300 text-slate-400"}`}>
              {done ? "✓" : index + 1}
            </span>
            <span className="min-w-0 flex-1 truncate">{item.label}</span>
            {isCurrent && !done && <span className="text-[10px] text-sky-600">进行中</span>}
          </button>
        })}
        <p className="m-0 px-2 pt-1 text-[10px] leading-4 text-ems-muted">状态从数据推导：有通道、有点位、有单元、有启用规则、有验证设备、最近预检通过。</p>
      </div>

      <div className="grid gap-3">
        <EmsCard>
          <EmsSectionHeader kicker="当前步骤" title={current.label} description={currentSummary.detail}
            status={<span className="text-xl font-semibold">{currentSummary.text}</span>}
            actions={current.page ? <EmsButton variant="secondary" onClick={() => goPage(current.page as Page)}>打开{steps.find((item) => item.key === step)?.label}页 →</EmsButton> : undefined} />
        </EmsCard>

        {step === "site" && <EmsCard>
          <div className="grid gap-3">
            <EmsSectionHeader kicker="项目复用" title="从已交付项目创建新站"
              description="复用点表、Profile、规则与验收配置；地址、凭据与控制授权不带入，跨节点引用需要重新绑定。" />
            <div className="grid gap-2 md:grid-cols-[1.2fr_1fr_auto_auto] md:items-end">
              <label className="grid gap-1 text-xs text-ems-muted">源项目
                <select className="ems-input" aria-label="源项目" value={reuseSource} onChange={(event) => { setReuseSource(event.target.value); setReusePreview(null); setReuseResult(null) }}>
                  <option value="">选择已交付站点</option>
                  {sites.map((site) => <option key={site.id} value={site.id}>{nameOf(site)}</option>)}
                </select>
              </label>
              <label className="grid gap-1 text-xs text-ems-muted">新站名称
                <input className="ems-input" aria-label="新站名称" value={reuseName} placeholder="例如 苏州二期" onChange={(event) => setReuseName(event.target.value)} />
              </label>
              <EmsButton variant="secondary" busy={reuseBusy} disabled={!reuseSource} onClick={() => void loadReusePreview()}>预览复用</EmsButton>
              <EmsButton busy={reuseBusy} disabled={!reusePreview || !reuseName.trim()} onClick={() => void createFromSource()}>创建新站</EmsButton>
            </div>
            {reusePreview && <div className="grid gap-2 text-xs text-ems-muted">
              <div className="flex flex-wrap gap-2">
                <StatusBadge tone="success">复用 {reusePreview.counts.reuse || 0}</StatusBadge>
                <StatusBadge tone="warning">变化 {reusePreview.counts.change || 0}</StatusBadge>
                <StatusBadge tone="neutral">待确认 {reusePreview.counts.confirm || 0}</StatusBadge>
              </div>
              {(reusePreview.change || []).length > 0 && <div>需要填写：{(reusePreview.change || []).map((item) => `${item.name}（${item.reason}）`).join("、")}</div>}
              {(reusePreview.confirm || []).length > 0 && <div>需要确认：{(reusePreview.confirm || []).slice(0, 6).map((item) => `${item.name}（${item.reason}）`).join("、")}{(reusePreview.confirm || []).length > 6 ? " …" : ""}</div>}
            </div>}
            {reuseResult && <InlineNotice tone="success">
              已创建 {reuseResult.created} 个节点；复用 {reuseResult.counts.reuse || 0} 项，变化 {reuseResult.counts.change || 0} 项，待确认 {reuseResult.counts.confirm || 0} 项。地址与引用请在对应步骤重新填写。
            </InlineNotice>}
          </div>
        </EmsCard>}

        {step === "points" && <EmsCard>
          <div className="grid gap-3">
            <EmsSectionHeader kicker="换厂商" title="按 Profile 批量重映射"
              description="按逻辑点名称自动匹配物理点位；缩放不一致标为 scaled，缺失点位列入待确认并阻断相关控制，不会用默认点冒充。" />
            <div className="grid gap-2 md:grid-cols-[1fr_1fr_170px_auto_auto] md:items-end">
              <label className="grid gap-1 text-xs text-ems-muted">通道
                <select className="ems-input" aria-label="Profile 通道" value={applyChannel} onChange={(event) => { setApplyChannel(event.target.value); setApplyResult(null) }}>
                  <option value="">选择通道</option>
                  {channels.map((channel) => <option key={channel.id} value={channel.id}>{nameOf(channel)}</option>)}
                </select>
              </label>
              <label className="grid gap-1 text-xs text-ems-muted">Profile
                <select className="ems-input" aria-label="目标 Profile" value={applyProfileID} onChange={(event) => { setApplyProfileID(event.target.value); setApplyResult(null) }}>
                  <option value="">选择 Profile</option>
                  {profiles.map((item) => <option key={item.id} value={item.id}>{item.vendor} {item.model}</option>)}
                </select>
              </label>
              <label className="grid gap-1 text-xs text-ems-muted">站号
                <input className="ems-input" aria-label="站号范围" value={applyServers} placeholder="1-4,6" onChange={(event) => { setApplyServers(event.target.value); setApplyResult(null) }} />
              </label>
              <EmsButton variant="secondary" busy={applyBusy} onClick={() => void runApply(true)}>预览</EmsButton>
              <EmsButton busy={applyBusy} disabled={!applyResult || !applyResult.dry_run} onClick={() => void runApply(false)}>应用</EmsButton>
            </div>
            {applyResult
              ? <div className="grid gap-2 text-xs">
                {applyResult.results.map((result) => <div key={result.server} className="rounded-lg border border-ems-line px-3 py-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <strong>站号 {result.server}</strong>
                    <span className="text-ems-muted">映射 {result.mapped} · 精确 {result.exact} · 缩放 {result.scaled} · 缺失 {result.missing.length}</span>
                  </div>
                  {result.missing.length > 0 && <div className="text-ems-amber">缺失：{result.missing.join("、")}</div>}
                  {result.blocked.length > 0 && <ul className="m-0 mt-1 list-disc pl-4 text-ems-amber">{result.blocked.map((item) => <li key={item}>{item}</li>)}</ul>}
                </div>)}
                {applyMessage && <InlineNotice tone="info">{applyMessage}</InlineNotice>}
              </div>
              : applyMessage ? <InlineNotice tone="warning">{applyMessage}</InlineNotice> : null}
          </div>
        </EmsCard>}

        {step === "rules" && <EmsCard>
          <div className="grid gap-3">
            <EmsSectionHeader kicker="策略与试算" title="96 点计划与 15 分钟需量"
              description="每个 15 分钟时段一个目标（正=放电，负=充电）；留空表示该时段无计划，不回落到 0。需量按 (t-15min, t] 的 15 个一分钟 bin 计算，缺 bin 时不做预测性削峰。" />
            <div className="grid gap-2">
              {(demand?.plants || []).map((plant) => <div key={plant.plant_id} className="rounded-lg border border-ems-line px-3 py-2 text-xs">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <strong>{plant.plant_name}</strong>
                  <span className="text-ems-muted">{plant.reason
                    ? plant.reason
                    : `需量 ${plant.window.demand_kw.toFixed(1)} kW · 上限 ${plant.upper_limit_kw || "未设"} · ${plant.window.valid_bins}/15 bin`}</span>
                </div>
                {!plant.window.available && plant.window.missing_bins && plant.window.missing_bins.length > 0 && <span className="text-ems-amber">缺样：{plant.window.missing_bins.join("、")}</span>}
              </div>)}
              {demand && demand.plants.length === 0 && <p className="m-0 text-sm text-ems-muted">该站点还没有电站或 PCC 反馈。</p>}
            </div>
            <div className="grid gap-1">
              {[0, 1, 2, 3].map((row) => <div key={row} className="grid grid-cols-[42px_1fr] items-center gap-2">
                <span className="text-[10px] text-ems-muted">{String(row * 15).padStart(2, "0")} 分</span>
                <div className="grid gap-1" style={{ gridTemplateColumns: "repeat(24, minmax(0, 1fr))" }}>
                  {Array.from({ length: 24 }, (_, hour) => {
                    const index = hour * 4 + row
                    const value = planSlots[index]
                    return <input key={index} className="ems-input w-full px-1 text-center text-[10px]" inputMode="decimal"
                      aria-label={`计划时段 ${String(hour).padStart(2, "0")}:${String(row * 15).padStart(2, "0")}`}
                      value={value === null ? "" : String(value)} placeholder="—"
                      onChange={(event) => {
                        const text = event.target.value.trim()
                        setPlanSlots((prev) => {
                          const next = [...prev]
                          next[index] = text === "" ? null : Number(text)
                          return next
                        })
                      }} />
                  })}
                </div>
              </div>)}
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-xs text-ems-muted">{planMessage || (planLoaded ? "计划已加载；留空表示该时段无计划" : "正在读取计划…")}</span>
              <EmsButton busy={planBusy} disabled={!currentSite} onClick={() => void savePlan()}>保存 96 点计划</EmsButton>
            </div>
          </div>
        </EmsCard>}

        <SegmentedTabs value={tab} ariaLabel="交付工作台视图" onChange={(value) => setTab(value)}
          options={[{ value: "gates", label: "投运门槛" }, { value: "acceptance", label: "验收进度" }, { value: "versions", label: "版本与发布" }, { value: "tools", label: "接入工具" }]} />

        {tab === "gates" && <EmsCard>
          <div className="grid gap-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-xs text-ems-muted">
                实时状态每 10 秒刷新{refreshedAt ? ` · 更新于 ${clock(refreshedAt.toISOString())}` : ""}
                {latest && <> · 最近预检 {clock(latest.generated_at)}（{latest.operator || "—"}，{gateNames[latest.status] || latest.status}）</>}
              </div>
              <EmsButton busy={busy} busyLabel="预检中…" disabled={!canConfigure} onClick={() => void runPreflight()}>运行预检</EmsButton>
            </div>
            {!gates ? <p className="m-0 text-sm text-ems-muted">正在读取门槛…</p> : <div className="grid gap-2">
              {gates.gates.map((gate) => <div key={gate.key} className="flex flex-wrap items-start justify-between gap-3 rounded-lg border border-ems-line px-3 py-2">
                <div className="min-w-0">
                  <strong className="block text-sm">{gate.title}</strong>
                  <span className="text-xs text-ems-muted">{gate.detail}{gate.object ? ` · ${gate.object}` : ""}</span>
                </div>
                <div className="flex items-center gap-3 text-xs text-ems-muted">
                  {gate.next_step && <span>下一步：{gate.next_step}</span>}
                  <StatusBadge tone={gateTones[gate.status] || "neutral"}>{gateNames[gate.status] || gate.status}</StatusBadge>
                </div>
              </div>)}
            </div>}
            <p className="m-0 text-xs text-ems-muted">预检不改变发布行为；快照写入 data/preflight.jsonl 与审计，用于交接与复查。</p>
          </div>
        </EmsCard>}

        {tab === "acceptance" && <EmsCard>
          <div className="grid gap-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-xs text-ems-muted">状态以 <code>src/data/acceptance-status.json</code> 为准 · 更新于 {acceptance.updated_at} · 证据见 docs/acceptance-evidence.md</div>
              <div className="flex gap-2 text-xs">
                <StatusBadge tone="success">已实现 {acceptanceCounts.implemented}</StatusBadge>
                <StatusBadge tone="warning">部分 {acceptanceCounts.partial}</StatusBadge>
                <StatusBadge tone="neutral">未实现 {acceptanceCounts.unimplemented}</StatusBadge>
              </div>
            </div>
            <div className="overflow-x-auto">
              <table className="ems-table w-full text-sm">
                <thead><tr><th>ID</th><th>验收项</th><th>状态</th><th>摘要与缺口</th><th>证据</th></tr></thead>
                <tbody>{acceptance.rows.map((row) => <tr key={row.id}>
                  <td className="whitespace-nowrap font-semibold">{row.id}</td>
                  <td className="whitespace-nowrap">{row.title}</td>
                  <td><StatusBadge tone={acceptanceTones[row.status] || "neutral"}>{acceptanceNames[row.status] || row.status}</StatusBadge></td>
                  <td><span>{row.summary}</span>{row.gap && <small className="block text-ems-muted">缺口：{row.gap}</small>}</td>
                  <td className="text-xs text-ems-muted">{row.evidence.length ? row.evidence.join("、") : "—"}</td>
                </tr>)}</tbody>
              </table>
            </div>
            <p className="m-0 text-xs text-ems-muted">未实现项单独计数，不参与已实现比例；A 行进度是验收证据，不等于运行时门槛。</p>
          </div>
        </EmsCard>}

        {tab === "versions" && <EmsCard>
          <div className="grid gap-3">
            <EmsSectionHeader kicker="版本与发布" title="冻结当前工程配置"
              description="发布只写版本文件与审计，不改变运行中的配置；回退只重放工程配置，历史（审计、报警、命令）不回滚，也不回放旧命令。" />
            <div className="grid gap-2 md:grid-cols-[1fr_auto] md:items-end">
              <label className="grid gap-1 text-xs text-ems-muted">发布说明（可选）
                <input className="ems-input" aria-label="发布说明" value={publishNote} placeholder="例如 首次交付 / 更换 PCS 前" onChange={(event) => setPublishNote(event.target.value)} />
              </label>
              <EmsButton busy={versionBusy} disabled={!currentSite} onClick={() => void publishProject()}>发布当前配置</EmsButton>
            </div>
            {versionMessage && <InlineNotice tone="info">{versionMessage}</InlineNotice>}
            {versions.length === 0
              ? <p className="m-0 text-sm text-ems-muted">还没有版本。发布后会出现在这里，可回退到任意历史版本。</p>
              : <div className="grid gap-2">
                {versions.map((version) => <div key={version.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-ems-line px-3 py-2 text-xs">
                  <div className="min-w-0">
                    <strong className="block text-sm">{clock(version.created_at)} · {version.operator}</strong>
                    <span className="text-ems-muted">{version.id} · {version.nodes} 个节点{version.note ? ` · ${version.note}` : ""}{version.preflight_status ? ` · 预检 ${gateNames[version.preflight_status] || version.preflight_status}` : ""}</span>
                  </div>
                  <EmsButton variant="secondary" busy={versionBusy} onClick={() => void rollbackProject(version.id)}>回退到此版本</EmsButton>
                </div>)}
              </div>}
            <p className="m-0 text-xs text-ems-muted">回退前会自动冻结当前配置，回退失败或需要反悔时仍可找回。</p>
          </div>
        </EmsCard>}

        {tab === "tools" && <EmsCard>
          <DeployView token={token} isAdmin={canConfigure} goDelivery={() => undefined} embedded />
        </EmsCard>}
      </div>
    </div>
  </section>
}
