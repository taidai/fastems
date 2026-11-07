import { useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { nameOf, Node, point, request } from "./api"
import type { Frame } from "./generated/contract"

export default function FrameDrawer({ token, channel, open, onClose, onError }: {
  token: string
  channel: Node | null
  open: boolean
  onClose: () => void
  onError: (message: string) => void
}) {
  const [frames, setFrames] = useState<Frame[]>([])
  const [direction, setDirection] = useState<"all" | "tx" | "rx">("all")
  const [paused, setPaused] = useState(false)
  const [loading, setLoading] = useState(false)
  const peer = channel ? (point(channel, "protocol")?.text === "RTU" ? point(channel, "port")?.text || "" : point(channel, "uri")?.text || "") : ""

  useEffect(() => {
    if (!open) return
    let cancelled = false
    const load = async () => {
      try {
        const result = await request<{ frames: Frame[] }>(token, "/ems/frames?limit=500")
        if (!cancelled && !paused) setFrames(result.frames || [])
      } catch (cause) {
        if (!cancelled) onError(cause instanceof Error ? cause.message : "报文读取失败")
      }
    }
    setLoading(true)
    void load().finally(() => { if (!cancelled) setLoading(false) })
    const timer = window.setInterval(() => void load(), 1000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [open, paused, token, onError])

  useEffect(() => { if (open) setPaused(false) }, [open])

  const visible = useMemo(() => frames.filter((frame) => {
    if (peer && frame.peer !== peer) return false
    return direction === "all" || frame.direction === direction
  }), [frames, peer, direction])

  async function clearFrames() {
    try {
      await request(token, "/ems/frames", "DELETE")
      setFrames([])
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "清空失败")
    }
  }

  async function copyFrames() {
    const text = visible.map((frame) => `${new Date(frame.time).toLocaleString("zh-CN", { hour12: false })} ${frame.direction === "tx" ? "发" : "收"} ${frame.hex} ${frame.peer || ""}`).join("\n")
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      onError("复制失败，请手动选择报文")
    }
  }

  if (!open || !channel) return null
  return createPortal(<>
    <div className="frame-backdrop" onClick={onClose} />
    <section className="frame-modal" role="dialog" aria-label="对下通讯报文">
      <div className="frame-modal-head">
        <div><span className="section-kicker">原始报文</span><h2>{nameOf(channel)} · 对下通讯</h2><p>{peer || "未设置通信地址"} · 只保留最近 500 帧，不写磁盘</p></div>
        <button className="icon-button" aria-label="关闭" onClick={onClose}>×</button>
      </div>
      <div className="frame-toolbar">
        <div className="frame-segment" role="group" aria-label="报文方向">
          <button className={direction === "all" ? "active" : ""} onClick={() => setDirection("all")}>全部</button>
          <button className={direction === "rx" ? "active" : ""} onClick={() => setDirection("rx")}>收</button>
          <button className={direction === "tx" ? "active" : ""} onClick={() => setDirection("tx")}>发</button>
        </div>
        <button className="secondary-button" onClick={() => setPaused(!paused)}>{paused ? "继续" : "暂停"}</button>
        <button className="secondary-button" onClick={() => void clearFrames()}>清空</button>
        <button className="secondary-button" onClick={() => void copyFrames()}>复制</button>
        <span className="frame-count">{visible.length} 条{loading && !paused ? " · 读取中" : ""}</span>
      </div>
      <div className="frame-table-wrap">
        <table className="frame-table">
          <thead><tr><th>时间</th><th>方向</th><th>报文（HEX）</th><th>对端</th></tr></thead>
          <tbody>
            {visible.map((frame) => <tr key={frame.id}>
              <td>{new Date(frame.time).toLocaleTimeString("zh-CN", { hour12: false })}.{String(new Date(frame.time).getMilliseconds()).padStart(3, "0")}</td>
              <td><span className={`frame-direction ${frame.direction}`}>{frame.direction === "tx" ? "发" : "收"}</span></td>
              <td><code>{frame.hex}</code></td>
              <td>{frame.peer || "—"}</td>
            </tr>)}
          </tbody>
        </table>
        {visible.length === 0 && <div className="empty-state"><strong>还没有报文</strong><span>{peer ? `等待 ${peer} 的下一轮采集或命令写入。` : "选择通道后等待采集。"}</span></div>}
      </div>
    </section>
  </>, document.body)
}
