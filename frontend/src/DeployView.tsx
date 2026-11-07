import { useCallback, useEffect, useState } from "react"
import { ApiError, downloadCSV, request } from "./api"

type RuntimeInfo = {
  source: string
  channels: number
  devices: number
  points: number
  started_at: string
}

type Profile = {
  id: string; vendor: string; model: string; points: number; commands: number
  verified?: boolean; verification?: string; source?: string; identity_hint?: string
}

type ScanFound = { server_id: number; responded: boolean; latency_ms: number; probe: string; error: string }
type ScanResult = { scanned: number; found: ScanFound[]; errors: string[] }

type IdentifyCandidate = { profile: Profile; score: number; evidence: string; strong: boolean }
type IdentifyResult = {
  server_id: number
  matched: boolean
  profile: Profile | null
  candidates: IdentifyCandidate[]
  detail: string
}

type AcceptanceCheck = {
  key: string
  title: string
  status: "passed" | "warning" | "failed"
  detail: string
  object: string
  next_step: string
}

type AcceptanceReport = {
  id: string
  generated_at: string
  status: "passed" | "warning" | "failed"
  checks: AcceptanceCheck[]
  summary: { passed: number; warning: number; failed: number }
}

type ChannelPoint = {
  id: string
  name: string
  function: string
  address: number
  format: string
  scale: number
  offset: number
  kind: string
  read_only: boolean
}

type ChannelDevice = {
  id?: string
  device_id?: string
  server_id?: number
  health: string
  points?: ChannelPoint[]
}

type EmsChannel = {
  id: string
  node_id?: string
  persisted?: boolean
  protocol?: string
  address?: string
  period_ms?: number
  devices?: ChannelDevice[]
}

type CsvError = { line: number; reason: string }
type CsvResult = { preview?: boolean; imported?: number; count?: number; errors?: CsvError[] }

const statusText = { passed: "通过", warning: "需关注", failed: "需处理" } as const

function messageOf(cause: unknown) {
  if (cause instanceof ApiError) {
    if (cause.message === "channel_busy") return "该地址正被运行中的通道占用；先停用通道或换一个地址再扫描。"
    if (cause.message === "invalid_range") return "站号范围无效：起始站号不能大于结束站号。"
    if (cause.status === 501 || cause.status === 404) return `该接口不可用（${cause.status}）。`
  }
  return cause instanceof Error ? cause.message : "操作失败"
}

export default function DeployView({ token, isAdmin, goDelivery }: { token: string; isAdmin: boolean; goDelivery: () => void }) {
  const [runtime, setRuntime] = useState<RuntimeInfo | null>(null)
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [channels, setChannels] = useState<EmsChannel[]>([])
  const [report, setReport] = useState<AcceptanceReport | null>(null)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const [busy, setBusy] = useState("")

  const [protocol, setProtocol] = useState("tcp")
  const [address, setAddress] = useState("127.0.0.1:1502")
  const [baud, setBaud] = useState(9600)
  const [from, setFrom] = useState(1)
  const [to, setTo] = useState(16)
  const [timeout, setTimeout] = useState(300)
  const [scan, setScan] = useState<ScanResult | null>(null)
  const [identify, setIdentify] = useState<IdentifyResult | null>(null)
  const [serverID, setServerID] = useState(1)
  const [profileID, setProfileID] = useState("")
  const selectedProfile = profiles.find((profile) => profile.id === profileID)

  const [channelID, setChannelID] = useState("")
  const [csv, setCSV] = useState("")
  const [csvName, setCSVName] = useState("")
  const [csvResult, setCSVResult] = useState<CsvResult | null>(null)

  const loadChannels = useCallback(async () => {
    setBusy("channels")
    try {
      const result = await request<{ channels: EmsChannel[] }>(token, "/ems/channels")
      setChannels(result.channels)
      if (!channelID && result.channels[0]) setChannelID(result.channels[0].id)
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy("")
    }
  }, [token, channelID])

  const loadRuntime = useCallback(async () => {
    setBusy("runtime")
    setError("")
    try {
      setRuntime(await request<RuntimeInfo>(token, "/ems/runtime"))
      setNotice("运行状态已刷新。")
    } catch (cause) {
      setRuntime(null)
      setNotice(messageOf(cause))
    } finally {
      setBusy("")
    }
  }, [token])

  const loadProfiles = useCallback(async () => {
    setBusy("profiles")
    setError("")
    try {
      const result = await request<{ profiles: Profile[] }>(token, "/ems/profiles")
      setProfiles(result.profiles)
      setNotice(result.profiles.length ? `已加载 ${result.profiles.length} 个内置 Profile。` : "当前版本没有内置 Profile。")
    } catch (cause) {
      setNotice(messageOf(cause))
    } finally {
      setBusy("")
    }
  }, [token])

  async function downloadProfilePoints() {
    if (!profileID) return
    setBusy("profile-csv")
    setError("")
    try {
      await downloadCSV(token, `/ems/profiles/${encodeURIComponent(profileID)}/points.csv?server_id=${serverID}`, `${profileID}-points.csv`)
      setNotice("Profile 点表已下载；请在下方点表校验中先预检、再导入。")
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy("")
    }
  }

  const loadLatest = useCallback(async () => {
    setBusy("latest")
    setError("")
    try {
      setReport(await request<AcceptanceReport>(token, "/ems/acceptance/latest"))
    } catch (cause) {
      setReport(null)
      setNotice(cause instanceof ApiError && cause.status === 404 ? "还没有验收报告。" : messageOf(cause))
    } finally {
      setBusy("")
    }
  }, [token])

  useEffect(() => {
    void loadChannels()
  }, [loadChannels])

  async function runScan() {
    setBusy("scan")
    setError("")
    setNotice("")
    setIdentify(null)
    try {
      const result = await request<ScanResult>(token, "/ems/scan", "POST", {
        protocol, address, baud, from, to, timeout_ms: timeout,
      })
      setScan(result)
      const first = result.found.find((item) => item.responded)
      if (first) setServerID(first.server_id)
      setNotice(`扫描 ${result.scanned} 个站号，响应 ${result.found.filter((item) => item.responded).length} 个。扫描只读，不写设备。`)
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy("")
    }
  }

  async function runIdentify(id = serverID) {
    setBusy("identify")
    setError("")
    setNotice("")
    try {
      setIdentify(await request<IdentifyResult>(token, "/ems/identify", "POST", {
        protocol, address, baud, server_id: id, profile_id: profileID,
      }))
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy("")
    }
  }

  async function runAcceptance() {
    setBusy("acceptance")
    setError("")
    setNotice("")
    try {
      setReport(await request<AcceptanceReport>(token, "/ems/acceptance/run", "POST", {}))
      setNotice("验收报告已生成。失败项带有对象和下一步。")
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy("")
    }
  }

  async function uploadPoints(preview: boolean) {
    if (!channelID || !csv) return
    setBusy(preview ? "preview" : "import")
    setError("")
    setCSVResult(null)
    try {
      const path = `/api/modbus/${channelID}/io/import${preview ? "?preview=true" : ""}`
      const response = await fetch(path, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "text/csv" },
        body: csv,
      })
      const result = (await response.json().catch(() => ({}))) as CsvResult & { error?: string; line?: number; reason?: string }
      if (!response.ok) {
        if (response.status === 501) throw new ApiError("not_implemented", 501)
        const detail = result.line ? `第 ${result.line} 行：${result.reason || result.error || "格式错误"}` : result.error || "点表校验失败"
        throw new Error(detail)
      }
      setCSVResult(result)
      setNotice(preview ? "预检通过，尚未写入任何点位。" : `已导入 ${result.imported ?? result.count ?? 0} 个点位。`)
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy("")
    }
  }

  async function exportPoints() {
    if (!channelID) return
    setBusy("export")
    setError("")
    try {
      const response = await fetch(`/api/modbus/${channelID}/io/export`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!response.ok) {
        if (response.status === 501) throw new ApiError("not_implemented", 501)
        throw new Error("点表导出失败")
      }
      const url = URL.createObjectURL(await response.blob())
      const link = document.createElement("a")
      link.href = url
      link.download = `${channels.find((channel) => channel.id === channelID)?.address || "easyems"}-points.csv`
      link.click()
      URL.revokeObjectURL(url)
    } catch (cause) {
      setError(messageOf(cause))
    } finally {
      setBusy("")
    }
  }

  const responded = scan?.found.filter((item) => item.responded) || []
  const selectedChannel = channels.find((channel) => channel.id === channelID)

  return <section className="deploy-page">
    <div className="deploy-top">
      <article className="panel deploy-card">
        <div className="deploy-card-head">
          <div><span className="section-kicker">只有一套运行路径</span><h2>运行状态</h2></div>
          <button className="secondary-button" disabled={busy === "runtime"} onClick={() => void loadRuntime()}>{busy === "runtime" ? "读取中…" : "刷新"}</button>
        </div>
        {runtime ? <div className="runtime-chips">
          <div><strong>{runtime.channels}</strong><span>通道</span></div>
          <div><strong>{runtime.devices}</strong><span>设备</span></div>
          <div><strong>{runtime.points}</strong><span>点位</span></div>
          <div><strong>{runtime.source}</strong><span>数据来源</span></div>
        </div> : <p className="inline-note">运行状态读取失败；通道页面和工作台数据仍可用。</p>}
      </article>

      <article className="panel deploy-card">
        <div className="deploy-card-head">
          <div><span className="section-kicker">基于当前实例</span><h2>验收报告</h2></div>
          <div className="deploy-actions">
            <button className="secondary-button" disabled={busy === "latest"} onClick={() => void loadLatest()}>{busy === "latest" ? "读取中…" : "读取最近"}</button>
            <button className="secondary-button" onClick={goDelivery}>交付检查</button>
            <button className="primary-button" disabled={!isAdmin || busy === "acceptance"} onClick={() => void runAcceptance()}>{busy === "acceptance" ? "检查中…" : "运行验收"}</button>
          </div>
        </div>
        {report ? <>
          <div className={`acceptance-summary ${report.status}`}>
            <strong>{report.status === "passed" ? "可以进入交付" : report.status === "warning" ? "有事项需要关注" : "有失败项需要处理"}</strong>
            <span>{report.summary.passed} 通过 · {report.summary.warning} 需关注 · {report.summary.failed} 需处理</span>
            <small>{report.id} · {new Date(report.generated_at).toLocaleString("zh-CN")}</small>
          </div>
          <div className="acceptance-checks">{report.checks.map((check) => <div className={`acceptance-check ${check.status}`} key={check.key}>
            <span className="delivery-status">{statusText[check.status]}</span>
            <div><strong>{check.title}</strong><p>{check.detail}</p>{check.next_step && <small>下一步：{check.next_step}</small>}</div>
          </div>)}</div>
        </> : <p className="inline-note">还没有验收报告。运行验收会检查接口、通道、点位、规则、报警、历史、权限和备份。</p>}
      </article>
    </div>

    <article className="panel deploy-card">
      <div className="deploy-card-head">
        <div><span className="section-kicker">只读探测，不写设备</span><h2>扫描与识别</h2></div>
        <button className="secondary-button" disabled={!isAdmin || busy === "scan"} onClick={() => void runScan()}>{busy === "scan" ? "扫描中…" : "扫描站号"}</button>
      </div>
      <div className="scan-form">
        <label>协议<select value={protocol} onChange={(event) => setProtocol(event.target.value)}><option value="tcp">Modbus TCP</option><option value="rtu">Modbus RTU</option></select></label>
        <label>{protocol === "tcp" ? "地址" : "串口"}<input value={address} onChange={(event) => setAddress(event.target.value)} placeholder={protocol === "tcp" ? "127.0.0.1:1502" : "COM3"} /></label>
        {protocol === "rtu" && <label>波特率<input type="number" value={baud} onChange={(event) => setBaud(Number(event.target.value))} /></label>}
        <label>起始站号<input type="number" min={1} max={247} value={from} onChange={(event) => setFrom(Number(event.target.value))} /></label>
        <label>结束站号<input type="number" min={1} max={247} value={to} onChange={(event) => setTo(Number(event.target.value))} /></label>
        <label>超时（毫秒）<input type="number" min={50} value={timeout} onChange={(event) => setTimeout(Number(event.target.value))} /></label>
      </div>
      {scan && <div className="scan-results">
        <div className="scan-results-head"><strong>扫描结果</strong><span>{scan.scanned} 个站号 · {responded.length} 个响应</span></div>
        <div className="scan-table">
          {scan.found.map((item) => <div className={`scan-row ${item.responded ? "found" : ""}`} key={item.server_id}>
            <span className="scan-id">{item.server_id}</span>
            <strong>{item.responded ? "有响应" : "无响应"}</strong>
            <span>{item.responded ? `${item.latency_ms} ms · ${item.probe}` : item.error || "无应答"}</span>
            {item.responded && <button className="manage-button" disabled={busy === "identify"} onClick={() => { setServerID(item.server_id); void runIdentify(item.server_id) }}>识别</button>}
          </div>)}
          {scan.found.length === 0 && <p className="inline-note">没有发现响应站号。检查地址、站号和接线后重试。</p>}
        </div>
      </div>}
      <div className="identify-bar">
        <label>站号<input type="number" min={1} max={247} value={serverID} onChange={(event) => setServerID(Number(event.target.value))} /></label>
        <label>Profile<select value={profileID} onChange={(event) => setProfileID(event.target.value)}><option value="">自动匹配</option>{profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.vendor} {profile.model}{profile.verified ? "" : "（未验证）"}</option>)}</select></label>
        <button className="secondary-button" disabled={busy === "profiles"} onClick={() => void loadProfiles()}>{busy === "profiles" ? "加载中…" : "加载 Profile"}</button>
        <button className="secondary-button" disabled={!isAdmin || busy === "identify"} onClick={() => void runIdentify()}>识别设备</button>
        {profiles.length > 0 && <span className="inline-note">{profiles.length} 个内置 Profile 可选</span>}
      </div>
      {identify && <div className={`identify-result ${identify.matched ? "matched" : ""}`}>
        <strong>{identify.matched && identify.profile ? `${identify.profile.vendor} ${identify.profile.model}${identify.profile.verified ? "" : " · 未验证"}` : "未自动匹配"}</strong>
        <span>{identify.detail}</span>
        {identify.candidates.length > 0 && <div className="profile-list">{identify.candidates.filter((candidate) => candidate.profile.id !== identify.profile?.id).map((candidate) => <button key={candidate.profile.id} className="manage-button" title={candidate.evidence} onClick={() => { setProfileID(candidate.profile.id); void runIdentify() }}>{candidate.profile.vendor} {candidate.profile.model} · {candidate.profile.points} 点位 · {candidate.strong ? "匹配" : "候选"}<small>{candidate.evidence}</small></button>)}</div>}
        <small>识别只读；匹配不到时可人工选择 Profile 后继续接入，不阻塞。</small>
      </div>}
      {selectedProfile && <div className="profile-hint">
        <div><strong>{selectedProfile.vendor} {selectedProfile.model}</strong><span>{selectedProfile.verified ? "已现场验证" : "未现场验证"} · {selectedProfile.points} 点位 · {selectedProfile.commands} 命令</span></div>
        {selectedProfile.verification && <p>{selectedProfile.verification}</p>}
        {selectedProfile.identity_hint && <p>识别：{selectedProfile.identity_hint}</p>}
        {selectedProfile.source && <p>来源：{selectedProfile.source}</p>}
        <button className="secondary-button" disabled={busy === "profile-csv"} onClick={() => void downloadProfilePoints()}>{busy === "profile-csv" ? "下载中…" : "下载 Profile 点表"}</button>
      </div>}
    </article>

    <article className="panel deploy-card">
      <div className="deploy-card-head">
        <div><span className="section-kicker">预检通过才写入</span><h2>点表校验</h2></div>
        <button className="secondary-button" disabled={!channelID || busy === "export"} onClick={() => void exportPoints()}>导出当前点表</button>
      </div>
      {channels.length === 0 ? <p className="inline-note">还没有持久化通道。先在工作台接入 Modbus，或在通道页确认节点已生成。</p> : <>
        <div className="point-table-tools">
          <label>通道<select value={channelID} onChange={(event) => setChannelID(event.target.value)}>{channels.map((channel) => <option key={channel.id} value={channel.id}>{channel.address || channel.id} · {channel.devices?.length ?? 0} 台设备</option>)}</select></label>
          <label className="file-label">CSV 点表<input type="file" accept=".csv,text/csv" onChange={(event) => {
            const file = event.target.files?.[0]
            if (!file) return
            setCSVName(file.name)
            setCSVResult(null)
            void file.text().then(setCSV)
          }} /><span>{csvName || "选择文件"}</span></label>
          <button className="secondary-button" disabled={!csv || busy === "preview"} onClick={() => void uploadPoints(true)}>{busy === "preview" ? "预检中…" : "预检点表"}</button>
          <button className="primary-button" disabled={!csv || busy === "import"} onClick={() => void uploadPoints(false)}>{busy === "import" ? "导入中…" : "确认导入"}</button>
        </div>
        <p className="inline-note">列：名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位。预检失败不写入任何点位。</p>
        {selectedChannel && <div className="channel-point-summary">{(selectedChannel.devices || []).map((device, index) => <span key={device.id || device.device_id || index}>站号 {device.server_id ?? "—"} · {device.points?.length ?? 0} 点位 · {device.health}</span>)}</div>}
        {csvResult && <div className="csv-result">
          <strong>{csvResult.preview ? `预检通过：${csvResult.count ?? 0} 个点位` : `已导入 ${csvResult.imported ?? csvResult.count ?? 0} 个点位`}</strong>
          {csvResult.errors && csvResult.errors.length > 0 && <ul>{csvResult.errors.map((item) => <li key={`${item.line}-${item.reason}`}>第 {item.line} 行：{item.reason}</li>)}</ul>}
        </div>}
      </>}
    </article>

    {(error || notice) && <div className={error ? "inline-error" : "success-message"} role={error ? "alert" : "status"}>{error || notice}</div>}
  </section>
}
