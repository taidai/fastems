import { useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { request } from "./api"
import ConfirmDialog from "./ConfirmDialog"
import type { CommandOutcome, LedgerDevice, UnitCommand, UnitModel } from "./deviceTypes"

const deviceKey = (device: LedgerDevice) => `${device.channel_id}/${device.server_id}`
const stateNames: Record<string, string> = { queued: "排队", running: "执行中", confirmed: "已确认", failed: "失败", timeout: "超时", unknown: "结果未知" }

function defaultParams(command: UnitCommand) {
  return Object.fromEntries((command.parameters || []).map((parameter) => [parameter.name, parameter.default]))
}

export default function CommonCommands({ token, canOperate, onError, onClose }: {
  token: string
  canOperate: boolean
  onError: (message: string) => void
  onClose: () => void
}) {
  const [devices, setDevices] = useState<LedgerDevice[]>([])
  const [commandsByDevice, setCommandsByDevice] = useState<Record<string, UnitCommand[]>>({})
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [pending, setPending] = useState<{ device: LedgerDevice; command: UnitCommand } | null>(null)
  const [results, setResults] = useState<Record<string, string>>({})

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const result = await request<{ devices: LedgerDevice[] }>(token, "/device-ledger")
        if (cancelled) return
        const list = (result.devices || []).filter((device) => device.command_count > 0)
        setDevices(list)
        for (const device of list) {
          try {
            const model = await request<UnitModel>(token, `/modbus/${encodeURIComponent(device.channel_id)}/units/${device.server_id}/model`)
            if (cancelled) return
            const commands = (model.commands || []).filter((command) => command.available)
            if (commands.length) setCommandsByDevice((current) => ({ ...current, [deviceKey(device)]: commands }))
          } catch {
            // 单台设备模型读取失败不影响其它设备
          }
        }
      } catch (cause) {
        if (!cancelled) onError(cause instanceof Error ? cause.message : "常用命令读取失败")
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => { cancelled = true }
  }, [token, onError])

  const rows = useMemo(() => devices
    .map((device) => ({ device, commands: commandsByDevice[deviceKey(device)] || [] }))
    .filter((row) => row.commands.length > 0), [devices, commandsByDevice])

  async function execute(device: LedgerDevice, command: UnitCommand) {
    if (!canOperate) return
    setBusy(true)
    const base = `/modbus/${encodeURIComponent(device.channel_id)}/units/${device.server_id}`
    const requestID = globalThis.crypto?.randomUUID?.() || `cmd-${Date.now()}-${Math.random().toString(16).slice(2)}`
    try {
      const outcome = await request<CommandOutcome>(token, `${base}/commands`, "POST", {
        request_id: requestID, command_id: command.id, parameters: defaultParams(command),
      })
      let state = outcome.state
      for (let i = 0; i < 12 && !["confirmed", "failed", "timeout", "unknown"].includes(state); i += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 500))
        const latest = await request<{ commands: CommandOutcome[] }>(token, `${base}/commands`)
        const item = (latest.commands || []).find((entry) => entry.request_id === requestID)
        if (item) state = item.state
      }
      setResults((current) => ({ ...current, [`${deviceKey(device)}/${command.id}`]: stateNames[state] || state }))
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : "命令执行失败")
    } finally {
      setBusy(false)
    }
  }

  return createPortal(<div className="device-confirm-backdrop" onClick={onClose}>
    <section className="device-confirm common-commands" role="dialog" aria-label="常用命令" onClick={(event) => event.stopPropagation()}>
      <span className="section-kicker">受控操作</span>
      <h3>常用命令</h3>
      <p>{canOperate ? "点击执行后走确认对话框，命令会写入设备并等待回读。" : "当前账号只能查看，不能执行命令。"}</p>
      {loading && <div className="empty-state"><strong>正在读取设备命令…</strong></div>}
      {!loading && rows.length === 0 && <div className="empty-state"><strong>还没有可用命令</strong><span>先在“通道 → 端口配置”绑定 Profile，Profile 里的命令会出现在这里。</span></div>}
      {rows.map(({ device, commands }) => <div className="common-command-group" key={deviceKey(device)}>
        <div className="common-command-device"><strong>{device.business_node_name || device.name}</strong><small>{device.channel_name} / 站号 {device.server_id}</small></div>
        {commands.map((command) => {
          const state = results[`${deviceKey(device)}/${command.id}`]
          return <div className="common-command-row" key={command.id}>
            <div className="common-command-main"><strong>{command.name}</strong><small>{command.point_name} = {command.value}</small></div>
            {state ? <span className={`device-command-state ${state === "已确认" ? "confirmed" : ""}`}>{state}</span> : null}
            <button className="secondary-button" disabled={!canOperate || busy} onClick={() => setPending({ device, command })}>执行</button>
          </div>
        })}
      </div>)}
      <div className="device-confirm-actions">
        <button className="secondary-button" onClick={onClose}>关闭</button>
      </div>
    </section>

    {pending && <ConfirmDialog
      title={`确认执行「${pending.command.name}」`}
      message="命令会写入设备并等待回读；失败不会自动重试。"
      rows={[
        { label: "设备", value: pending.device.business_node_name || pending.device.name },
        { label: "目标", value: `${pending.command.point_name} = ${pending.command.value}` },
        ...(pending.command.parameters || []).map((parameter) => ({ label: parameter.label || parameter.name, value: String(parameter.default) })),
      ]}
      confirmLabel="确认执行"
      busy={busy}
      onCancel={() => setPending(null)}
      onConfirm={() => {
        const target = pending
        setPending(null)
        void execute(target.device, target.command)
      }}
    />}
  </div>, document.body)
}
