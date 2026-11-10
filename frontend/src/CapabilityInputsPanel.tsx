import { useEffect, useState } from "react"
import { IonCardContent } from "@ionic/react"
import { EmsButton, EmsCard, EmsInput, EmsSelect, EmsSelectOption, EmsSectionHeader, InlineNotice } from "./components/ems"
import { fetchCapabilityInputs, saveCapabilityInputs, type CapabilityInputs } from "./api"
import type { Node } from "./api"

const empty: CapabilityInputs = { soc_point_id: "", temp_point_id: "", soc_min: 10, soc_max: 90, temp_derate: 45, temp_limit: 55 }

// 动态能力来源：从 SOC/温度点位刷新 EnergyUnit 的可用能力（接入建模层）。
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

  useEffect(() => {
    void fetchCapabilityInputs(token, unitId)
      .then((result) => { if (result.configured) { setInputs(result.inputs); setConfigured(true) } })
      .catch(() => {})
  }, [token, unitId])

  async function save() {
    if (!inputs.soc_point_id) { onError("请选择 SOC 点位"); return }
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

  return <EmsCard>
    <IonCardContent className="grid gap-4">
      <EmsSectionHeader title="动态能力来源" description="每秒读取 SOC/温度刷新可用功率；SOC 越界禁相应方向，温度超限降额或停机。"
        status={configured ? <span className="text-xs text-ems-mint">已启用</span> : undefined} />
      <div className="grid gap-3 sm:grid-cols-2">
        <EmsSelect label="SOC 点位" aria-label="SOC 点位" value={inputs.soc_point_id} onIonChange={(event) => setInputs({ ...inputs, soc_point_id: event.detail.value })}>
          <EmsSelectOption value="">选择 SOC 点位</EmsSelectOption>
          {points.map((node) => <EmsSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</EmsSelectOption>)}
        </EmsSelect>
        <EmsSelect label="温度点位（可选）" aria-label="温度点位" value={inputs.temp_point_id} onIonChange={(event) => setInputs({ ...inputs, temp_point_id: event.detail.value })}>
          <EmsSelectOption value="">不配置</EmsSelectOption>
          {points.map((node) => <EmsSelectOption key={node.id} value={node.id}>{node.points.find((p) => p.type === "description")?.text || node.id}</EmsSelectOption>)}
        </EmsSelect>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <EmsInput label="SOC 下限（%）" aria-label="SOC 下限" type="number" value={inputs.soc_min} onIonInput={(event) => setInputs({ ...inputs, soc_min: Number(event.detail.value) })} />
        <EmsInput label="SOC 上限（%）" aria-label="SOC 上限" type="number" value={inputs.soc_max} onIonInput={(event) => setInputs({ ...inputs, soc_max: Number(event.detail.value) })} />
        <EmsInput label="温度降额起点（℃）" aria-label="温度降额起点" type="number" value={inputs.temp_derate} onIonInput={(event) => setInputs({ ...inputs, temp_derate: Number(event.detail.value) })} />
        <EmsInput label="温度上限（℃）" aria-label="温度上限" type="number" value={inputs.temp_limit} onIonInput={(event) => setInputs({ ...inputs, temp_limit: Number(event.detail.value) })} />
      </div>
      {!inputs.temp_point_id && <InlineNotice tone="info">未配置温度点位时只按 SOC 约束能力，不做温度降额。</InlineNotice>}
      <div className="flex justify-end"><EmsButton busy={busy} busyLabel="保存中…" onClick={() => void save()}>保存能力来源</EmsButton></div>
    </IonCardContent>
  </EmsCard>
}
