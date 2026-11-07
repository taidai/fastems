import { FormEvent, useState } from "react"
import { nameOf, Node, point, request, Snapshot } from "./api"

type Props = {
  token: string
  node: Node
  snapshot: Snapshot
  run: (action: () => Promise<unknown>) => Promise<boolean>
  onClose: () => void
}

const numberPoint = (node: Node, type: string) => point(node, type)?.value || 0
const textPoint = (node: Node, type: string) => point(node, type)?.text || ""

export default function NodeManager({ token, node, snapshot, run, onClose }: Props) {
  const [description, setDescription] = useState(nameOf(node))
  const [disabled, setDisabled] = useState(numberPoint(node, "disabled") !== 0)
  const [newParent, setNewParent] = useState(node.parent)
  const [connection, setConnection] = useState({
    parent: node.parent, description: nameOf(node), protocol: textPoint(node, "protocol") || "TCP",
    uri: textPoint(node, "uri"), port: textPoint(node, "port"), baud: textPoint(node, "baud") || "9600",
    poll_period: numberPoint(node, "pollPeriod") || 1000, timeout: numberPoint(node, "timeout") || 1000,
    disabled,
  })
  const [io, setIO] = useState({
    parent: node.parent, description: nameOf(node), server_id: numberPoint(node, "id") || 1,
    address: numberPoint(node, "address"), io_type: textPoint(node, "modbusIoType") || "modbusHoldingRegister",
    data_format: textPoint(node, "dataFormat") || "uint16", read_only: numberPoint(node, "readOnly") !== 0,
    scale: numberPoint(node, "scale") || 1, offset: numberPoint(node, "offset"), units: textPoint(node, "units"), disabled,
  })
  const destinations = snapshot.nodes.filter((item) => item.type === "group" || snapshot.roots.includes(item.id))
  const isBus = node.type === "modbus"
  const isIO = node.type === "modbusIo"
  const canDisable = node.type !== "group"
  const canMove = !isIO
  const lastGoodPoll = textPoint(node, "lastGoodPoll") || point(node, "lastGoodPoll")?.time
  const errors = numberPoint(node, "errorCount")

  async function save(event: FormEvent) {
    event.preventDefault()
    const success = await run(() => {
      if (isBus) return request(token, `/modbus/${node.id}`, "PUT", { ...connection, description, disabled })
      if (isIO) return request(token, `/modbus/io/${node.id}`, "PUT", { ...io, description, disabled })
      return request(token, `/nodes/${node.id}`, "PATCH", { description, ...(canDisable ? { disabled } : {}) })
    })
    if (success) onClose()
  }
  async function move() {
    const success = await run(() => request(token, `/nodes/${node.id}/move`, "POST", { old_parent: node.parent, new_parent: newParent }))
    if (success) onClose()
  }
  async function remove() {
    if (!window.confirm(`确定删除“${nameOf(node)}”吗？有子节点或规则引用时系统会拒绝。`)) return
    const success = await run(() => request(token, `/nodes/${node.id}`, "DELETE", { parent: node.parent }))
    if (success) onClose()
  }

  return <aside className="node-drawer" aria-label={`${nameOf(node)}管理`}>
    <div className="drawer-heading"><div><span className="section-kicker">设备与点位管理</span><h2>{nameOf(node)}</h2></div><button className="icon-button" aria-label="关闭管理" onClick={onClose}>×</button></div>
    <div className="drawer-meta"><span>{({ group: "设备分组", variable: "测试变量", modbus: "Modbus 连接", modbusIo: "Modbus 点位" } as Record<string, string>)[node.type]}</span><span>{disabled ? "已停用" : "运行中"}</span></div>
    {(isBus || isIO) && <div className="diagnostic-box"><strong>连接诊断</strong><span>最近完整成功轮询：{lastGoodPoll ? new Date(lastGoodPoll).toLocaleString("zh-CN") : "尚无"}</span><span>通信错误计数：{errors}</span><span>{disabled ? "连接/点位已停用" : "自动输入仍受数据有效期检查"}</span></div>}
    <form className="drawer-form" onSubmit={save}>
      <label>名称<input value={description} onChange={(event) => setDescription(event.target.value)} required /></label>
      {isBus && <>
        <label>协议<select value={connection.protocol} onChange={(event) => setConnection({ ...connection, protocol: event.target.value })}><option value="TCP">Modbus TCP</option><option value="RTU">Modbus RTU</option></select></label>
        {connection.protocol === "TCP" ? <label>TCP 地址<input value={connection.uri} onChange={(event) => setConnection({ ...connection, uri: event.target.value })} required /></label> : <><label>串口<input value={connection.port} onChange={(event) => setConnection({ ...connection, port: event.target.value })} required /></label><label>波特率<input value={connection.baud} onChange={(event) => setConnection({ ...connection, baud: event.target.value })} required /></label></>}
        <div className="form-row"><label>轮询（毫秒）<input type="number" min={10} value={connection.poll_period} onChange={(event) => setConnection({ ...connection, poll_period: Number(event.target.value) })} /></label><label>超时（毫秒）<input type="number" min={1} value={connection.timeout} onChange={(event) => setConnection({ ...connection, timeout: Number(event.target.value) })} /></label></div>
      </>}
      {isIO && <>
        <div className="form-row"><label>站号<input type="number" min={1} max={247} value={io.server_id} onChange={(event) => setIO({ ...io, server_id: Number(event.target.value) })} /></label><label>地址<input type="number" min={0} max={65535} value={io.address} onChange={(event) => setIO({ ...io, address: Number(event.target.value) })} /></label></div>
        <label>点位类型<select value={io.io_type} onChange={(event) => { const value = event.target.value; setIO({ ...io, io_type: value, read_only: value === "modbusInputRegister" || value === "modbusDiscreteInput" }) }}><option value="modbusHoldingRegister">保持寄存器</option><option value="modbusInputRegister">输入寄存器</option><option value="modbusCoil">线圈</option><option value="modbusDiscreteInput">离散输入</option></select></label>
        <label>数据格式<select value={io.data_format} onChange={(event) => setIO({ ...io, data_format: event.target.value })}><option value="uint16">uint16</option><option value="int16">int16</option><option value="uint32">uint32</option><option value="int32">int32</option><option value="float32">float32</option></select></label>
        <div className="form-row"><label>缩放<input type="number" step="any" value={io.scale} onChange={(event) => setIO({ ...io, scale: Number(event.target.value) })} /></label><label>偏移<input type="number" step="any" value={io.offset} onChange={(event) => setIO({ ...io, offset: Number(event.target.value) })} /></label></div>
        <label>单位<input value={io.units} onChange={(event) => setIO({ ...io, units: event.target.value })} /></label>
        <label className="checkbox-label"><input type="checkbox" checked={io.read_only} disabled={io.io_type === "modbusInputRegister" || io.io_type === "modbusDiscreteInput"} onChange={(event) => setIO({ ...io, read_only: event.target.checked })} />只读，不允许控制</label>
      </>}
      {canDisable && <label className="checkbox-label"><input type="checkbox" checked={disabled} onChange={(event) => setDisabled(event.target.checked)} />停用此对象，禁止人工或自动使用</label>}
      <button className="primary-button">保存修改</button>
    </form>
    {canMove && <div className="move-section"><label>移动到<select value={newParent} onChange={(event) => setNewParent(event.target.value)}>{destinations.map((item) => <option key={item.id} value={item.id}>{snapshot.roots.includes(item.id) ? "根节点" : nameOf(item)}</option>)}</select></label><button className="secondary-button" disabled={newParent === node.parent} onClick={move}>移动位置</button></div>}
    <div className="danger-zone"><strong>删除对象</strong><p>系统会检查子节点和规则引用。有依赖时不会删除。</p><button className="danger-button" onClick={remove}>删除</button></div>
  </aside>
}
