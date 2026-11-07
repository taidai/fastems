import { useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { ApiError, request } from "./api"
import type { StateValue, UnitModel, UnitModelConfig } from "./deviceTypes"

type WizardChannel = {
  id: string
  protocol?: string
  address?: string
  devices?: { server_id?: number; health: string; points?: unknown[] }[]
}

type ScanFound = { server_id: number; responded: boolean; latency_ms: number; probe: string; error: string }
type ScanResult = { scanned: number; found: ScanFound[]; errors: string[] }

const stepTitles = ["选择设备", "绑定 Profile", "映射点位", "只读核对", "记录验证人"]
const qualityNames: Record<string, string> = { good: "正常", stale: "过期", invalid: "无效", uninitialized: "无值" }
const healthNames: Record<string, string> = { healthy: "在线", degraded: "不稳定", offline: "离线", unknown: "未知" }
const roleNames: Record<string, string> = { telemetry: "遥测", status: "遥信", energy: "电度", setpoint: "定值", protection: "压板" }

function messageOf(cause: unknown) {
  if (cause instanceof ApiError) {
    if (cause.message === "channel_busy") return "该地址正被运行中的通道占用，无法扫描；可直接从设备列表选择站号。"
    if (cause.message === "invalid_range") return "站号范围无效：起始站号不能大于结束站号。"
  }
  return cause instanceof Error ? cause.message : "操作失败"
}

function formatValue(value?: StateValue) {
  if (!value) return "—"
  if (value.value.kind === "number") {
    const number = value.value.number
    if (number === undefined || !Number.isFinite(number)) return "—"
    return Math.abs(number) >= 1000 ? number.toFixed(0) : number.toFixed(2).replace(/\.?0+$/, "")
  }
  if (value.value.kind === "bool") return value.value.bool ? "合" : "分"
  if (value.value.kind === "enum") return value.value.enum || "—"
  return "—"
}

function timeText(value?: string) {
  if (!value) return "—"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString("zh-CN", { hour12: false })
}

export default function DeployWizard({ token, channels, onClose }: {
  token: string
  channels: WizardChannel[]
  onClose: () => void
}) {
  const [step, setStep] = useState(1)
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  const [channelID, setChannelID] = useState(channels[0]?.id || "")
  const [serverID, setServerID] = useState(1)
  const [baud, setBaud] = useState(9600)
  const [from, setFrom] = useState(1)
  const [to, setTo] = useState(16)
  const [scan, setScan] = useState<ScanResult | null>(null)

  const [model, setModel] = useState<UnitModel | null>(null)
  const [name, setName] = useState("")
  const [profileID, setProfileID] = useState("")
  const [pointMap, setPointMap] = useState<Record<string, string>>({})

  const [verified, setVerified] = useState(false)
  const [verifier, setVerifier] = useState("")
  const [verifiedAt, setVerifiedAt] = useState("")
  const [deviceSerial, setDeviceSerial] = useState("")
  const [verification, setVerification] = useState("")

  const [values, setValues] = useState<Record<string, StateValue>>({})

  const channel = channels.find((item) => item.id === channelID)
  const profile = useMemo(() => model?.profiles.find((item) => item.id === profileID), [model, profileID])
  const serverPoints = useMemo(() => (model?.points || []).filter((point) => point.server_id === serverID), [model, serverID])
  const requiredSpecs = useMemo(() => {
    const required = new Set<string>()
    for (const command of profile?.command_specs || []) {
      if (command.point) required.add(command.point)
      for (const parameter of command.parameters || []) {
        if (parameter.point) required.add(parameter.point)
      }
    }
    return required
  }, [profile])
  const mappedCount = useMemo(() => Object.values(pointMap).filter(Boolean).length, [pointMap])
  const missingRequired = useMemo(() =>
    [...requiredSpecs].filter((spec) => !pointMap[spec]), [requiredSpecs, pointMap])

  useEffect(() => {
    if (step !== 4) return
    let cancelled = false
    const deviceID = `${channelID}/${serverID}`
    const load = async () => {
      try {
        const result = await request<{ values: StateValue[] }>(token, "/ems/state")
        if (!cancelled) setValues(Object.fromEntries((result.values || []).filter((value) => value.device_id === deviceID).map((value) => [value.point_id, value])))
      } catch {
        // 核对页读取失败时保留上一次的值；错误会在其他请求中呈现。
      }
    }
    void load()
    const timer = window.setInterval(() => void load(), 2000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [step, token, channelID, serverID])

  async function runScan() {
    if (!channel) return
    setBusy(true)
    setError("")
    try {
      const result = await request<ScanResult>(token, "/ems/scan", "POST", {
        protocol: channel.protocol || "tcp", address: channel.address || "", baud,
        from, to, timeout_ms: 300,
      })
      setScan(result)
      const first = result.found.find((item) => item.responded)
      if (first) setServerID(first.server_id)
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy(false)
    }
  }

  async function loadModel() {
    if (!channelID || serverID < 1 || serverID > 247) {
      setError("先选择通道并填写 1-247 的站号。")
      return
    }
    setBusy(true)
    setError("")
    try {
      const result = await request<UnitModel>(token, `/modbus/${encodeURIComponent(channelID)}/units/${serverID}/model`)
      setModel(result)
      const config = result.model
      setName(config?.name || `站号 ${serverID}`)
      setProfileID(config?.profile_id || "")
      setPointMap(config?.profile_id ? { ...config.point_map } : {})
      setVerified(Boolean(config?.verified))
      setVerifier(config?.verifier || "")
      setVerifiedAt(config?.verified_at || "")
      setDeviceSerial(config?.device_serial || "")
      setVerification(config?.verification || "")
      setStep(2)
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy(false)
    }
  }

  function selectProfile(id: string) {
    setProfileID(id)
    if (!model) return
    if (id && id === model.model?.profile_id) {
      setPointMap({ ...model.model.point_map })
      return
    }
    const specs = model.profiles.find((item) => item.id === id)?.point_specs || []
    const next: Record<string, string> = {}
    for (const spec of specs) {
      const match = serverPoints.find((point) => point.name === spec.name)
      if (match) next[spec.name] = match.id
    }
    setPointMap(next)
  }

  function buildConfig(): UnitModelConfig {
    const cleaned: Record<string, string> = {}
    for (const [specName, pointID] of Object.entries(pointMap)) {
      if (pointID) cleaned[specName] = pointID
    }
    return {
      name: name.trim() || `站号 ${serverID}`,
      profile_id: profileID,
      business_node_id: model?.model?.business_node_id || "",
      point_map: cleaned,
      verified,
      verifier: verifier.trim(),
      verified_at: verifiedAt,
      device_serial: deviceSerial.trim(),
      verification: verification.trim(),
    }
  }

  async function saveModel(nextStep: number, markDone = false) {
    if (!profileID) {
      setError("先选择设备 Profile。")
      return
    }
    if (markDone && verified && (!verifier.trim() || !verifiedAt || !deviceSerial.trim())) {
      setError("标记已验证时必须填写验证人、日期和设备序列号。")
      return
    }
    setBusy(true)
    setError("")
    try {
      const result = await request<UnitModel>(token, `/modbus/${encodeURIComponent(channelID)}/units/${serverID}/model`, "PUT", buildConfig())
      setModel(result)
      if (markDone) setDone(true)
      else setStep(nextStep)
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy(false)
    }
  }

  const responded = scan?.found.filter((item) => item.responded) || []
  const mappedSpecs = (profile?.point_specs || []).filter((spec) => pointMap[spec.name])
  const badCount = mappedSpecs.filter((spec) => {
    const value = values[pointMap[spec.name]]
    return !value || value.quality !== "good"
  }).length

  return createPortal(<>
    <div className="comm-modal-backdrop" onClick={onClose} />
    <section className="comm-modal deploy-wizard" role="dialog" aria-label="接入向导">
      <div className="comm-modal-head">
        <div>
          <span className="section-kicker">引导式接入 · 每步复用现有接口</span>
          <h2>接入向导</h2>
        </div>
        <button className="icon-button" aria-label="关闭向导" onClick={onClose}>×</button>
      </div>
      <ol className="wizard-steps">
        {stepTitles.map((title, index) => <li key={title} className={done || step > index + 1 ? "done" : step === index + 1 ? "active" : ""}>
          <span>{index + 1}</span>{title}
        </li>)}
      </ol>
      {error && <div className="inline-error" role="alert">{error}</div>}

      {!done && step === 1 && <div className="wizard-body">
        <p className="comm-modal-note">选择要接入的通道和站号。通道运行中无法扫描；已在轮询的设备直接从列表选择。</p>
        <div className="comm-form-grid">
          <label>通道<select value={channelID} onChange={(event) => { setChannelID(event.target.value); setScan(null) }}>
            {channels.map((item) => <option key={item.id} value={item.id}>{item.address || item.id} · {item.devices?.length ?? 0} 台设备</option>)}
          </select></label>
          <label>站号<input type="number" min={1} max={247} value={serverID} onChange={(event) => setServerID(Number(event.target.value))} /></label>
        </div>
        {(channel?.devices?.length ?? 0) > 0 && <div className="wizard-pick-list">
          {(channel?.devices || []).map((device, index) => device.server_id === undefined ? null : <button key={index} className={`wizard-pick-row ${device.server_id === serverID ? "active" : ""}`} onClick={() => setServerID(device.server_id!)}>
            <strong>站号 {device.server_id}</strong>
            <span>{device.points?.length ?? 0} 点位 · {healthNames[device.health] || device.health}</span>
          </button>)}
        </div>}
        <div className="wizard-scan">
          <div className="wizard-scan-form">
            {channel?.protocol === "rtu" && <label>波特率<input type="number" value={baud} onChange={(event) => setBaud(Number(event.target.value))} /></label>}
            <label>起始站号<input type="number" min={1} max={247} value={from} onChange={(event) => setFrom(Number(event.target.value))} /></label>
            <label>结束站号<input type="number" min={1} max={247} value={to} onChange={(event) => setTo(Number(event.target.value))} /></label>
            <button className="secondary-button" disabled={busy || !channel} onClick={() => void runScan()}>{busy ? "扫描中…" : "扫描站号（只读）"}</button>
          </div>
          {scan && <div className="wizard-pick-list">
            {responded.map((item) => <button key={item.server_id} className={`wizard-pick-row ${item.server_id === serverID ? "active" : ""}`} onClick={() => setServerID(item.server_id)}>
              <strong>站号 {item.server_id}</strong>
              <span>{item.latency_ms} ms · {item.probe}</span>
            </button>)}
            {responded.length === 0 && <p className="inline-note">扫描 {scan.scanned} 个站号没有响应。</p>}
          </div>}
        </div>
      </div>}

      {!done && step === 2 && model && <div className="wizard-body">
        <p className="comm-modal-note">为该站号绑定设备 Profile。选择后按名称自动匹配点位，下一步可以逐个调整。</p>
        <div className="comm-form-grid">
          <label>设备名称<input value={name} onChange={(event) => setName(event.target.value)} placeholder={`站号 ${serverID}`} /></label>
          <label>设备 Profile<select value={profileID} onChange={(event) => selectProfile(event.target.value)}>
            <option value="">未选择</option>
            {model.profiles.filter((item) => (item.point_specs?.length ?? 0) > 0).map((item) => <option key={item.id} value={item.id}>{item.vendor} {item.model}{item.custom ? "（自定义）" : ""}{item.verified ? "" : "（未验证）"}</option>)}
          </select></label>
        </div>
        {(model.profile_errors || []).length > 0 && <div className="inline-note" role="alert">自定义 Profile 文件有 {model.profile_errors!.length} 个未加载：{model.profile_errors!.join("；")}</div>}
        {profile && <div className="wizard-profile-hint">
          <strong>{profile.vendor} {profile.model}</strong>
          <span>{profile.verified ? "已现场验证" : "未现场验证"} · {profile.point_specs?.length ?? 0} 个变量 · 自动匹配 {mappedCount} 个</span>
          {profile.verification && <small>{profile.verification}</small>}
        </div>}
        {serverPoints.length === 0 && <div className="inline-note" role="alert">该站号还没有点位。先在“点表校验”导入点表或在通道页新增点位，映射才有目标。</div>}
      </div>}

      {!done && step === 3 && model && profile && <div className="wizard-body">
        <p className="comm-modal-note">把 Profile 变量映射到该站号的点位。可写命令引用的变量必须映射。</p>
        <div className="unit-map-head"><div><strong>点位映射</strong><span>{mappedCount} / {profile.point_specs?.length || 0} 已映射</span></div>
          {missingRequired.length > 0 && <span className="unit-map-state warn">缺 {missingRequired.join("、")}</span>}
        </div>
        <div className="unit-map-wrap"><table className="unit-map-table">
          <thead><tr><th>Profile 变量</th><th>角色</th><th>映射到点位</th><th>状态</th></tr></thead>
          <tbody>{(profile.point_specs || []).map((spec) => {
            const mapped = pointMap[spec.name] || ""
            const required = requiredSpecs.has(spec.name)
            return <tr key={spec.name}>
              <td><strong>{spec.name}</strong>{spec.unit && <small>{spec.unit}</small>}</td>
              <td>{spec.role ? roleNames[spec.role] || spec.role : "—"}</td>
              <td><select value={mapped} onChange={(event) => setPointMap((current) => ({ ...current, [spec.name]: event.target.value }))}>
                <option value="">未映射</option>
                {serverPoints.map((point) => <option key={point.id} value={point.id}>{point.name} · {point.address}</option>)}
              </select></td>
              <td><span className={`unit-map-state ${mapped ? "ok" : required ? "warn" : ""}`}>{mapped ? "已映射" : required ? "需映射" : "可选"}</span></td>
            </tr>
          })}</tbody>
        </table></div>
      </div>}

      {!done && step === 4 && <div className="wizard-body">
        <p className="comm-modal-note">只读核对：下表每 2 秒刷新当前值与质量，不写设备。请与设备面板逐项核对。</p>
        <div className="wizard-check-summary">{mappedSpecs.length} 个已映射点位 · {badCount ? `${badCount} 个质量异常` : "质量全部正常"}</div>
        <div className="unit-map-wrap"><table className="unit-map-table">
          <thead><tr><th>Profile 变量</th><th>点位</th><th>当前值</th><th>质量</th><th>更新时间</th></tr></thead>
          <tbody>{mappedSpecs.map((spec) => {
            const point = serverPoints.find((item) => item.id === pointMap[spec.name])
            const value = values[pointMap[spec.name]]
            return <tr key={spec.name}>
              <td><strong>{spec.name}</strong>{spec.unit && <small>{spec.unit}</small>}</td>
              <td>{point ? `${point.name} · ${point.address}` : "—"}</td>
              <td>{formatValue(value)}{point?.unit ? ` ${point.unit}` : ""}</td>
              <td><span className={`device-quality ${value?.quality || "uninitialized"}`}>{qualityNames[value?.quality || "uninitialized"] || value?.quality}</span></td>
              <td>{timeText(value?.source_time)}</td>
            </tr>
          })}</tbody>
        </table></div>
        {mappedSpecs.length === 0 && <div className="inline-note" role="alert">还没有映射点位，返回上一步完成映射。</div>}
      </div>}

      {!done && step === 5 && <div className="wizard-body">
        <p className="comm-modal-note">记录现场验证人。验证记录保存到设备模型，可在设备详情和验证记录导出中追溯。</p>
        <div className="comm-form-grid">
          <label>验证人<input value={verifier} onChange={(event) => setVerifier(event.target.value)} placeholder="现场实施人员" /></label>
          <label>验证日期<input type="date" value={verifiedAt} onChange={(event) => setVerifiedAt(event.target.value)} /></label>
          <label>设备序列号<input value={deviceSerial} onChange={(event) => setDeviceSerial(event.target.value)} placeholder="铭牌序列号" /></label>
          <label className="comm-check"><input type="checkbox" checked={verified} onChange={(event) => setVerified(event.target.checked)} /> 已按 Profile 逐点核对</label>
          <label className="wizard-wide">验证说明<textarea value={verification} onChange={(event) => setVerification(event.target.value)} rows={3} placeholder="地址、缩放、字序、写权限、回读结果等" /></label>
        </div>
      </div>}

      {done && <div className="wizard-body wizard-done">
        <strong>接入完成</strong>
        <p>{name || `站号 ${serverID}`} · {profile ? `${profile.vendor} ${profile.model}` : profileID} · {mappedCount} 个点位已映射{verified ? ` · 已验证（${verifier} ${verifiedAt}）` : " · 未标记验证"}</p>
        <p className="inline-note">后续可在设备台账打开该设备复核，或在本页运行验收生成报告。模拟器或当前实例的检查不替代真机签收。</p>
      </div>}

      <div className="comm-modal-actions">
        {!done && step > 1 && <button className="secondary-button" disabled={busy} onClick={() => setStep(step - 1)}>上一步</button>}
        {!done && step === 1 && <button className="primary-button" disabled={busy || !channelID} onClick={() => void loadModel()}>{busy ? "读取中…" : "下一步"}</button>}
        {!done && step === 2 && <button className="primary-button" disabled={busy || !profileID} onClick={() => void saveModel(3)}>{busy ? "保存中…" : "保存绑定，下一步"}</button>}
        {!done && step === 3 && <button className="primary-button" disabled={busy} onClick={() => void saveModel(4)}>{busy ? "保存中…" : "保存映射，下一步"}</button>}
        {!done && step === 4 && <button className="primary-button" disabled={busy} onClick={() => setStep(5)}>核对完成，下一步</button>}
        {!done && step === 5 && <button className="primary-button" disabled={busy} onClick={() => void saveModel(5, true)}>{busy ? "保存中…" : "保存验证记录"}</button>}
        {done && <button className="primary-button" onClick={onClose}>完成</button>}
      </div>
    </section>
  </>, document.body)
}
