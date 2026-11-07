import { useCallback, useEffect, useState } from "react"
import { request } from "./api"

type Item = { id: string; title: string; status: "pass" | "warning" | "fail"; detail: string }

export default function DeliveryView({ token }: { token: string }) {
  const [items, setItems] = useState<Item[]>([])
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)
  const load = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      const result = await request<{ items: Item[] }>(token, "/delivery/check")
      setItems(result.items)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "交付检查失败")
    } finally {
      setLoading(false)
    }
  }, [token])
  useEffect(() => { void load() }, [load])
  const failures = items.filter((item) => item.status === "fail").length
  const warnings = items.filter((item) => item.status === "warning").length
  return <section className="panel delivery-panel">
    <div className="panel-heading"><div><span className="section-kicker">基于当前真实数据</span><h2>交付检查</h2><p>检查结果只反映本实例当前状态，不替代真实设备与独立用户验收。</p></div><button className="secondary-button" disabled={loading} onClick={() => void load()}>{loading ? "检查中…" : "重新检查"}</button></div>
    <div className="delivery-summary"><strong>{failures ? `${failures} 项必须处理` : warnings ? `${warnings} 项需要关注` : "可以进入交付"}</strong><span>{failures ? "请先解决失败项" : warnings ? "查看警告并确认现场验收与备份计划" : "当前配置检查通过"}</span></div>
    {error && <div className="inline-error">{error}</div>}
    <div className="delivery-list">{items.map((item) => <article className={`delivery-item ${item.status}`} key={item.id}><span className="delivery-status">{item.status === "pass" ? "通过" : item.status === "fail" ? "需处理" : "需关注"}</span><div><strong>{item.title}</strong><p>{item.detail}</p></div></article>)}</div>
  </section>
}
