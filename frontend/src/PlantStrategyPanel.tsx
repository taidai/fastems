import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsOverlay, EmsSectionHeader, EmsSelect, EmsSelectOption, InlineNotice, StatusBadge } from "./components/ems"
import { fetchPlants, fetchPlantPCC, savePlantPCC } from "./api"
import type { Node } from "./api"

// PlantStrategyPanel 配置电站级策略（防逆流/需量）：选 PCC 主表点位 +
// 反送/需量上限，绑定后 EnergyScheduler 每周期自动闭环。与单柜规则分离。
export default function PlantStrategyPanel({ token, nodes, onError }: {
  token: string
  nodes: Node[]
  onError: (message: string) => void
}) {
  const [plantId, setPlantId] = useState("")
  const [meterId, setMeterId] = useState("")
  const [reverseLimit, setReverseLimit] = useState(0)
  const [demandLimit, setDemandLimit] = useState(0)
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const meters = nodes.filter((node) => node.type === "variable" || node.type === "modbusIo")

  useEffect(() => {
    void fetchPlants(token).then((result) => {
      const first = result.plants?.[0]
      if (first) setPlantId(first.id)
    }).catch(() => onError("电站列表读取失败"))
  }, [token])

  useEffect(() => {
    if (!plantId) return
    void fetchPlantPCC(token, plantId).then((result) => {
      if (result.configured && result.binding) {
        setMeterId(result.binding.MeterPointID)
        setReverseLimit(result.binding.LowerLimitKW)
        setDemandLimit(result.binding.UpperLimitKW)
        setSaved(true)
      }
    }).catch(() => {})
  }, [token, plantId])

  async function save() {
    if (!plantId) { onError("未选择电站"); return }
    if (!meterId) { onError("请选择 PCC 主表点位"); return }
    setBusy(true)
    try {
      await savePlantPCC(token, plantId, { MeterPointID: meterId, Sign: 1, MaxAgeMS: 15000, UpperLimitKW: demandLimit, LowerLimitKW: reverseLimit })
      setSaved(true)
    } catch (error) {
      onError(error instanceof Error ? error.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  return <EmsCard>
    <IonCardContent className="grid gap-4">
      <EmsSectionHeader title="电站策略" description="选并网点主表，设反送与需量上限，保存后每秒自动闭环。"
        status={saved ? <StatusBadge tone="success">已启用</StatusBadge> : undefined} />
      <EmsSelect label="PCC 主表" aria-label="PCC 主表点位" value={meterId} onIonChange={(event) => setMeterId(event.detail.value)}>
        <EmsSelectOption value="">选择关口表点位</EmsSelectOption>
        {meters.map((node) => <EmsSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</EmsSelectOption>)}
      </EmsSelect>
      <div className="grid gap-3 sm:grid-cols-2">
        <EmsInput label="反送上限（kW，0=禁止反送）" aria-label="反送上限" type="number" value={reverseLimit} onIonInput={(event) => setReverseLimit(Number(event.detail.value))} />
        <EmsInput label="需量上限（kW，0=不限）" aria-label="需量上限" type="number" value={demandLimit} onIonInput={(event) => setDemandLimit(Number(event.detail.value))} />
      </div>
      <InlineNotice tone="info">启用后，电站按反送上限 {reverseLimit} kW、需量上限 {demandLimit || "不限"} kW 每秒自动调节储能充放。</InlineNotice>
      <div className="flex justify-end"><EmsButton busy={busy} busyLabel="保存中…" onClick={() => {
        if (!plantId) { onError("未选择电站"); return }
        if (!meterId) { onError("请选择 PCC 主表点位"); return }
        setConfirm(true)
      }}>{saved ? "更新策略" : "启用策略"}</EmsButton></div>
    </IonCardContent>

    <EmsOverlay open={confirm} title={saved ? "更新电站策略" : "启用电站策略"} busy={busy} onClose={() => setConfirm(false)}
      actions={<>
        <EmsButton variant="secondary" onClick={() => setConfirm(false)}>取消</EmsButton>
        <EmsButton onClick={() => { setConfirm(false); void save() }}>确认启用</EmsButton>
      </>}>
      <p className="m-0 text-sm leading-6 text-ems-ink">启用后，该电站将按反送上限 {reverseLimit} kW、需量上限 {demandLimit || "不限"} kW 每秒自动调节储能充放。确认{saved ? "更新" : "启用"}？</p>
    </EmsOverlay>
  </EmsCard>
}
