import { useCallback, useEffect, useState } from "react"
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

export default function AuditView({ token, onError }: { token: string; onError: (message: string) => void }) {
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

  return <section className="audit-page panel">
    <div className="audit-head">
      <div><span className="section-kicker">只读留痕</span><h2>审计日志</h2><p>登录、配置、控制、报警与账号操作；不记录密码、令牌或请求体。</p></div>
      <button className="secondary-button" disabled={loading} onClick={() => void load(filters)}>{loading ? "读取中…" : "刷新"}</button>
    </div>
    <div className="audit-filters">
      <label>操作人<input value={draft.actor} onChange={(event) => setDraft({ ...draft, actor: event.target.value })} placeholder="账号或用户 ID" /></label>
      <label>动作<select value={draft.action} onChange={(event) => setDraft({ ...draft, action: event.target.value })}><option value="">全部</option>{actions.map((action) => <option key={action} value={action}>{action}</option>)}</select></label>
      <label>结果<select value={draft.result} onChange={(event) => setDraft({ ...draft, result: event.target.value })}><option value="">全部</option><option value="success">成功</option><option value="failure">失败</option></select></label>
      <label>开始日期<input type="date" value={draft.from} onChange={(event) => setDraft({ ...draft, from: event.target.value })} /></label>
      <label>结束日期<input type="date" value={draft.to} onChange={(event) => setDraft({ ...draft, to: event.target.value })} /></label>
      <div className="audit-actions">
        <button className="primary-button" disabled={loading} onClick={() => { setFilters(draft); void load(draft) }}>查询</button>
        <button className="secondary-button" disabled={loading} onClick={() => { setDraft(emptyFilters); setFilters(emptyFilters); void load(emptyFilters) }}>重置</button>
        <button className="secondary-button" disabled={loading} onClick={() => void downloadCSV(token, `/audit/export?${queryString(filters)}`, "easyems-audit.csv").catch((cause) => onError(cause instanceof Error ? cause.message : "导出失败"))}>导出 CSV</button>
      </div>
    </div>

    <div className="audit-layout">
      <div className="audit-table-wrap">
        <table className="audit-table">
          <thead><tr><th>时间</th><th>操作人</th><th>动作</th><th>对象</th><th>结果</th><th>IP</th></tr></thead>
          <tbody>
            {records.map((record) => <tr key={record.id} className={selected?.id === record.id ? "selected" : ""} onClick={() => setSelected(record)}>
              <td>{new Date(record.time).toLocaleString("zh-CN", { hour12: false })}</td>
              <td>{record.actor}</td>
              <td><span className="audit-action">{record.action}</span></td>
              <td title={record.object}>{record.object || "—"}</td>
              <td><span className={`audit-result ${record.result}`}>{record.result === "success" ? "成功" : "失败"}</span></td>
              <td>{record.ip || "—"}</td>
            </tr>)}
          </tbody>
        </table>
        {!loading && records.length === 0 && <div className="empty-state"><strong>没有匹配的审计记录</strong><span>登录或执行一次配置/控制后，这里会出现记录。</span></div>}
      </div>
      <aside className="audit-detail panel">
        {selected ? <>
          <div className="audit-detail-head"><strong>{selected.action}</strong><span className={`audit-result ${selected.result}`}>{selected.result === "success" ? "成功" : "失败"}</span></div>
          <dl>
            <div><dt>时间</dt><dd>{new Date(selected.time).toLocaleString("zh-CN", { hour12: false })}</dd></div>
            <div><dt>操作人</dt><dd>{selected.actor}</dd></div>
            <div><dt>对象</dt><dd>{selected.object || "—"}</dd></div>
            <div><dt>详情</dt><dd>{selected.detail || "—"}</dd></div>
            <div><dt>IP</dt><dd>{selected.ip || "—"}</dd></div>
            <div><dt>记录 ID</dt><dd><code>{selected.id}</code></dd></div>
          </dl>
        </> : <div className="empty-state"><span>选择一条记录查看详情。</span></div>}
      </aside>
    </div>
  </section>
}
