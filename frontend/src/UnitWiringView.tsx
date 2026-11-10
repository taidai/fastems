import { useEffect, useState } from "react"
import { EmsButton, EmsSectionHeader, EmptyState, InlineNotice, StatusBadge } from "./components/ems"
import UnitDevicePanel from "./UnitDevicePanel"
import CapabilityInputsPanel from "./CapabilityInputsPanel"
import { fetchEnergyUnits } from "./api"
import type { EnergyUnitSummary, Node } from "./api"

// 能源设备接线（接入建模层）：为能源设备挂 PCS，并映射功率设定点位。
// 与 EMS 业务层的"能源设备/策略"分离——点位与映射属于 IoT/SCADA 层。
export default function UnitWiringView({ token, nodes, onError }: {
  token: string
  nodes: Node[]
  onError: (message: string) => void
}) {
  const [units, setUnits] = useState<EnergyUnitSummary[]>([])
  const [selected, setSelected] = useState("")

  useEffect(() => {
    void fetchEnergyUnits(token)
      .then((result) => setUnits(result.units || []))
      .catch(() => onError("能源设备列表读取失败"))
  }, [token, onError])

  const current = units.find((unit) => unit.id === selected)

  return <section className="grid gap-4">
    <EmsSectionHeader kicker="设备映射" title="能源设备接线" description="把 PCS 挂到能源设备并映射功率设定点位；EMS 策略不再接触点位地址。"
      status={current ? <StatusBadge tone="info">{current.description || current.asset_code || current.id}</StatusBadge> : undefined}
      actions={current ? <EmsButton variant="secondary" onClick={() => setSelected("")}>返回能源设备列表</EmsButton> : undefined} />

    {!current && <InlineNotice tone="info">先在左侧选择能源设备。点位地址、寄存器与映射关系只在这一层出现，运行与策略页面不显示。</InlineNotice>}

    {!current && (units.length === 0
      ? <EmptyState title="还没有能源设备" description="先在“EMS 业务 → 能源设备”创建柜体，再回到这里完成设备接线。" />
      : <div className="grid gap-2">
        {units.map((unit) => <button key={unit.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-ems-line bg-white/80 px-4 py-3 text-left" onClick={() => setSelected(unit.id)}>
          <span className="grid gap-1"><strong className="text-sm text-ems-ink">{unit.description || unit.asset_code || unit.id}</strong><span className="text-xs text-ems-muted">{unit.asset_code || unit.id}</span></span>
          <span className="text-xs text-ems-accent">接线 →</span>
        </button>)}
      </div>)}

    {current && <>
      <UnitDevicePanel token={token} unitId={current.id} nodes={nodes} onError={onError} />
      <CapabilityInputsPanel token={token} unitId={current.id} nodes={nodes} onError={onError} />
    </>}
  </section>
}
