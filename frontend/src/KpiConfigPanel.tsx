import { useState } from "react"
import { createPortal } from "react-dom"
import { DEFAULT_KPI_CONFIG, type KpiConfig, type KpiKey, type KpiThreshold } from "./KpiStrip"

const LABELS: Record<KpiKey, string> = {
  revenue: "总收益",
  "ess-revenue": "储能收益",
  efficiency: "转换效率",
  fee: "今日电费",
  throughput: "储能吞吐",
  energy: "光伏发电量",
  "today-load": "今日用电",
  demand: "最大需量",
}

const ORDER: KpiKey[] = DEFAULT_KPI_CONFIG.order

export default function KpiConfigPanel({ config, onSave, onClose }: {
  config: KpiConfig
  onSave: (config: KpiConfig) => void
  onClose: () => void
}) {
  const [order, setOrder] = useState<KpiKey[]>(config.order)
  const [thresholds, setThresholds] = useState<KpiConfig["thresholds"]>(config.thresholds)

  const selected = new Set(order)

  function toggle(key: KpiKey, checked: boolean) {
    setOrder((current) => checked ? [...current, key] : current.filter((item) => item !== key))
  }

  function move(key: KpiKey, delta: -1 | 1) {
    setOrder((current) => {
      const index = current.indexOf(key)
      const target = index + delta
      if (index < 0 || target < 0 || target >= current.length) return current
      const next = [...current]
      const [item] = next.splice(index, 1)
      next.splice(target, 0, item)
      return next
    })
  }

  function setThreshold(key: KpiKey, patch: Partial<KpiThreshold> | null) {
    setThresholds((current) => {
      const next = { ...current }
      if (patch === null) delete next[key]
      else next[key] = { ...(current[key] || { value: 0, direction: "over" }), ...patch }
      return next
    })
  }

  function save() {
    onSave({ order, thresholds })
    onClose()
  }

  return createPortal(<div className="device-confirm-backdrop" onClick={onClose}>
    <section className="device-confirm kpi-config-panel" role="dialog" aria-label="配置 KPI" onClick={(event) => event.stopPropagation()}>
      <span className="section-kicker">本机保存</span>
      <h3>配置 KPI</h3>
      <p>勾选要显示的指标、上下调整顺序；为指标设置阈值，命中后该卡片数值会变色。仅保存在本机浏览器。</p>
      <div className="kpi-config-list">
        {ORDER.map((key) => {
          const active = selected.has(key)
          const threshold = thresholds[key]
          return <div className={`kpi-config-row ${active ? "" : "off"}`} key={key}>
            <label className="kpi-config-check"><input type="checkbox" checked={active} onChange={(event) => toggle(key, event.target.checked)} /><span>{LABELS[key]}</span></label>
            <div className="kpi-config-order">
              <button type="button" disabled={!active || order.indexOf(key) === 0} onClick={() => move(key, -1)}>上移</button>
              <button type="button" disabled={!active || order.indexOf(key) === order.length - 1} onClick={() => move(key, 1)}>下移</button>
            </div>
            <div className="kpi-config-threshold">
              <label className="kpi-config-threshold-on"><input type="checkbox" checked={Boolean(threshold)} onChange={(event) => setThreshold(key, event.target.checked ? { value: 0, direction: "over" } : null)} />阈值</label>
              {threshold && <>
                <select aria-label={`${LABELS[key]}阈值方向`} value={threshold.direction} onChange={(event) => setThreshold(key, { direction: event.target.value as "over" | "under" })}>
                  <option value="over">超过</option><option value="under">低于</option>
                </select>
                <input aria-label={`${LABELS[key]}阈值`} type="number" value={threshold.value} onChange={(event) => setThreshold(key, { value: Number(event.target.value) })} />
              </>}
            </div>
          </div>
        })}
      </div>
      <div className="device-confirm-actions">
        <button className="secondary-button" onClick={() => { setOrder(DEFAULT_KPI_CONFIG.order); setThresholds({}) }}>恢复默认</button>
        <button className="secondary-button" onClick={onClose}>取消</button>
        <button className="primary-button" onClick={save}>保存</button>
      </div>
    </section>
  </div>, document.body)
}
