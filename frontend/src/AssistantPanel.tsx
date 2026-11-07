import { FormEvent, useEffect, useMemo, useRef, useState } from "react"
import { nameOf, Node, point } from "./api"
import { sumByKeywords } from "./EnergyFlow"
import Icon from "./Icons"

type Message = { role: "assistant" | "user"; text: string }

const suggestions = [
  "现在的功率平衡怎么样？",
  "哪些点位离线或过期？",
  "报警和规则在做什么？",
  "光伏和储能电量多少？",
]

const powerWords = [
  { key: "pv", label: "光伏", words: ["光伏", "PV"] },
  { key: "ess", label: "储能", words: ["储能", "电池", "ESS"] },
  { key: "grid", label: "电网", words: ["电网", "并网", "配电", "关口"] },
  { key: "load", label: "负载", words: ["负载", "负荷", "用电"] },
  { key: "charge", label: "充电", words: ["充电", "桩"] },
]

const fmt = (value: number | null, unit = "kW") => value === null ? "—" : `${value.toFixed(1)} ${unit}`

function freshRows(points: Node[]) {
  const now = Date.now()
  return points
    .filter((node) => node.type === "modbusIo" || node.type === "variable")
    .map((node) => {
      const value = point(node, "value")
      const time = value?.time ? new Date(value.time).getTime() : NaN
      const numeric = value?.value
      return {
        name: nameOf(node),
        hasValue: numeric !== undefined && Number.isFinite(numeric),
        fresh: Number.isFinite(time) && now - time <= 15000,
      }
    })
}

export default function AssistantPanel({ points, alarmCount, errors, rules, activeRules, stationName, open, onClose }: {
  points: Node[]
  alarmCount: number
  errors: number
  rules: number
  activeRules: number
  stationName: string
  open: boolean
  onClose: () => void
}) {
  const [messages, setMessages] = useState<Message[]>(() => [{
    role: "assistant",
    text: `我是本机的能源管家。只读取这台 EMS 的实时点位、规则和报警来回答，不联网、不编造。当前站点：${stationName}。`,
  }])
  const [input, setInput] = useState("")
  const body = useRef<HTMLDivElement>(null)
  const rows = useMemo(() => freshRows(points), [points])

  useEffect(() => {
    if (!open) return
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") onClose() }
    window.addEventListener("keydown", escape)
    return () => window.removeEventListener("keydown", escape)
  }, [open, onClose])

  function answer(question: string): string {
    const q = question.trim().toLowerCase()
    if (!q) return "请先输入一个问题。"

    if (/报警|告警|异常|故障/.test(q) && /规则|调度|决策|自动/.test(q)) {
      return `当前有 ${alarmCount} 条活动报警，${errors} 个对象上报错误或命令失败。`
        + (rules ? `决策表共 ${rules} 张，其中 ${activeRules} 张已启用；未启用的只保留草稿。` : "当前还没有决策表。")
    }

    if (/报警|告警|异常|故障/.test(q)) {
      const issue = errors ? `另有 ${errors} 个对象上报错误或命令失败。` : "没有对象上报错误或命令失败。"
      return alarmCount
        ? `当前有 ${alarmCount} 条活动报警。${issue}到“报警与事件”页可以逐条确认和处理。`
        : `当前没有活动报警。${issue}`
    }

    if (/离线|过期|质量|刷新|点位|采集/.test(q)) {
      const missing = rows.filter((row) => !row.hasValue)
      const stale = rows.filter((row) => row.hasValue && !row.fresh)
      const names = [...missing, ...stale].slice(0, 3).map((row) => row.name)
      if (!missing.length && !stale.length) return `已检查 ${rows.length} 个点位，全部有实时值且在 15 秒内刷新。`
      return `共 ${rows.length} 个点位：${missing.length} 个没有值，${stale.length} 个超过 15 秒未刷新。`
        + (names.length ? `例如：${names.join("、")}。` : "")
        + "可以在“点位表”按质量筛选后查看通道和最后一次刷新时间。"
    }

    if (/规则|调度|决策|自动/.test(q)) {
      return rules
        ? `共有 ${rules} 张决策表，其中 ${activeRules} 张已启用。未启用的表只保留草稿，不参与现场调度。`
        : "当前没有决策表。需要自动控制时，先在“决策表”页建立并发布规则。"
    }

    if (/光伏|发电|储能|电池|电量|kwh|度电/.test(q)) {
      const pv = sumByKeywords(points, ["光伏", "PV"], "kWh")
      const ess = sumByKeywords(points, ["储能", "电池", "ESS"], "kWh")
      if (pv === null && ess === null) return "没有找到带 kWh 单位的光伏或储能电量点位，因此不能给出电量数字。"
      return `累计电量：光伏 ${fmt(pv, "kWh")}，储能 ${fmt(ess, "kWh")}。这是点位的当前累计值，不是按今天重新统计的日发电量。`
    }

    if (/功率|能源|能流|平衡|负载|负荷|电网|并网|充电|现在|当前/.test(q)) {
      const values = powerWords.map((item) => `${item.label} ${fmt(sumByKeywords(points, item.words))}`)
      const bound = powerWords.filter((item) => sumByKeywords(points, item.words) !== null).length
      const balance = `光伏 ${fmt(sumByKeywords(points, powerWords[0].words))} + 储能 ${fmt(sumByKeywords(points, powerWords[1].words))} + 电网 ${fmt(sumByKeywords(points, powerWords[2].words))}`
      return bound
        ? `${balance}；负载 ${fmt(sumByKeywords(points, powerWords[3].words))}，充电 ${fmt(sumByKeywords(points, powerWords[4].words))}。功率符号按点位原值直接相加，正负方向以设备点表为准。`
        : `能流节点还没有绑定功率点位，无法计算功率平衡。当前共 ${points.length} 个点位，可以到“点位表”检查名称和单位。`
    }

    if (/你是谁|能做什么|帮助|怎么用/.test(q)) {
      return "我可以回答四类问题：当前功率与能流、活动报警与错误、点位离线或过期、规则启用情况。所有答案只来自本机数据。"
    }

    return "这个问题超出我能读取的范围。我只能回答功率、报警、点位质量和规则状态，不会推测设备行为或给出未经点位验证的结论。"
  }

  function send(question: string) {
    const text = question.trim()
    if (!text) return
    setMessages((current) => [...current, { role: "user", text }, { role: "assistant", text: answer(text) }])
    setInput("")
    window.setTimeout(() => { if (body.current) body.current.scrollTop = body.current.scrollHeight }, 0)
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    send(input)
  }

  if (!open) return null
  return <aside className="assistant-panel" aria-label="能源管家">
    <header className="assistant-head">
      <span className="assistant-mark">AI</span>
      <div><strong>能源管家</strong><small>本地规则应答 · 不联网</small></div>
      <button className="icon-button" aria-label="关闭能源管家" onClick={onClose}><Icon name="close" size={16} /></button>
    </header>
    <div className="assistant-body" ref={body}>
      {messages.map((message, index) => <div key={index} className={`assistant-msg ${message.role}`}>
        {message.role === "assistant" && <span className="assistant-mark small">AI</span>}
        <p>{message.text}</p>
      </div>)}
      {messages.length === 1 && <div className="assistant-suggestions">
        {suggestions.map((item) => <button key={item} onClick={() => send(item)}>{item}</button>)}
      </div>}
    </div>
    <form className="assistant-input" onSubmit={submit}>
      <input value={input} onChange={(event) => setInput(event.target.value)} placeholder="问当前功率、报警、点位或规则" aria-label="向能源管家提问" />
      <button type="submit" aria-label="发送"><Icon name="send" size={16} /></button>
    </form>
    <p className="assistant-note">只读当前站点数据；不会下发控制命令，也不会连接云端模型。</p>
  </aside>
}
