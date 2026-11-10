import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSectionHeader, EmptyState, InlineNotice, EmsRecordList, StatusBadge } from "./components/ems"
import { createOpcuaChannel, createOpcuaPoint, fetchOpcuaChannels, fetchOpcuaPoints, type OpcuaChannel, type OpcuaPoint } from "./api"

// OPC-UA 接入（接入建模层）：每条通道是一个端点，测点使用完整 NodeID。
// 在线状态来自 runner 的真实轮询；未连通时显示“—”，不伪造数值。
export default function OpcuaView({ token, onError }: { token: string; onError: (message: string) => void }) {
  const [channels, setChannels] = useState<OpcuaChannel[]>([])
  const [selected, setSelected] = useState<string>("")
  const [points, setPoints] = useState<OpcuaPoint[]>([])
  const [description, setDescription] = useState("")
  const [endpoint, setEndpoint] = useState("")
  const [pollMS, setPollMS] = useState(2000)
  const [nodeID, setNodeID] = useState("")
  const [pointName, setPointName] = useState("")
  const [readOnly, setReadOnly] = useState(false)
  const [busy, setBusy] = useState(false)

  function refresh() {
    void fetchOpcuaChannels(token).then((result) => {
      setChannels(result.channels || [])
      setSelected((current) => current || result.channels?.[0]?.id || "")
    }).catch(() => onError("OPC-UA 通道读取失败"))
  }
  useEffect(refresh, [token])
  useEffect(() => {
    if (!selected) { setPoints([]); return }
    void fetchOpcuaPoints(token, selected).then((result) => setPoints(result.points || [])).catch(() => {})
  }, [token, selected])

  async function addChannel() {
    if (!endpoint.trim()) { onError("请填写端点地址，例如 opc.tcp://192.168.1.10:4840"); return }
    setBusy(true)
    try {
      await createOpcuaChannel(token, { description: description.trim() || endpoint.trim(), endpoint: endpoint.trim(), poll_ms: pollMS })
      setDescription(""); setEndpoint(""); refresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "创建失败")
    } finally {
      setBusy(false)
    }
  }

  async function addPoint() {
    if (!selected) { onError("请先选择通道"); return }
    if (!nodeID.trim()) { onError("请填写 OPC-UA NodeID，例如 ns=2;s=MyVar"); return }
    setBusy(true)
    try {
      await createOpcuaPoint(token, selected, { node_id: nodeID.trim(), description: pointName.trim(), read_only: readOnly })
      setNodeID(""); setPointName(""); setReadOnly(false)
      void fetchOpcuaPoints(token, selected).then((result) => setPoints(result.points || []))
    } catch (error) {
      onError(error instanceof Error ? error.message : "创建失败")
    } finally {
      setBusy(false)
    }
  }

  return <section className="grid gap-4">
    <EmsSectionHeader kicker="接入建模" title="OPC-UA" description="连接 OPC-UA 服务器并按完整 NodeID 映射测点；首版支持匿名轮询读取与写入。" />
    <EmsCard><IonCardContent className="grid gap-4">
      <strong className="text-sm text-ems-ink">新增端点</strong>
      <div className="grid gap-3 sm:grid-cols-2">
        <EmsInput label="端点" aria-label="OPC-UA 端点" value={endpoint} onIonInput={(event) => setEndpoint(event.detail.value || "")} placeholder="opc.tcp://host:4840" />
        <EmsInput label="说明（可选）" aria-label="通道说明" value={description} onIonInput={(event) => setDescription(event.detail.value || "")} placeholder="例如：1 号逆变器网关" />
        <EmsInput label="轮询周期（毫秒）" aria-label="轮询周期" type="number" value={pollMS} onIonInput={(event) => setPollMS(Number(event.detail.value))} />
      </div>
      <InlineNotice tone="info">首版仅匿名连接；证书与用户认证未启用前不宣称支持。轮询失败会显示离线与原因，不生成替代数据。</InlineNotice>
      <div className="flex justify-end"><EmsButton busy={busy} busyLabel="创建中…" onClick={() => void addChannel()}>创建通道</EmsButton></div>
    </IonCardContent></EmsCard>

    {channels.length === 0 ? <EmptyState title="还没有 OPC-UA 通道" description="先创建一个端点，再为它添加 NodeID 测点。" /> : <div className="grid gap-3">
      {channels.map((channel) => <EmsCard key={channel.id}><IonCardContent className="grid gap-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div><strong className="text-sm text-ems-ink">{channel.description || channel.id}</strong>
            <p className="m-0 break-all text-xs text-ems-muted">{channel.endpoint} · {channel.points} 个测点</p></div>
          <div className="flex items-center gap-2">
            <StatusBadge tone={channel.online ? "success" : "warning"}>{channel.online ? "在线" : "未连通"}</StatusBadge>
            <EmsButton size="small" variant="secondary" onClick={() => setSelected(channel.id)}>{selected === channel.id ? "已选择" : "管理测点"}</EmsButton>
          </div>
        </div>
        {channel.last_error && !channel.online && <InlineNotice tone="warning">最近错误：{channel.last_error}</InlineNotice>}
      </IonCardContent></EmsCard>)}
    </div>}

    {selected && <EmsCard><IonCardContent className="grid gap-4">
      <EmsSectionHeader title="测点映射" description="NodeID 使用完整写法：ns=2;s=Tag、ns=3;i=1001。" />
      <div className="grid gap-3 sm:grid-cols-3">
        <EmsInput label="NodeID" aria-label="NodeID" value={nodeID} onIonInput={(event) => setNodeID(event.detail.value || "")} placeholder="ns=2;s=MyVar" />
        <EmsInput label="说明（可选）" aria-label="测点说明" value={pointName} onIonInput={(event) => setPointName(event.detail.value || "")} />
        <label className="flex items-end gap-2 pb-2 text-sm text-ems-ink">
          <input type="checkbox" checked={readOnly} onChange={(event) => setReadOnly(event.target.checked)} /> 只读测点
        </label>
      </div>
      <div className="flex justify-end"><EmsButton busy={busy} busyLabel="保存中…" onClick={() => void addPoint()}>添加测点</EmsButton></div>
      <EmsRecordList rows={points} keyOf={(point) => point.id}
        empty={<EmptyState title="还没有测点" description="添加 NodeID 后，runner 会按周期读取并写回当前值。" />}
        columns={[
          { key: "name", label: "测点", render: (point) => point.description || point.id },
          { key: "node", label: "NodeID", render: (point) => <code className="break-all text-xs">{point.node_id}</code> },
          { key: "value", label: "当前值", render: (point) => point.value === undefined ? "—" : String(point.value) },
          { key: "ro", label: "读/写", render: (point) => point.read_only ? "只读" : "可写" },
        ]}
        mobileCard={(point) => <div className="grid gap-1"><strong>{point.description || point.id}</strong>
          <code className="break-all text-xs">{point.node_id}</code>
          <span className="text-xs text-ems-muted">{point.value === undefined ? "—" : point.value} · {point.read_only ? "只读" : "可写"}</span>
        </div>}
      />
    </IonCardContent></EmsCard>}
  </section>
}
