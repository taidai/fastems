import { useCallback, useEffect, useMemo, useState } from "react"
import { request } from "./api"
import DeviceDetail from "./DeviceDetail"
import type { LedgerDevice } from "./deviceTypes"


const healthNames: Record<string, string> = { healthy: "在线", degraded: "不稳定", offline: "离线", unknown: "未知" }

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

  return <section className="device-ledger-page">
    <div className="ledger-summary">
      <div><strong>{counts.total}</strong><span>设备</span></div>
      <div><strong>{counts.online}</strong><span>在线</span></div>
      <div className={counts.degraded ? "warn" : ""}><strong>{counts.degraded}</strong><span>不稳定</span></div>
      <div className={counts.offline ? "bad" : ""}><strong>{counts.offline}</strong><span>离线</span></div>
      <div><strong>{counts.verified}</strong><span>已验证</span></div>
      <button className="secondary-button" disabled={loading} onClick={() => void load()}>{loading ? "刷新中…" : "刷新"}</button>
    </div>

    <div className="panel ledger-panel">
      <div className="ledger-toolbar">
        <input aria-label="搜索设备" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索设备、业务节点、通道或 Profile" />
        <select aria-label="设备健康" value={health} onChange={(event) => setHealth(event.target.value)}><option value="all">全部健康状态</option><option value="healthy">在线</option><option value="degraded">不稳定</option><option value="offline">离线</option><option value="unknown">未知</option></select>
        <span>共 {shown.length} 台</span>
      </div>
      <div className="ledger-table-wrap">
        <table className="ledger-table">
          <thead><tr><th>设备</th><th>业务节点</th><th>通道 / 站号</th><th>Profile</th><th>点位 / 命令</th><th>健康</th><th>验证</th><th>操作</th></tr></thead>
          <tbody>
            {shown.map((device) => <tr key={`${device.channel_id}/${device.server_id}`}>
              <td><strong>{deviceLabel(device)}</strong><small>{device.name !== deviceLabel(device) ? device.name : device.device_serial || ""}</small></td>
              <td>{device.business_node_name || "未绑定"}</td>
              <td>{device.channel_name} / 站号 {device.server_id}<small>{device.address || "—"}</small></td>
              <td>{device.profile_id ? <><strong>{device.vendor} {device.model}</strong><small>{device.profile_id}</small></> : <><span className="ledger-unverified">未绑定 Profile</span><button className="ledger-inline-action" onClick={goChannels}>去绑定</button></>}</td>
              <td>{device.point_count} / {device.command_count}</td>
              <td><span className={`ledger-health ${device.health}`}>{healthNames[device.health] || device.health}</span>{device.success_rate > 0 && <small>通信 {device.success_rate.toFixed(0)}% · {Math.round(device.avg_latency_ms)}ms</small>}</td>
              <td>{device.verified ? <span className="ledger-verified">已验证</span> : <span className="ledger-unverified">未验证</span>}</td>
              <td className="ledger-actions"><button onClick={() => setSelected(device)}>详情</button></td>
            </tr>)}
          </tbody>
        </table>
        {!loading && shown.length === 0 && <div className="empty-state"><strong>没有匹配的设备</strong><span>先在“通道”里配置端口和单元，或在“端口配置”绑定 Profile。</span></div>}
      </div>
    </div>

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
