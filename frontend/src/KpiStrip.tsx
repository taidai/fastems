import { sumByKeywords } from "./EnergyFlow"
import { Node } from "./api"
import Icon, { type IconName } from "./Icons"

export type KpiKey = "revenue" | "ess-revenue" | "efficiency" | "fee" | "throughput" | "energy" | "today-load" | "demand"

export type KpiThreshold = { value: number; direction: "over" | "under" }

export type KpiConfig = {
  order: KpiKey[]
  thresholds: Partial<Record<KpiKey, KpiThreshold>>
}

const DEFAULT_ORDER: KpiKey[] = ["revenue", "ess-revenue", "efficiency", "fee", "throughput", "energy", "today-load", "demand"]

export const DEFAULT_KPI_CONFIG: KpiConfig = { order: DEFAULT_ORDER, thresholds: {} }

const STORAGE_KEY = "fastiot-kpi-config"

export function loadKpiConfig(): KpiConfig {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}")
    const order = Array.isArray(parsed.order)
      ? parsed.order.filter((key: KpiKey): key is KpiKey => DEFAULT_ORDER.includes(key))
      : DEFAULT_ORDER
    const thresholds: KpiConfig["thresholds"] = {}
    if (parsed.thresholds && typeof parsed.thresholds === "object") {
      for (const key of DEFAULT_ORDER) {
        const entry = parsed.thresholds[key]
        if (!entry || typeof entry !== "object") continue
        const value = Number(entry.value)
        if (!Number.isFinite(value)) continue
        thresholds[key] = { value, direction: entry.direction === "under" ? "under" : "over" }
      }
    }
    return { order: order.length ? order : DEFAULT_ORDER, thresholds }
  } catch {
    return DEFAULT_KPI_CONFIG
  }
}

export function saveKpiConfig(config: KpiConfig) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(config))
}

type Card = {
  key: KpiKey
  icon: IconName
  label: string
  value: number | null
  digits: number
  unit: string
  note: string
  keywords: string[]
  compact?: boolean
}

function buildCards(points: Node[], tariff: number | null): Card[] {
  const chargeEnergy = sumByKeywords(points, ["储能充电", "充电量"], "kWh")
  const dischargeEnergy = sumByKeywords(points, ["储能放电", "放电量"], "kWh")
  const todayLoad = sumByKeywords(points, ["今日用电", "用电量"], "kWh")
  const feePoint = sumByKeywords(points, ["今日电费"], "元")
  const computedFee = todayLoad !== null && tariff !== null ? todayLoad * tariff : null
  const todayFee = feePoint ?? computedFee
  const feeNote = feePoint !== null ? "按已接入电费点位" : computedFee !== null ? `${tariff?.toFixed(2)} 元/kWh × 今日用电` : tariff === null ? "未设置电价" : "无今日用电点位"
  const number = (value: number | null, digits = 2) => value === null ? "—" : value.toFixed(digits)
  return [
    { key: "revenue", icon: "coin", label: "总收益", value: sumByKeywords(points, ["总收益", "累计收益"], "元"), digits: 0, unit: "元", note: "按已接入收益点位", keywords: ["总收益", "累计收益"] },
    { key: "ess-revenue", icon: "yuan", label: "储能收益", value: sumByKeywords(points, ["储能收益"], "元"), digits: 0, unit: "元", note: "放电收益 − 充电成本", keywords: ["储能收益"] },
    { key: "efficiency", icon: "efficiency", label: "转换效率", value: sumByKeywords(points, ["转换效率", "综合效率"], "%"), digits: 1, unit: "%", note: "综合 / 当日", keywords: ["转换效率", "综合效率"] },
    { key: "fee", icon: "bolt", label: "今日电费", value: todayFee, digits: 2, unit: "元", note: feeNote, keywords: ["今日电费", "今日用电", "用电量"] },
    { key: "throughput", icon: "battery", label: "储能吞吐", value: null, digits: 0, unit: "", note: `充 ${number(chargeEnergy)} / 放 ${number(dischargeEnergy)} kWh`, keywords: ["储能充电", "储能放电"], compact: true },
    { key: "energy", icon: "solar", label: "光伏发电量", value: sumByKeywords(points, ["光伏发电量", "发电量"], "kWh"), digits: 2, unit: "kWh", note: "", keywords: ["光伏发电量", "发电量"] },
    { key: "today-load", icon: "home", label: "今日用电", value: sumByKeywords(points, ["今日用电", "用电量"], "kWh"), digits: 2, unit: "kWh", note: "", keywords: ["今日用电", "用电量"] },
    { key: "demand", icon: "bars", label: "最大需量", value: sumByKeywords(points, ["最大需量", "需量"], "kW"), digits: 1, unit: "kW", note: "", keywords: ["最大需量", "需量"] },
  ]
}

function thresholdHit(card: Card, threshold?: KpiThreshold) {
  if (!threshold || card.value === null) return false
  return threshold.direction === "over" ? card.value >= threshold.value : card.value <= threshold.value
}

export default function KpiStrip({ points, tariff, config, onOpen }: {
  points: Node[]
  tariff: number | null
  config: KpiConfig
  onOpen: (keywords: string[]) => void
}) {
  const cards = buildCards(points, tariff)
  const byKey = new Map(cards.map((card) => [card.key, card]))
  const ordered = config.order.map((key) => byKey.get(key)).filter((card): card is Card => Boolean(card))
  return <section className="kpi-strip" aria-label="站点关键指标">
    {ordered.map((card) => {
      const hit = thresholdHit(card, config.thresholds[card.key])
      return <button
        type="button"
        key={card.key}
        className={`kpi-card ${card.key} ${card.compact ? "compact" : ""} ${hit ? "threshold-hit" : ""}`}
        onClick={() => onOpen(card.keywords)}
        title={card.keywords.length ? `查看${card.label}点位` : `打开${card.label}页面`}
      >
        <span className="kpi-icon" aria-hidden="true"><Icon name={card.icon} size={38} /></span>
        <span className="kpi-copy"><span className="kpi-label">{card.label}</span>{!card.compact && <strong className="kpi-value">{card.value === null || card.value === undefined ? "—" : card.value.toFixed(card.digits)}<small> {card.unit}</small></strong>}</span>
        <span className="kpi-note">{card.note}</span>
      </button>
    })}
  </section>
}
