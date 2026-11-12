import { useCallback, useEffect, useMemo, useState } from "react"
import { EmsButton, EmsRecordList, EmsSectionHeader, EmptyState, FilterToolbar, MetricStrip, StatusBadge, type EmsTone } from "./components/ems"
import { request } from "./api"
import DeviceDetail from "./DeviceDetail"
import type { LedgerDevice } from "./deviceTypes"

const healthNames: Record<string, string> = { healthy: "在线", degraded: "不稳定", offline: "离线", unknown: "未知" }
const healthTones: Record<string, EmsTone> = { healthy: "success", degraded: "warning", offline: "danger", unknown: "neutral" }

export default function DeviceLedgerView({ token, onError, canConfigure, canOperate, goChannels, goPoints, goHistory, goAlarms }: {
  token: string
  onError: (message: string) => void
  canConfigure: boolean
  canOperate: boolean
  goChannels: () => void
  goPoints: () => void
  goHistory: () => void
  goAlarms: () => void
}) {
  const [devices, setDevices] = useState<LedgerDevice[]>([])
  const [query, setQuery] = useState("")
  const [health, setHealth] = useState("all")
  const [selected, setSelected] = useState<LedgerDevice | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const result = await request<{ devices: LedgerDevice[] }>(token, "/device-ledger")
      setDevices(result.devices || [])
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "设备台账读取失败")
    } finally {
      setLoading(false)
    }
  }, [token, onError])

  useEffect(() => {
    void load()
    const timer = window.setInterval(() => void load(), 3000)
    return () => window.clearInterval(timer)
  }, [load])

  const shown = useMemo(() => devices.filter((device) => {
    if (health !== "all" && device.health !== health) return false
    if (!query.trim()) return true
    const text = `${device.name} ${device.business_node_name || ""} ${device.channel_name} ${device.profile_id || ""} ${device.vendor || ""} ${device.model || ""}`.toLowerCase()
    return text.includes(query.trim().toLowerCase())
  }), [devices, query, health])

  const counts = {
    total: devices.length,
    online: devices.filter((device) => device.health === "healthy").length,
    degraded: devices.filter((device) => device.health === "degraded").length,
    offline: devices.filter((device) => device.health === "offline").length,
    verified: devices.filter((device) => device.verified).length,
  }

  const deviceLabel = (device: LedgerDevice) => device.business_node_name ||
    (device.name && device.name !== `站号 ${device.server_id}` ? device.name : `${device.channel_name} · 站号 ${device.server_id}`)

  function nextAction(device: LedgerDevice) {
    // 客户工作台是只读视图：只提供查看详情，不引导到通道或绑定流程。
    if (!canConfigure) return { label: "查看", run: () => setSelected(device) }
    if (!device.profile_id) return { label: "绑定 Profile →", run: goChannels }
    if (!device.verified) return { label: "去验证 →", run: () => setSelected(device) }
    return { label: "管理", run: () => setSelected(device) }
  }

  return <section className="grid gap-4">
    <EmsSectionHeader kicker="设备对象" title="设备与验证" description="一台设备一行：业务节点、通道站号、Profile、健康与验证。"
      actions={<>
        <EmsButton variant="secondary" busy={loading} busyLabel="刷新中…" onClick={() => void load()}>刷新</EmsButton>
        {canConfigure && <EmsButton onClick={goChannels}>+ 接入新设备</EmsButton>}
      </>} />

    <MetricStrip items={[
      { label: "设备", value: counts.total },
      { label: "在线", value: counts.online, tone: "success" },
      { label: "不稳定", value: counts.degraded, tone: counts.degraded ? "warning" : "neutral" },
      { label: "离线", value: counts.offline, tone: counts.offline ? "danger" : "neutral" },
      { label: "已验证", value: counts.verified },
    ]} />

    <FilterToolbar trailing={<span className="text-xs text-ems-muted">共 {shown.length} 台</span>}>
      <input className="ems-input" aria-label="搜索设备" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索设备、业务节点、通道或 Profile" />
      <select className="ems-input" aria-label="设备健康" value={health} onChange={(event) => setHealth(event.target.value)}><option value="all">全部健康状态</option><option value="healthy">在线</option><option value="degraded">不稳定</option><option value="offline">离线</option><option value="unknown">未知</option></select>
    </FilterToolbar>

    <EmsRecordList rows={shown} keyOf={(device) => `${device.channel_id}/${device.server_id}`} onRowClick={setSelected}
      empty={<EmptyState title="没有匹配的设备" description={canConfigure ? "先在“通道”里配置端口和单元，或在“端口配置”绑定 Profile。" : "当前没有可显示的设备。"} />}
      columns={[
        { key: "device", label: "设备", render: (device) => <><strong>{deviceLabel(device)}</strong><small className="block text-ems-muted">{device.name !== deviceLabel(device) ? device.name : device.device_serial || ""}</small></> },
        { key: "business", label: "业务节点", render: (device) => device.business_node_name || "未绑定" },
        { key: "channel", label: "通道 / 站号", render: (device) => <>{device.channel_name} / 站号 {device.server_id}<small className="block text-ems-muted">{device.address || "—"}</small></> },
        { key: "profile", label: "Profile", render: (device) => device.profile_id ? <><strong>{device.vendor} {device.model}</strong><small className="block text-ems-muted">{device.profile_id}</small></> : <span className="text-ems-amber">未绑定 Profile</span> },
        { key: "points", label: "点位 / 命令", render: (device) => `${device.point_count} / ${device.command_count}` },
        { key: "health", label: "健康", render: (device) => <><StatusBadge tone={healthTones[device.health] || "neutral"}>{healthNames[device.health] || device.health}</StatusBadge>{device.success_rate > 0 && <small className="block text-ems-muted">通信 {device.success_rate.toFixed(0)}% · {Math.round(device.avg_latency_ms)}ms</small>}</> },
        { key: "verified", label: "验证", render: (device) => device.verified ? <StatusBadge tone="success">已验证</StatusBadge> : <StatusBadge tone="warning">未验证</StatusBadge> },
        { key: "action", label: "下一步", render: (device) => { const action = nextAction(device); return <EmsButton variant={device.verified ? "secondary" : "quiet"} size="small" onClick={(event) => { event.stopPropagation(); action.run() }}>{action.label}</EmsButton> } },
      ]}
      mobileCard={(device) => { const action = nextAction(device); return <div className="grid gap-2">
        <div className="flex items-start justify-between gap-2"><strong>{deviceLabel(device)}</strong><StatusBadge tone={healthTones[device.health] || "neutral"}>{healthNames[device.health] || device.health}</StatusBadge></div>
        <span className="text-xs text-ems-muted">{device.channel_name} / 站号 {device.server_id} · {device.profile_id || "未绑定 Profile"}</span>
        <div className="flex items-center justify-between gap-2"><span className="text-xs text-ems-muted">{device.point_count} 点位 / {device.command_count} 命令</span><EmsButton variant={device.verified ? "secondary" : "quiet"} size="small" onClick={(event) => { event.stopPropagation(); action.run() }}>{action.label}</EmsButton></div>
      </div> }}
    />

    {selected && <DeviceDetail
      key={`${selected.channel_id}/${selected.server_id}`}
      token={token}
      device={selected}
      canConfigure={canConfigure}
      canOperate={canOperate}
      onError={onError}
      onClose={() => setSelected(null)}
      goChannels={() => { setSelected(null); goChannels() }}
      goAlarms={() => { setSelected(null); goAlarms() }}
    />}
  </section>
}
