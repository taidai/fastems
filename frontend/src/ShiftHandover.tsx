import { useMemo, useState } from "react"
import { createPortal } from "react-dom"

type Handover = { id: string; shift: string; operator: string; note: string; at: string }

const STORAGE_KEY = "fastems-shift-handovers"

function loadHandovers(): Handover[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || "[]")
    return Array.isArray(parsed) ? parsed.filter((item) => item && typeof item.shift === "string" && typeof item.operator === "string") : []
  } catch {
    return []
  }
}

function saveHandovers(list: Handover[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(list))
}

export default function ShiftHandover({ onClose }: { onClose: () => void }) {
  const [records, setRecords] = useState<Handover[]>(loadHandovers)
  const [shift, setShift] = useState("")
  const [operator, setOperator] = useState("")
  const [note, setNote] = useState("")

  const recent = useMemo(() => [...records].sort((a, b) => b.at.localeCompare(a.at)).slice(0, 12), [records])

  function add(event: React.FormEvent) {
    event.preventDefault()
    const record: Handover = {
      id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
      shift: shift.trim() || "未注明班次",
      operator: operator.trim() || "未注明值班人",
      note: note.trim(),
      at: new Date().toISOString(),
    }
    const next = [...records, record]
    setRecords(next)
    saveHandovers(next)
    setShift("")
    setOperator("")
    setNote("")
  }

  function exportCSV() {
    const header = ["交接时间", "班次", "值班人", "备注"]
    const rows = [...records].sort((a, b) => b.at.localeCompare(a.at)).map((record) => [
      new Date(record.at).toLocaleString("zh-CN", { hour12: false }),
      record.shift,
      record.operator,
      record.note,
    ])
    const csv = [header, ...rows].map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(",")).join(String.fromCharCode(10))
    const url = URL.createObjectURL(new Blob([String.fromCharCode(0xFEFF) + csv], { type: "text/csv;charset=utf-8" }))
    const link = document.createElement("a")
    link.href = url
    link.download = `fastems-班次交接-${new Date().toISOString().slice(0, 10)}.csv`
    link.click()
    URL.revokeObjectURL(url)
  }

  return createPortal(<div className="device-confirm-backdrop" onClick={onClose}>
    <section className="device-confirm shift-handover" role="dialog" aria-label="班次交接" onClick={(event) => event.stopPropagation()}>
      <span className="section-kicker">本机保存</span>
      <h3>班次交接</h3>
      <p>记录交接班次、值班人与备注，导出为 CSV。记录只保存在本机浏览器，不含任何密码。</p>
      <form className="shift-handover-form" onSubmit={add}>
        <label>班次<input value={shift} onChange={(event) => setShift(event.target.value)} placeholder="例如：早班 / 中班 / 晚班" /></label>
        <label>值班人<input value={operator} onChange={(event) => setOperator(event.target.value)} placeholder="交接值班人" /></label>
        <label className="shift-note">备注<textarea value={note} onChange={(event) => setNote(event.target.value)} placeholder="交接事项、设备异常、注意事项" /></label>
        <div className="device-confirm-actions">
          <button type="button" className="secondary-button" onClick={exportCSV} disabled={records.length === 0}>导出 CSV</button>
          <button type="submit" className="primary-button">记录交接</button>
        </div>
      </form>
      {recent.length > 0 && <div className="shift-handover-list">
        <strong>最近交接</strong>
        {recent.map((record) => <div className="shift-handover-row" key={record.id}>
          <div><strong>{record.shift} · {record.operator}</strong><small>{record.note || "无备注"}</small></div>
          <time>{new Date(record.at).toLocaleString("zh-CN", { hour12: false })}</time>
        </div>)}
      </div>}
      <div className="device-confirm-actions">
        <button className="secondary-button" onClick={onClose}>关闭</button>
      </div>
    </section>
  </div>, document.body)
}
