import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSectionHeader, EmptyState, InlineNotice, StatusBadge } from "./components/ems"
import { createResourceGroup, dispatchResourceGroup, fetchEnergyUnits, fetchResourceGroups, type EnergyUnitSummary, type GroupDispatchResult, type ResourceGroup } from "./api"

// 资源集群（EMS 业务层）：一组能源设备共享馈线/功率上限，分配时先受组上限约束。
export default function ResourceGroupsView({ token, onError }: { token: string; onError: (message: string) => void }) {
  const [groups, setGroups] = useState<ResourceGroup[]>([])
  const [units, setUnits] = useState<EnergyUnitSummary[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [name, setName] = useState("")
  const [limit, setLimit] = useState(0)
  const [members, setMembers] = useState<string[]>([])
  const [request, setRequest] = useState(0)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ groupId: string; data: GroupDispatchResult } | null>(null)

  function refresh() {
    void fetchResourceGroups(token).then((r) => setGroups(r.groups || [])).catch(() => onError("资源集群读取失败"))
    void fetchEnergyUnits(token).then((r) => setUnits(r.units || [])).catch(() => {})
  }
  useEffect(refresh, [token])

  const unitName = (id: string) => units.find((unit) => unit.id === id)?.description || units.find((unit) => unit.id === id)?.asset_code || id

  async function add() {
    if (!name.trim()) { onError("请填写集群名称"); return }
    if (members.length === 0) { onError("请至少选择一个能源设备"); return }
    setBusy(true)
    try {
      await createResourceGroup(token, { description: name.trim(), shared_limit_kw: limit, members })
      setName(""); setLimit(0); setMembers([]); setShowAdd(false)
      refresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "创建失败")
    } finally {
      setBusy(false)
    }
  }

  async function dispatch(groupId: string) {
    setBusy(true)
    try {
      const data = await dispatchResourceGroup(token, groupId, request)
      setResult({ groupId, data })
    } catch (error) {
      onError(error instanceof Error ? error.message : "分配失败")
    } finally {
      setBusy(false)
    }
  }

  return <section className="grid gap-4">
    <EmsSectionHeader kicker="EMS 业务" title="资源集群" description="一组能源设备共享馈线或功率上限；分配先受组上限约束，缺口如实上报。"
      actions={<EmsButton variant="secondary" onClick={() => setShowAdd(!showAdd)}>{showAdd ? "收起" : "+ 新建集群"}</EmsButton>} />

    {showAdd && <EmsCard variant="subtle"><IonCardContent className="grid gap-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <EmsInput label="集群名称" aria-label="集群名称" value={name} onIonInput={(event) => setName(event.detail.value || "")} placeholder="例如：1 号馈线储能组" />
        <EmsInput label="共享功率上限（kW，0=不限制）" aria-label="共享功率上限" type="number" value={limit} onIonInput={(event) => setLimit(Number(event.detail.value))} />
      </div>
      <div className="grid gap-2">
        <strong className="text-sm text-ems-ink">成员</strong>
        {units.length === 0 ? <p className="m-0 text-xs text-ems-muted">还没有能源设备，请先在“能源设备”创建。</p> : <div className="grid gap-1 sm:grid-cols-2">
          {units.map((unit) => <label key={unit.id} className="flex items-center gap-2 rounded-lg border border-ems-line px-3 py-2 text-sm">
            <input className="h-4 w-4 accent-ems-accent" type="checkbox" checked={members.includes(unit.id)} onChange={() => setMembers(members.includes(unit.id) ? members.filter((id) => id !== unit.id) : [...members, unit.id])} />
            {unit.description || unit.asset_code || unit.id}
          </label>)}
        </div>}
      </div>
      <div className="flex justify-end"><EmsButton busy={busy} busyLabel="创建中…" onClick={() => void add()}>创建集群</EmsButton></div>
    </IonCardContent></EmsCard>}

    {groups.length === 0 ? <EmptyState title="还没有资源集群" description="当多台设备共用馈线或功率配额时，建一个集群并设置共享上限。" /> : <div className="grid gap-3">
      {groups.map((group) => <EmsCard key={group.id}><IonCardContent className="grid gap-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div><strong className="text-sm text-ems-ink">{group.description || group.id}</strong><p className="m-0 text-xs text-ems-muted">{group.members.length} 台成员</p></div>
          <StatusBadge tone={group.shared_limit_kw > 0 ? "info" : "neutral"}>{group.shared_limit_kw > 0 ? `共享上限 ${group.shared_limit_kw} kW` : "无共享上限"}</StatusBadge>
        </div>
        <div className="flex flex-wrap gap-2 text-xs text-ems-muted">{group.members.map((id) => <span key={id} className="rounded-md bg-black/5 px-2 py-1">{unitName(id)}</span>)}</div>
        <div className="flex flex-wrap items-end gap-3 border-t border-ems-line pt-3">
          <EmsInput label="请求功率（kW，负=充电）" aria-label="请求功率" type="number" value={request} onIonInput={(event) => setRequest(Number(event.detail.value))} />
          <EmsButton busy={busy} busyLabel="分配中…" onClick={() => void dispatch(group.id)}>试分配</EmsButton>
        </div>
        {result?.groupId === group.id && <div className="grid gap-2">
          <InlineNotice tone={result.data.shortfall_kw > 0 ? "warning" : "success"}>
            合计下发 {result.data.delivered_kw} kW，缺口 {result.data.shortfall_kw} kW{result.data.limited_by ? `（受集团上限限制）` : ""}
          </InlineNotice>
          <div className="grid gap-1">{result.data.allocations.map((item) => <div key={item.energy_unit_id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-ems-line px-3 py-2 text-xs">
            <strong>{unitName(item.energy_unit_id)}</strong>
            <span>{item.excluded ? `未分配：${item.reason || "能力不足"}` : `${item.setpoint_kw} kW`}</span>
          </div>)}</div>
        </div>}
      </IonCardContent></EmsCard>)}
    </div>}
  </section>
}
