import { useCallback, useEffect, useState } from "react"
import { request } from "./api"
import { EmsButton, EmsCard, EmsInput, EmsSectionHeader, EmptyState, InlineNotice, SegmentedTabs } from "./components/ems"

export type AlarmSoundSettings = {
  enabled: boolean
  volume: number
  silenceEnabled: boolean
  silenceFrom: string
  silenceTo: string
}

type DayStats = { date: string; raised: number; unhandled: number; handled: number }
type Ranking = { device_id: string; name: string; count: number }
type Stats = { today: DayStats; yesterday: DayStats; trend: DayStats[]; ranking: Ranking[]; since?: string }

const defaultSound: AlarmSoundSettings = {
  enabled: true, volume: 60, silenceEnabled: false, silenceFrom: "22:00", silenceTo: "07:00",
}

export function loadAlarmSound(): AlarmSoundSettings {
  try {
    const parsed = JSON.parse(localStorage.getItem("fastems-alarm-sound") || "{}")
    return {
      enabled: parsed.enabled !== false,
      volume: Math.min(100, Math.max(0, Number(parsed.volume) || defaultSound.volume)),
      silenceEnabled: Boolean(parsed.silenceEnabled),
      silenceFrom: parsed.silenceFrom || defaultSound.silenceFrom,
      silenceTo: parsed.silenceTo || defaultSound.silenceTo,
    }
  } catch {
    return defaultSound
  }
}

export function saveAlarmSound(settings: AlarmSoundSettings) {
  localStorage.setItem("fastems-alarm-sound", JSON.stringify(settings))
}

export function inSilenceWindow(settings: AlarmSoundSettings, now = new Date()) {
  if (!settings.silenceEnabled) return false
  const minutes = now.getHours() * 60 + now.getMinutes()
  const parse = (value: string) => {
    const [hour, minute] = value.split(":").map(Number)
    return (Number.isFinite(hour) ? hour : 0) * 60 + (Number.isFinite(minute) ? minute : 0)
  }
  const from = parse(settings.silenceFrom)
  const to = parse(settings.silenceTo)
  if (from === to) return false
  return from < to ? minutes >= from && minutes < to : minutes >= from || minutes < to
}

export default function AlarmStatsPanel({ token, onError, sound, onSound, onTest }: {
  token: string
  onError: (message: string) => void
  sound: AlarmSoundSettings
  onSound: (next: AlarmSoundSettings) => void
  onTest: () => void
}) {
  const [days, setDays] = useState(7)
  const [stats, setStats] = useState<Stats | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async (windowDays: number) => {
    setLoading(true)
    try {
      setStats(await request<Stats>(token, `/alarm-stats?days=${windowDays}`))
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "报警统计读取失败")
    } finally {
      setLoading(false)
    }
  }, [token, onError])

  useEffect(() => { void load(days) }, [load, days])
  const max = Math.max(1, ...(stats?.trend.map((item) => item.raised) || [1]))

  return <div className="alarm-stats-page min-w-0">
    <div className="alarm-stats-top !grid-cols-1 sm:!grid-cols-3">
      <EmsCard className="alarm-stat-card"><span>今日告警</span><strong>{stats?.today.raised ?? "—"}</strong><small>未处理 {stats?.today.unhandled ?? "—"} · 已处理 {stats?.today.handled ?? "—"}</small></EmsCard>
      <EmsCard className="alarm-stat-card"><span>昨日告警</span><strong>{stats?.yesterday.raised ?? "—"}</strong><small>未处理 {stats?.yesterday.unhandled ?? "—"} · 已处理 {stats?.yesterday.handled ?? "—"}</small></EmsCard>
      <EmsCard className="alarm-stat-card"><span>统计起点</span><strong className="alarm-stat-date">{stats?.since || "—"}</strong><small>报警事件本地留痕后开始累计</small></EmsCard>
    </div>

    <div className="alarm-stats-grid !grid-cols-1 !content-start lg:!grid-cols-3">
      <EmsCard className="alarm-trend-panel min-h-[280px] min-w-0 self-start">
        <EmsSectionHeader kicker="按天" title="告警趋势" actions={<SegmentedTabs value={String(days)} onChange={(value) => setDays(Number(value))} ariaLabel="统计天数" options={[{ value: "7", label: "7 天" }, { value: "30", label: "30 天" }]} />} />
        <div className={`alarm-trend ${days === 30 ? "dense" : ""}`}>
          {(stats?.trend || []).map((item) => <div className="alarm-trend-item" key={item.date} title={`${item.date} · ${item.raised} 条`}>
            <span className="alarm-trend-bar" style={{ height: `${Math.max(2, (item.raised / max) * 100)}%` }} />
            <small>{item.date.slice(5)}</small>
          </div>)}
        </div>
        {loading && <InlineNotice tone="busy">正在读取统计…</InlineNotice>}
        {!loading && stats?.trend.length === 0 && <EmptyState title="暂无告警趋势" description="统计窗口内没有告警记录。" />}
      </EmsCard>

      <EmsCard className="alarm-ranking-panel min-h-[280px] min-w-0 self-start">
        <EmsSectionHeader kicker="按对象" title="告警排行" />
        <div className="alarm-ranking-list">
          {(stats?.ranking || []).map((item, index) => <div className="alarm-ranking-row" key={item.device_id}><span>{index + 1}</span><strong title={item.name}>{item.name}</strong><small>{item.count}</small></div>)}
          {stats && stats.ranking.length === 0 && <EmptyState title="暂无排行" description="统计窗口内没有告警。" />}
        </div>
      </EmsCard>

      <EmsCard className="alarm-sound-panel min-h-[280px] min-w-0 self-start">
        <EmsSectionHeader kicker="本地提醒" title="报警声音与静默" />
        <label className="comm-check min-h-8"><input type="checkbox" checked={sound.enabled} onChange={(event) => onSound({ ...sound, enabled: event.target.checked })} /> 启用报警声音</label>
        <label className="alarm-volume">音量 <input aria-label="报警音量" type="range" min={0} max={100} value={sound.volume} onChange={(event) => onSound({ ...sound, volume: Number(event.target.value) })} /><span>{sound.volume}%</span></label>
        <div><EmsButton variant="secondary" size="small" onClick={onTest}>试听</EmsButton></div>
        <label className="comm-check min-h-8"><input type="checkbox" checked={sound.silenceEnabled} onChange={(event) => onSound({ ...sound, silenceEnabled: event.target.checked })} /> 静默时段</label>
        <div className="alarm-silence-times flex-wrap">
          <EmsInput label="从" aria-label="静默开始时间" type="time" value={sound.silenceFrom} disabled={!sound.silenceEnabled} onIonInput={(event) => onSound({ ...sound, silenceFrom: event.detail.value || "" })} />
          <EmsInput label="到" aria-label="静默结束时间" type="time" value={sound.silenceTo} disabled={!sound.silenceEnabled} onIonInput={(event) => onSound({ ...sound, silenceTo: event.detail.value || "" })} />
        </div>
        <p className="alarm-sound-note">声音只在浏览器页面打开时播放；静默时段仍会记录、显示和统计报警，只是不响铃。</p>
      </EmsCard>
    </div>
  </div>
}
