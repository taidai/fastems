import { useState } from "react"
import type { EmsAlarm } from "./api"
import { EmsButton, EmsOverlay } from "./components/ems"

export default function AckDialog({ alarm, busy, onCancel, onConfirm }: {
  alarm: EmsAlarm
  busy: boolean
  onCancel: () => void
  onConfirm: (note: string) => void
}) {
  const [note, setNote] = useState("")
  return <EmsOverlay open title={`确认「${alarm.title}」`} busy={busy} onClose={onCancel}
    actions={<>
      <EmsButton variant="secondary" disabled={busy} onClick={onCancel}>取消</EmsButton>
      <EmsButton busy={busy} busyLabel="确认中…" disabled={!note.trim()} onClick={() => onConfirm(note.trim())}>确认报警</EmsButton>
    </>}>
    <div className="grid gap-4">
      <p className="m-0 text-sm leading-6 text-ems-muted">确认只表示已知悉，不会清除活动条件。请填写处理说明，便于交接和审计。</p>
      <label className="ems-field"><span className="ems-field-label">处理说明</span><textarea aria-label="处理说明" className="ems-input w-full resize-y" autoFocus rows={3} value={note} onChange={(event) => setNote(event.target.value)} placeholder="例如：已联系现场，检查通讯线缆" /></label>
    </div>
  </EmsOverlay>
}
