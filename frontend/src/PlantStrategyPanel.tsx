import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsOverlay, EmsSectionHeader, EmsSelect, EmsSelectOption, InlineNotice, StatusBadge } from "./components/ems"
import { fetchPlants, fetchPlantPCC, savePlantPCC, fetchPlantStrategy, savePlantStrategy } from "./api"
import type { Node, TariffPeriod } from "./api"

const toMin = (text: string) => {
  const [h, m] = text.split(":").map(Number)
  return Number.isFinite(h) && Number.isFinite(m) ? h * 60 + m : 0
}
const toTime = (minutes: number) => `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`

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
  const [pvPower, setPvPower] = useState(0)
  const [valleyStart, setValleyStart] = useState("00:00")
  const [valleyEnd, setValleyEnd] = useState("08:00")
  const [peakStart, setPeakStart] = useState("18:00")
  const [peakEnd, setPeakEnd] = useState("22:00")
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
    void fetchPlantStrategy(token, plantId).then((result) => {
      if (!result.configured) return
      setPvPower(result.strategy.peak_valley_power_kw || 0)
      const valley = result.strategy.periods?.find((period) => period.Kind === "valley")
      const peak = result.strategy.periods?.find((period) => period.Kind === "peak")
      if (valley) { setValleyStart(toTime(valley.StartMin)); setValleyEnd(toTime(valley.EndMin)) }
      if (peak) { setPeakStart(toTime(peak.StartMin)); setPeakEnd(toTime(peak.EndMin)) }
    }).catch(() => {})
  }, [token, plantId])

  async function save() {
    if (!plantId) { onError("未选择电站"); return }
    if (!meterId) { onError("请选择 PCC 主表点位"); return }
    setBusy(true)
    try {
      await savePlantPCC(token, plantId, { MeterPointID: meterId, Sign: 1, MaxAgeMS: 15000, UpperLimitKW: demandLimit, LowerLimitKW: reverseLimit })
      const periods: TariffPeriod[] = []
      if (pvPower > 0) {
        periods.push({ StartMin: toMin(valleyStart), EndMin: toMin(valleyEnd), Kind: "valley", Price: 0 })
        periods.push({ StartMin: toMin(peakStart), EndMin: toMin(peakEnd), Kind: "peak", Price: 0 })
      }
      await savePlantStrategy(token, plantId, { peak_valley_power_kw: pvPower, periods })
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
      <div className="grid gap-3 rounded-xl border border-ems-line p-3">
        <strong className="text-sm text-ems-ink">削峰填谷（分时电价）</strong>
        <p className="m-0 text-xs text-ems-muted">谷时段充电、峰时段放电；功率为 0 表示不启用。优先级低于防逆流与需量。</p>
        <EmsInput label="充放功率（kW）" aria-label="削峰填谷功率" type="number" value={pvPower} onIonInput={(event) => setPvPower(Number(event.detail.value))} />
        <div className="grid gap-3 sm:grid-cols-2">
          <EmsInput label="谷时段开始" aria-label="谷时段开始" type="time" value={valleyStart} onIonInput={(event) => setValleyStart(event.detail.value || "00:00")} />
          <EmsInput label="谷时段结束" aria-label="谷时段结束" type="time" value={valleyEnd} onIonInput={(event) => setValleyEnd(event.detail.value || "08:00")} />
          <EmsInput label="峰时段开始" aria-label="峰时段开始" type="time" value={peakStart} onIonInput={(event) => setPeakStart(event.detail.value || "18:00")} />
          <EmsInput label="峰时段结束" aria-label="峰时段结束" type="time" value={peakEnd} onIonInput={(event) => setPeakEnd(event.detail.value || "22:00")} />
        </div>
      </div>
      <InlineNotice tone="info">启用后，电站按硬优先级自动调节：防逆流 → 需量（{demandLimit || "不限"} kW）→ 削峰填谷（{pvPower > 0 ? `${pvPower} kW` : "未启用"}）。</InlineNotice>
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
      <p className="m-0 text-sm leading-6 text-ems-ink">启用后，该电站将按硬优先级自动调节：防逆流（反送 {reverseLimit} kW）→ 需量（{demandLimit || "不限"} kW）→ 削峰填谷（{pvPower > 0 ? `${pvPower} kW` : "未启用"}）。确认{saved ? "更新" : "启用"}？</p>
    </EmsOverlay>
  </EmsCard>
}
