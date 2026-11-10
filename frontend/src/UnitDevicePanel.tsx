import { useEffect, useState } from "react"
import { IonCardContent, IonCardHeader, IonCardSubtitle, IonCardTitle } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSelect, EmsSelectOption } from "./components/ems"
import { fetchEnergyDevices, createEnergyDevice } from "./api"
import type { EnergyDevice, Node } from "./api"

// 柜内设备接线：把 Device 挂到 EnergyUnit，并映射功率设定点位。
// 属于接入建模层——EMS 业务层不应接触点位。
export default function UnitDevicePanel({ token, unitId, nodes, onError }: {
  token: string
  unitId: string
  nodes: Node[]
  onError: (message: string) => void
}) {
  const [devices, setDevices] = useState<EnergyDevice[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [name, setName] = useState("")
  const [rated, setRated] = useState(100)
  const [pointId, setPointId] = useState("")
  const [busy, setBusy] = useState(false)
  const powerPoints = nodes.filter((node) => node.type === "modbusIo")

  function refresh() {
    void fetchEnergyDevices(token, unitId)
      .then((result) => setDevices(result.devices || []))
      .catch(() => onError("设备列表读取失败"))
  }
  useEffect(refresh, [token, unitId])

  async function add() {
    if (!name.trim()) { onError("请填写设备名称"); return }
    if (rated <= 0) { onError("额定功率必须大于 0"); return }
    if (!pointId) { onError("请选择功率设定点位"); return }
    setBusy(true)
    try {
      await createEnergyDevice(token, unitId, { description: name.trim(), rated_power_kw: rated, power_setpoint_id: pointId })
      setName(""); setRated(100); setPointId(""); setShowAdd(false)
      refresh()
    } catch (error) {
      onError(error instanceof Error ? error.message : "添加设备失败")
    } finally {
      setBusy(false)
    }
  }

  return <EmsCard>
    <IonCardHeader className="pb-2">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><IonCardTitle className="text-base">柜内设备</IonCardTitle><IonCardSubtitle>设备映射功率设定点位后，调度才能写到真实设备</IonCardSubtitle></div>
        <EmsButton variant="secondary" size="small" onClick={() => setShowAdd(!showAdd)}>{showAdd ? "收起" : "+ 添加设备"}</EmsButton>
      </div>
    </IonCardHeader>
    <IonCardContent>
      {devices.length === 0 ? <p className="m-0 text-sm text-ems-muted">还没有设备。添加一台 PCS 并映射功率点位，调度才能下发。</p> : <div className="grid gap-2">
        {devices.map((device) => <div key={device.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-ems-line px-3 py-2 text-sm">
          <strong className="text-ems-ink">{device.description || device.asset_code}</strong>
          <span className="text-ems-muted">额定 {device.rated_power_kw} kW</span>
          <span className="break-all text-xs text-ems-muted">功率点位 {device.power_setpoint_id || "未映射"}</span>
        </div>)}
      </div>}

      {showAdd && <div className="mt-4 grid gap-3 border-t border-ems-line pt-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <EmsInput label="设备名称" aria-label="设备名称" placeholder="例如：1 号 PCS" value={name} onIonInput={(event) => setName(event.detail.value || "")} />
          <EmsInput label="额定功率（kW）" aria-label="设备额定功率" type="number" value={rated} onIonInput={(event) => setRated(Number(event.detail.value))} />
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <EmsSelect label="功率设定点位" aria-label="功率设定点位" value={pointId} onIonChange={(event) => setPointId(event.detail.value)}>
            <EmsSelectOption value="">选择 modbus 点位</EmsSelectOption>
            {powerPoints.map((node) => <EmsSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</EmsSelectOption>)}
          </EmsSelect>
          <EmsButton busy={busy} busyLabel="添加中…" onClick={() => void add()}>添加</EmsButton>
        </div>
        {powerPoints.length === 0 && <p className="m-0 text-xs text-ems-amber">还没有 modbus 点位，请先在“通道”页配置功率设定点位。</p>}
      </div>}
    </IonCardContent>
  </EmsCard>
}
