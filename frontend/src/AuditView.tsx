import { useCallback, useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsOverlay, EmsRecordList, EmsSectionHeader, EmsSelect, EmsSelectOption, EmptyState, FilterToolbar, StatusBadge } from "./components/ems"
import { downloadCSV, request } from "./api"
import type { AuditRecord } from "./generated/contract"

type Filters = { actor: string; action: string; result: string; from: string; to: string }
const emptyFilters: Filters = { actor: "", action: "", result: "", from: "", to: "" }

function queryString(filters: Filters) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value) params.set(key, value)
  }
  params.set("limit", "200")
  return params.toString()
}

type AuditSetting = { max_mb: number; backups: number; retention_days: number }

export default function AuditView({ token, onError }: { token: string; onError: (message: string) => void }) {
  const [auditSetting, setAuditSetting] = useState<AuditSetting>({ max_mb: 5, backups: 1, retention_days: 0 })
  const [settingBusy, setSettingBusy] = useState(false)
  useEffect(() => {
    void request<AuditSetting>(token, "/audit-settings").then(setAuditSetting).catch(() => {})
  }, [token])
  async function saveAuditSetting() {
    setSettingBusy(true)
    try {
      await request<AuditSetting>(token, "/audit-settings", "PUT", auditSetting)
    } catch (error) {
      onError(error instanceof Error ? error.message : "保存失败")
    } finally {
      setSettingBusy(false)
    }
  }
  const [filters, setFilters] = useState<Filters>(emptyFilters)
  const [draft, setDraft] = useState<Filters>(emptyFilters)
  const [records, setRecords] = useState<AuditRecord[]>([])
  const [actions, setActions] = useState<string[]>([])
  const [selected, setSelected] = useState<AuditRecord | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async (next: Filters) => {
    setLoading(true)
    try {
      const result = await request<{ records: AuditRecord[]; actions: string[] }>(token, `/audit?${queryString(next)}`)
      setRecords(result.records || [])
      setActions(result.actions || [])
      setSelected(null)
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "审计日志读取失败")
    } finally {
      setLoading(false)
    }
  }, [token, onError])

  useEffect(() => { void load(emptyFilters) }, [load])

  return <section className="grid gap-4">
    <EmsSectionHeader kicker="只读留痕" title="审计日志" description="登录、配置、控制、报警与账号操作；不记录密码、令牌或请求体。"
      actions={<EmsButton variant="secondary" busy={loading} busyLabel="读取中…" onClick={() => void load(filters)}>刷新</EmsButton>} />

    <EmsCard><IonCardContent>
      <FilterToolbar trailing={<>
        <EmsButton busy={loading} onClick={() => { setFilters(draft); void load(draft) }}>查询</EmsButton>
        <EmsButton variant="secondary" disabled={loading} onClick={() => { setDraft(emptyFilters); setFilters(emptyFilters); void load(emptyFilters) }}>重置</EmsButton>
        <EmsButton variant="secondary" disabled={loading} onClick={() => void downloadCSV(token, `/audit/export?${queryString(filters)}`, "easyems-audit.csv").catch((cause) => onError(cause instanceof Error ? cause.message : "导出失败"))}>导出 CSV</EmsButton>
      </>}>
        <EmsInput label="操作人" value={draft.actor} onIonInput={(event) => setDraft({ ...draft, actor: event.detail.value || "" })} placeholder="账号或用户 ID" />
        <EmsSelect label="动作" value={draft.action} onIonChange={(event) => setDraft({ ...draft, action: event.detail.value })}>
          <EmsSelectOption value="">全部</EmsSelectOption>
          {actions.map((action) => <EmsSelectOption key={action} value={action}>{action}</EmsSelectOption>)}
        </EmsSelect>
        <EmsSelect label="结果" value={draft.result} onIonChange={(event) => setDraft({ ...draft, result: event.detail.value })}>
          <EmsSelectOption value="">全部</EmsSelectOption><EmsSelectOption value="success">成功</EmsSelectOption><EmsSelectOption value="failure">失败</EmsSelectOption>
        </EmsSelect>
        <EmsInput label="开始日期" type="date" value={draft.from} onIonInput={(event) => setDraft({ ...draft, from: event.detail.value || "" })} />
        <EmsInput label="结束日期" type="date" value={draft.to} onIonInput={(event) => setDraft({ ...draft, to: event.detail.value || "" })} />
      </FilterToolbar>
    </IonCardContent></EmsCard>

    <EmsRecordList rows={records} keyOf={(record) => record.id} onRowClick={setSelected}
      empty={<EmptyState title="没有匹配的审计记录" description="登录或执行一次配置/控制后，这里会出现记录。" />}
      columns={[
        { key: "time", label: "时间", render: (record) => new Date(record.time).toLocaleString("zh-CN", { hour12: false }) },
        { key: "actor", label: "操作人", render: (record) => record.actor },
        { key: "action", label: "动作", render: (record) => record.action },
        { key: "object", label: "对象", render: (record) => record.object || "—" },
        { key: "result", label: "结果", render: (record) => <StatusBadge tone={record.result === "success" ? "success" : "danger"}>{record.result === "success" ? "成功" : "失败"}</StatusBadge> },
        { key: "ip", label: "IP", render: (record) => record.ip || "—" },
      ]}
      mobileCard={(record) => <button className="grid gap-2 text-left" onClick={() => setSelected(record)}>
        <div className="flex items-center justify-between gap-2"><strong>{record.action}</strong><StatusBadge tone={record.result === "success" ? "success" : "danger"}>{record.result === "success" ? "成功" : "失败"}</StatusBadge></div>
        <span className="text-xs text-ems-muted">{new Date(record.time).toLocaleString("zh-CN", { hour12: false })} · {record.actor}</span>
        <span className="text-xs text-ems-ink">{record.object || "—"}</span>
      </button>}
    />

    <EmsCard><IonCardContent className="grid gap-3">
      <EmsSectionHeader title="审计保留" description="轮换大小、备份个数与按天保留；修改后立即生效，查询与导出包含仍保留的轮换文件。" />
      <div className="grid gap-3 sm:grid-cols-3">
        <EmsInput label="单个文件上限（MB）" aria-label="审计文件上限" type="number" value={auditSetting.max_mb} onIonInput={(event) => setAuditSetting({ ...auditSetting, max_mb: Number(event.detail.value) })} />
        <EmsInput label="保留备份个数" aria-label="审计备份个数" type="number" value={auditSetting.backups} onIonInput={(event) => setAuditSetting({ ...auditSetting, backups: Number(event.detail.value) })} />
        <EmsInput label="保留天数（0=不按时间清理）" aria-label="审计保留天数" type="number" value={auditSetting.retention_days} onIonInput={(event) => setAuditSetting({ ...auditSetting, retention_days: Number(event.detail.value) })} />
      </div>
      <div className="flex justify-end"><EmsButton busy={settingBusy} busyLabel="保存中…" onClick={() => void saveAuditSetting()}>保存审计保留</EmsButton></div>
    </IonCardContent></EmsCard>

    <EmsOverlay open={Boolean(selected)} variant="drawer" title={selected?.action || "审计详情"} onClose={() => setSelected(null)}
      actions={<EmsButton variant="secondary" onClick={() => setSelected(null)}>关闭</EmsButton>}>
      {selected && <dl className="grid gap-4 text-sm">
        <div><dt className="text-xs text-ems-muted">时间</dt><dd>{new Date(selected.time).toLocaleString("zh-CN", { hour12: false })}</dd></div>
        <div><dt className="text-xs text-ems-muted">操作人</dt><dd>{selected.actor}</dd></div>
        <div><dt className="text-xs text-ems-muted">对象</dt><dd>{selected.object || "—"}</dd></div>
        <div><dt className="text-xs text-ems-muted">详情</dt><dd className="whitespace-pre-wrap">{selected.detail || "—"}</dd></div>
        <div><dt className="text-xs text-ems-muted">IP</dt><dd>{selected.ip || "—"}</dd></div>
        <div><dt className="text-xs text-ems-muted">记录 ID</dt><dd><code className="break-all">{selected.id}</code></dd></div>
      </dl>}
    </EmsOverlay>
  </section>
}
