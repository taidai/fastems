import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSelect, EmsSelectOption, EmsSectionHeader, InlineNotice } from "./components/ems"
import { fetchCapabilityInputs, saveCapabilityInputs, type CapabilityInputs } from "./api"
import type { Node } from "./api"

const empty: CapabilityInputs = {
  soc_point_id: "", temp_point_id: "", charge_limit_point_id: "", discharge_limit_point_id: "",
  soc_min: 10, soc_max: 90, temp_derate: 45, temp_limit: 55,
  max_age_ms: 5000, ramp_rate_kw: 10, deadband_kw: 5,
}

type CapabilityStatus = { allowed: boolean; direction: string; power_max_kw?: number; charge_max_kw?: number; discharge_max_kw?: number; limit_reason?: string }

// 动态能力来源：从带时效的 BMS/SOC/温度点位分方向刷新能力（接入建模层）。
export default function CapabilityInputsPanel({ token, unitId, nodes, onError }: {
  token: string
  unitId: string
  nodes: Node[]
  onError: (message: string) => void
}) {
  const [inputs, setInputs] = useState<CapabilityInputs>(empty)
  const [configured, setConfigured] = useState(false)
  const [busy, setBusy] = useState(false)
  const points = nodes.filter((node) => node.type === "modbusIo" || node.type === "variable")
  const unit = nodes.find((node) => node.id === unitId)
  let capability: CapabilityStatus | null = null
  const capabilityText = unit?.points.find((point) => point.type === "capability")?.text
  if (capabilityText) {
    try { capability = JSON.parse(capabilityText) as CapabilityStatus } catch { capability = null }
  }

  useEffect(() => {
    void fetchCapabilityInputs(token, unitId)
      .then((result) => { if (result.configured) { setInputs({ ...empty, ...result.inputs }); setConfigured(true) } })
      .catch(() => {})
  }, [token, unitId])

  async function save() {
    if (!inputs.soc_point_id) { onError("请选择 SOC 点位"); return }
    if (inputs.max_age_ms <= 0 || inputs.ramp_rate_kw <= 0 || inputs.deadband_kw <= 0) { onError("数据时效、爬坡和死区必须大于 0"); return }
    setBusy(true)
    try {
      await saveCapabilityInputs(token, unitId, inputs)
      setConfigured(true)
    } catch (error) {
      onError(error instanceof Error ? error.message : "保存失败")
    } finally {
      setBusy(false)
    }
  }

  const pointOptions = points.map((node) => <EmsSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</EmsSelectOption>)
  return <EmsCard>
    <IonCardContent className="grid gap-4">
      <EmsSectionHeader title="动态能力来源" description="充/放能力分别受 BMS 允许值、SOC 和温度约束；关键数据过期时关闭对应方向。"
        status={configured ? <span className="text-xs text-ems-mint">已启用</span> : undefined} />
      {capability && <InlineNotice tone={capability.allowed ? (capability.limit_reason ? "warning" : "success") : "warning"}>
        当前可用：充电 {capability.charge_max_kw ?? capability.power_max_kw ?? 0} kW · 放电 {capability.discharge_max_kw ?? capability.power_max_kw ?? 0} kW{capability.limit_reason ? ` · ${capability.limit_reason}` : ""}
      </InlineNotice>}
      <div className="grid gap-3 sm:grid-cols-2">
        <EmsSelect label="SOC 点位" aria-label="SOC 点位" value={inputs.soc_point_id} onIonChange={(event) => setInputs({ ...inputs, soc_point_id: event.detail.value })}>
          <EmsSelectOption value="">选择 SOC 点位</EmsSelectOption>{pointOptions}
        </EmsSelect>
        <EmsSelect label="温度点位（可选）" aria-label="温度点位" value={inputs.temp_point_id} onIonChange={(event) => setInputs({ ...inputs, temp_point_id: event.detail.value })}>
          <EmsSelectOption value="">不配置</EmsSelectOption>{pointOptions}
        </EmsSelect>
        <EmsSelect label="BMS 允许充电功率点位（可选）" aria-label="BMS 允许充电功率点位" value={inputs.charge_limit_point_id} onIonChange={(event) => setInputs({ ...inputs, charge_limit_point_id: event.detail.value })}>
          <EmsSelectOption value="">按额定功率</EmsSelectOption>{pointOptions}
        </EmsSelect>
        <EmsSelect label="BMS 允许放电功率点位（可选）" aria-label="BMS 允许放电功率点位" value={inputs.discharge_limit_point_id} onIonChange={(event) => setInputs({ ...inputs, discharge_limit_point_id: event.detail.value })}>
          <EmsSelectOption value="">按额定功率</EmsSelectOption>{pointOptions}
        </EmsSelect>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <EmsInput label="SOC 下限（%）" aria-label="SOC 下限" type="number" value={inputs.soc_min} onIonInput={(event) => setInputs({ ...inputs, soc_min: Number(event.detail.value) })} />
        <EmsInput label="SOC 上限（%）" aria-label="SOC 上限" type="number" value={inputs.soc_max} onIonInput={(event) => setInputs({ ...inputs, soc_max: Number(event.detail.value) })} />
        <EmsInput label="温度降额起点（℃）" aria-label="温度降额起点" type="number" value={inputs.temp_derate} onIonInput={(event) => setInputs({ ...inputs, temp_derate: Number(event.detail.value) })} />
        <EmsInput label="温度上限（℃）" aria-label="温度上限" type="number" value={inputs.temp_limit} onIonInput={(event) => setInputs({ ...inputs, temp_limit: Number(event.detail.value) })} />
        <EmsInput label="数据最大年龄（毫秒）" aria-label="数据最大年龄" type="number" value={inputs.max_age_ms} onIonInput={(event) => setInputs({ ...inputs, max_age_ms: Number(event.detail.value) })} />
        <EmsInput label="爬坡速率（kW/s）" aria-label="爬坡速率" type="number" value={inputs.ramp_rate_kw} onIonInput={(event) => setInputs({ ...inputs, ramp_rate_kw: Number(event.detail.value) })} />
        <EmsInput label="调节死区（kW）" aria-label="调节死区" type="number" value={inputs.deadband_kw} onIonInput={(event) => setInputs({ ...inputs, deadband_kw: Number(event.detail.value) })} />
      </div>
      {!inputs.temp_point_id && <InlineNotice tone="info">未配置温度点位时只按 SOC 和 BMS 约束能力，不做温度降额。</InlineNotice>}
      <div className="flex justify-end"><EmsButton busy={busy} busyLabel="保存中…" onClick={() => void save()}>保存能力来源</EmsButton></div>
    </IonCardContent>
  </EmsCard>
}
