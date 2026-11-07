import { useState } from "react"
import { createPortal } from "react-dom"
import type { EmsAlarm } from "./api"

export default function AckDialog({ alarm, busy, onCancel, onConfirm }: {
  alarm: EmsAlarm
  busy: boolean
  onCancel: () => void
  onConfirm: (note: string) => void
}) {
  const [note, setNote] = useState("")
  return createPortal(<div className="device-confirm-backdrop" onClick={onCancel}>
    <section className="device-confirm" role="dialog" aria-label="确认报警" onClick={(event) => event.stopPropagation()}>
      <span className="section-kicker">报警处理</span>
      <h3>确认「{alarm.title}」</h3>
      <p>确认只表示已知悉，不会清除活动条件。请填写处理说明，便于交接和审计。</p>
      <label className="ack-note-field"><span>处理说明</span><textarea autoFocus rows={3} value={note} onChange={(event) => setNote(event.target.value)} placeholder="例如：已联系现场，检查通讯线缆" /></label>
      <div className="device-confirm-actions">
        <button className="secondary-button" disabled={busy} onClick={onCancel}>取消</button>
        <button className="primary-button" disabled={busy || !note.trim()} onClick={() => onConfirm(note.trim())}>{busy ? "确认中…" : "确认报警"}</button>
      </div>
    </section>
  </div>, document.body)
}
