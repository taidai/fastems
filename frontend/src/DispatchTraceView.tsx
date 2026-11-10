import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsRecordList, EmsSectionHeader, EmsSelect, EmsSelectOption, EmptyState, MetricStrip, StatusBadge } from "./components/ems"
import { fetchDispatchRecords, fetchEnergyUnits, type DispatchRecord, type EnergyUnitSummary } from "./api"

const sourceLabel: Record<string, string> = {
  anti_reverse: "防逆流", demand: "需量", peak_valley: "削峰填谷", schedule: "单柜规则", group: "资源集群", manual: "人工", site: "站级计划",
}
const stageLabel: Record<string, string> = {
  submitted: "已提交指令", valueSet_written: "已写设定", fallback: "安全回退",
}

// 调度追溯（运行层）：按设备查看每次调度的 dispatchID、来源、请求/下发/缺口与设备拆解，
// 回答“当时谁赢了、为什么没做到”。
export default function DispatchTraceView({ token, onError }: { token: string; onError: (message: string) => void }) {
  const [units, setUnits] = useState<EnergyUnitSummary[]>([])
  const [unitId, setUnitId] = useState("")
  const [records, setRecords] = useState<DispatchRecord[]>([])
  const [selected, setSelected] = useState<DispatchRecord | null>(null)

  useEffect(() => {
    void fetchEnergyUnits(token).then((result) => {
      setUnits(result.units || [])
      if (result.units?.[0]) setUnitId(result.units[0].id)
    }).catch(() => onError("能源设备读取失败"))
  }, [token, onError])

  useEffect(() => {
    if (!unitId) return
    void fetchDispatchRecords(token, unitId).then((result) => setRecords(result.records || [])).catch(() => onError("调度记录读取失败"))
  }, [token, unitId, onError])

  const last = records[0]
  return <section className="grid gap-4">
    <EmsSectionHeader kicker="运行" title="调度追溯" description="每次调度都带 dispatchID：请求、能力快照、分配与设备设定可复盘。"
      actions={<EmsSelect aria-label="能源设备" value={unitId} onIonChange={(event) => setUnitId(event.detail.value)}>
        {units.map((unit) => <EmsSelectOption key={unit.id} value={unit.id}>{unit.description || unit.asset_code || unit.id}</EmsSelectOption>)}
      </EmsSelect>} />

    <MetricStrip items={[
      { label: "记录条数", value: records.length },
      { label: "最近请求", value: last ? `${last.request_kw} kW` : "—" },
      { label: "最近下发", value: last ? `${last.delivered_kw} kW` : "—" },
      { label: "最近缺口", value: last ? `${last.shortfall_kw} kW` : "—", tone: last && last.shortfall_kw > 0 ? "warning" : "neutral" },
    ]} />

    <EmsRecordList rows={records} keyOf={(record) => record.dispatch_id} onRowClick={setSelected}
      empty={<EmptyState title="还没有调度记录" description="发布策略并接入设备后，每秒调度都会留下一条可复盘记录。" />}
      columns={[
        { key: "at", label: "时间", render: (record) => new Date(record.at).toLocaleString("zh-CN", { hour12: false }) },
        { key: "id", label: "dispatchID", render: (record) => <code className="break-all text-xs">{record.dispatch_id}</code> },
        { key: "state", label: "柜体状态", render: (record) => <StatusBadge tone={record.cabinet_state === "running" ? "success" : "warning"}>{record.cabinet_state}</StatusBadge> },
        { key: "source", label: "来源", render: (record) => sourceLabel[record.source || ""] || record.source || "—" },
        { key: "request", label: "请求", render: (record) => `${record.request_kw} kW` },
        { key: "delivered", label: "下发", render: (record) => `${record.delivered_kw} kW` },
        { key: "shortfall", label: "缺口", render: (record) => record.shortfall_kw > 0 ? <span className="text-ems-amber">{record.shortfall_kw} kW</span> : "0" },
      ]}
      mobileCard={(record) => <div className="grid gap-1">
        <div className="flex items-center justify-between gap-2"><strong>{new Date(record.at).toLocaleTimeString("zh-CN", { hour12: false })}</strong><StatusBadge tone={record.shortfall_kw > 0 ? "warning" : "success"}>{record.shortfall_kw > 0 ? `缺口 ${record.shortfall_kw} kW` : "已下发"}</StatusBadge></div>
        <span className="text-xs text-ems-muted">请求 {record.request_kw} kW · 下发 {record.delivered_kw} kW</span>
      </div>}
    />

    {selected && <EmsCard><IonCardContent className="grid gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2"><strong className="text-sm text-ems-ink">{selected.dispatch_id}</strong><EmsButton variant="secondary" size="small" onClick={() => setSelected(null)}>关闭</EmsButton></div>
      <p className="m-0 text-xs text-ems-muted">{new Date(selected.at).toLocaleString("zh-CN", { hour12: false })} · 来源 {sourceLabel[selected.source || ""] || selected.source || "—"}
        {selected.priority !== undefined ? `（优先级 ${selected.priority}）` : ""} · 规则 {selected.rule_id || "—"} · 柜体 {selected.cabinet_state}
        {selected.stage ? ` · ${stageLabel[selected.stage] || selected.stage}` : ""}{selected.actor ? ` · 操作者 ${selected.actor}` : ""}</p>
      <div className="grid gap-2">{(selected.dispatches || []).map((item) => <div key={item.device_id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-ems-line px-3 py-2 text-xs">
        <strong>{item.device_id}</strong><span>{item.excluded ? `未下发：${item.reason || "能力不足"}` : `${item.setpoint_kw} kW`}</span>
      </div>)}</div>
    </IonCardContent></EmsCard>}
  </section>
}
