import { EmsButton, EmsOverlay } from "./components/ems"

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
  return <EmsOverlay open title={title} busy={busy} onClose={onCancel}
    actions={<>
      <EmsButton variant="secondary" disabled={busy} onClick={onCancel}>取消</EmsButton>
      <EmsButton variant={danger ? "danger" : "primary"} busy={busy} busyLabel="执行中…" onClick={onConfirm}>{confirmLabel}</EmsButton>
    </>}>
    <p className="m-0 text-sm leading-6 text-ems-ink">{message}</p>
    {rows && rows.length > 0 && <div className="mt-4 grid gap-2">{rows.map((row) => <div key={row.label} className="flex items-center justify-between gap-4 rounded-lg border border-ems-line px-3 py-2 text-sm"><strong>{row.label}</strong><span>{row.value}</span></div>)}</div>}
  </EmsOverlay>
}
