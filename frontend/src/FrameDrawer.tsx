import { useEffect, useMemo, useState } from "react"
import { EmsButton, EmsOverlay, SegmentedTabs, EmptyState } from "./components/ems"
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
  return <EmsOverlay open variant="drawer" title={`${nameOf(channel)} · 对下通讯`} onClose={onClose}
    actions={<EmsButton variant="secondary" onClick={onClose}>关闭</EmsButton>}>
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="m-0 text-xs text-ems-muted">{peer || "未设置通信地址"} · 只保留最近 500 帧，不写磁盘</p>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <SegmentedTabs ariaLabel="报文方向" value={direction} onChange={setDirection} options={[{ value: "all", label: "全部" }, { value: "rx", label: "收" }, { value: "tx", label: "发" }]} />
        <EmsButton variant="secondary" size="small" onClick={() => setPaused(!paused)}>{paused ? "继续" : "暂停"}</EmsButton>
        <EmsButton variant="secondary" size="small" onClick={() => void clearFrames()}>清空</EmsButton>
        <EmsButton variant="secondary" size="small" onClick={() => void copyFrames()}>复制</EmsButton>
        <span className="text-xs text-ems-muted">{visible.length} 条{loading && !paused ? " · 读取中" : ""}</span>
      </div>
      <div className="overflow-x-auto">
        <table className="ems-table">
          <thead><tr><th>时间</th><th>方向</th><th>报文（HEX）</th><th>对端</th></tr></thead>
          <tbody>
            {visible.map((frame) => <tr key={frame.id}>
              <td className="whitespace-nowrap">{new Date(frame.time).toLocaleTimeString("zh-CN", { hour12: false })}.{String(new Date(frame.time).getMilliseconds()).padStart(3, "0")}</td>
              <td>{frame.direction === "tx" ? "发" : "收"}</td>
              <td><code className="break-all">{frame.hex}</code></td>
              <td>{frame.peer || "—"}</td>
            </tr>)}
          </tbody>
        </table>
        {visible.length === 0 && <EmptyState title="还没有报文" description={peer ? `等待 ${peer} 的下一轮采集或命令写入。` : "选择通道后等待采集。"} />}
      </div>
    </div>
  </EmsOverlay>
}
