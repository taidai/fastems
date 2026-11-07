import { createPortal } from "react-dom"

export default function ConfirmDialog({ title, message, rows, confirmLabel = "确认", danger = false, busy = false, onConfirm, onCancel }: {
  title: string
  message: string
  rows?: { label: string; value: string }[]
  confirmLabel?: string
  danger?: boolean
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  return createPortal(<div className="device-confirm-backdrop" onClick={onCancel}>
    <section className="device-confirm" role="dialog" aria-label={title} onClick={(event) => event.stopPropagation()}>
      <span className="section-kicker">受控操作</span>
      <h3>{title}</h3>
      <p>{message}</p>
      {rows && rows.length > 0 && <div className="device-target-list">{rows.map((row) => <div key={row.label}><strong>{row.label}</strong><span>{row.value}</span></div>)}</div>}
      <div className="device-confirm-actions">
        <button className="secondary-button" disabled={busy} onClick={onCancel}>取消</button>
        <button className={danger ? "danger-button" : "primary-button"} disabled={busy} onClick={onConfirm}>{busy ? "执行中…" : confirmLabel}</button>
      </div>
    </section>
  </div>, document.body)
}
