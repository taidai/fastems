import { useCallback, useEffect, useMemo, useState } from "react"
import { EmsButton, EmsCard, EmsSectionHeader, EmptyState, MetricStrip, StatusBadge, type EmsTone } from "./components/ems"
import { request } from "./api"
import type { LedgerDevice } from "./deviceTypes"
import type { QualityStats } from "./generated/contract"

// 客户侧的通信与健康：只读视图，回答"通不通、稳不稳、断在哪"。
// 运行中的滚动窗口（5 分钟 / 1 小时）来自内存，重启归零；没有运行通道时退回台账的累计健康，
// 两者都要如实标注来源，不能把台账的成功率冒充窗口成功率。

const healthNames: Record<string, string> = { healthy: "在线", degraded: "不稳定", offline: "离线", unknown: "未知" }
const healthTones: Record<string, EmsTone> = { healthy: "success", degraded: "warning", offline: "danger", unknown: "neutral" }
const errorNames: Record<string, string> = { timeout: "超时", disconnect: "断线", crc: "CRC", exception: "异常码", write: "写失败", other: "其他" }

const percent = (value?: number, polls?: number) => (polls && polls > 0 && value !== undefined ? `${value.toFixed(1)}%` : "—")
const latency = (value?: number) => (value && value > 0 ? `${Math.round(value)} ms` : "—")
const clock = (value?: string) => {
  if (!value) return "—"
  const time = new Date(value)
  return Number.isFinite(time.getTime()) ? time.toLocaleTimeString("zh-CN", { hour12: false }) : "—"
}

type Row = {
  id: string
  name: string
  node: string
  server: string
  health: string
  window5: string
  window1h: string
  delay: string
  lastSuccess: string
}

function channelHealth(devices: { health: string }[]): string {
  if (devices.some((device) => device.health === "offline")) return "offline"
  if (devices.some((device) => device.health === "degraded")) return "degraded"
  if (devices.length === 0 || devices.every((device) => device.health === "unknown")) return "unknown"
  return "healthy"
}

export default function HealthView({ token, onError }: { token: string; onError: (message: string) => void }) {
  const [ledger, setLedger] = useState<LedgerDevice[]>([])
  const [channels, setChannels] = useState<QualityStats[]>([])
  const [devices, setDevices] = useState<QualityStats[]>([])
  const [loading, setLoading] = useState(false)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [ledgerResult, quality] = await Promise.all([
        request<{ devices: LedgerDevice[] }>(token, "/device-ledger"),
        request<{ channels: QualityStats[]; devices: QualityStats[] }>(token, "/ems/quality"),
      ])
      setLedger(ledgerResult.devices || [])
      setChannels(quality.channels || [])
      setDevices(quality.devices || [])
      setUpdatedAt(new Date())
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "通信质量读取失败")
    } finally {
      setLoading(false)
    }
  }, [token, onError])

  useEffect(() => {
    void load()
    const timer = window.setInterval(() => void load(), 5000)
    return () => window.clearInterval(timer)
  }, [load])

  const byKey = useMemo(() => {
    const map = new Map<string, LedgerDevice>()
    for (const device of ledger) map.set(`${device.channel_id}/${device.server_id}`, device)
    return map
  }, [ledger])

  // 质量窗口按运行中的通道聚合；台账按配置的通道聚合。两者取并集，
  // 这样"已配置但还没跑起来"的通道不会从页面上消失。
  const groups = useMemo(() => {
    const ids = Array.from(new Set([...channels.map((channel) => channel.channel_id), ...ledger.map((device) => device.channel_id)]))
    return ids.map((id) => {
      const quality = channels.find((channel) => channel.channel_id === id)
      const qualityDevices = devices.filter((device) => device.channel_id === id)
      const ledgerDevices = ledger.filter((device) => device.channel_id === id)
      const rows: Row[] = qualityDevices.length > 0
        ? qualityDevices.map((device) => {
          const info = byKey.get(device.device_id || "")
          return {
            id: device.device_id || "",
            name: info?.name || device.device_id || "未知设备",
            node: info?.business_node_name || "未绑定业务节点",
            server: device.device_id?.split("/").pop() || "—",
            health: device.health || "unknown",
            window5: percent(device.window_5m?.success_rate, device.window_5m?.polls),
            window1h: percent(device.window_1h?.success_rate, device.window_1h?.polls),
            delay: latency(device.avg_latency_ms),
            lastSuccess: clock(device.last_success),
          }
        })
        : ledgerDevices.map((device) => ({
          id: `${device.channel_id}/${device.server_id}`,
          name: device.name,
          node: device.business_node_name || "未绑定业务节点",
          server: String(device.server_id),
          health: device.health,
          window5: "—",
          window1h: "—",
          delay: latency(device.avg_latency_ms),
          lastSuccess: "—",
        }))
      return {
        id,
        name: ledgerDevices[0]?.channel_name || id,
        quality,
        rows,
        hasWindow: qualityDevices.length > 0,
      }
    })
  }, [channels, devices, ledger, byKey])

  const counts = useMemo(() => ({
    online: ledger.filter((device) => device.health === "healthy").length,
    degraded: ledger.filter((device) => device.health === "degraded").length,
    offline: ledger.filter((device) => device.health === "offline").length,
  }), [ledger])

  return <section className="grid gap-4">
    <EmsSectionHeader kicker="运行保障" title="通信与健康" description="通道与设备的成功率、延迟和最近错误；只读，不提供配置入口。"
      actions={<>
        <span className="text-xs text-ems-muted">{updatedAt ? `更新于 ${clock(updatedAt.toISOString())}` : "读取中…"}</span>
        <EmsButton variant="secondary" busy={loading} busyLabel="刷新中…" onClick={() => void load()}>刷新</EmsButton>
      </>} />

    <MetricStrip items={[
      { label: "通道", value: groups.length },
      { label: "设备", value: ledger.length },
      { label: "在线", value: counts.online, tone: "success" },
      { label: "不稳定", value: counts.degraded, tone: counts.degraded ? "warning" : "neutral" },
      { label: "离线", value: counts.offline, tone: counts.offline ? "danger" : "neutral" },
    ]} />

    {groups.length === 0
      ? <EmptyState title="还没有接入通道" description="当前站点没有可显示的通信通道；需要接入设备时请联系交付人员。" />
      : groups.map((group) => {
        const health = channelHealth(group.rows)
        const errors = Object.entries(group.quality?.window_5m?.errors || {}).filter(([, count]) => count > 0)
        return <EmsCard key={group.id} variant="subtle">
          <div className="grid gap-3">
            <EmsSectionHeader
              title={group.name}
              description={group.hasWindow
                ? `${group.rows.length} 台设备 · 累计 ${group.quality?.polls || 0} 次请求、${group.quality?.failures || 0} 次失败`
                : `${group.rows.length} 台设备 · 尚未产生运行窗口，以下为台账累计值`}
              status={<StatusBadge tone={healthTones[health] || "neutral"}>{healthNames[health] || health}</StatusBadge>} />

            {group.hasWindow && <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <div><span className="text-xs text-ems-muted">5 分钟成功率</span><strong className="block text-lg">{percent(group.quality?.window_5m?.success_rate, group.quality?.window_5m?.polls)}</strong></div>
              <div><span className="text-xs text-ems-muted">1 小时成功率</span><strong className="block text-lg">{percent(group.quality?.window_1h?.success_rate, group.quality?.window_1h?.polls)}</strong></div>
              <div><span className="text-xs text-ems-muted">平均延迟</span><strong className="block text-lg">{latency(group.quality?.avg_latency_ms)}</strong></div>
              <div><span className="text-xs text-ems-muted">最大延迟</span><strong className="block text-lg">{latency(group.quality?.max_latency_ms)}</strong></div>
            </div>}

            <div className="flex flex-wrap gap-2 text-xs text-ems-muted">
              {group.hasWindow
                ? <>
                  <span>最近成功 {clock(group.quality?.last_success)}</span>
                  {errors.length > 0
                    ? errors.map(([name, count]) => <span key={name} className="text-ems-amber">{errorNames[name] || name} {count}</span>)
                    : <span>最近 5 分钟没有失败</span>}
                  {group.quality?.last_error && <span>最近错误：{group.quality.last_error}</span>}
                </>
                : <span>该通道还没有运行统计；设备健康来自台账，吞吐与延迟在通道开始轮询后出现。</span>}
            </div>

            <div className="grid gap-2">
              {group.rows.map((row) => <div key={row.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-ems-line px-3 py-2">
                <div className="min-w-0">
                  <strong className="block truncate text-sm">{row.name}</strong>
                  <span className="text-xs text-ems-muted">{row.node} · 站号 {row.server}</span>
                </div>
                <div className="flex flex-wrap items-center gap-3 text-xs text-ems-muted">
                  <span>5 分钟 {row.window5}</span>
                  <span>1 小时 {row.window1h}</span>
                  <span>延迟 {row.delay}</span>
                  <span>最近成功 {row.lastSuccess}</span>
                  <StatusBadge tone={healthTones[row.health] || "neutral"}>{healthNames[row.health] || row.health}</StatusBadge>
                </div>
              </div>)}
            </div>
          </div>
        </EmsCard>
      })}

    <p className="m-0 text-xs text-ems-muted">成功率与延迟来自内存中的滚动窗口，应用重启后归零；累计请求与失败数保留。页面只显示本账号可见的站点。</p>
  </section>
}
