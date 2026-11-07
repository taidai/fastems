import { useEffect, useMemo, useState } from "react"
import { writable } from "./writable"
import {
  AllocatorConfig,
  AllocatorMember,
  fetchAllocator,
  nameOf,
  Node,
  point,
  saveAllocator,
  Target,
} from "./api"

type Run = (action: () => Promise<unknown>) => Promise<boolean>

const emptyConfig = (): AllocatorConfig => ({ grace_ms: 1000, timeout_ms: 3000, members: [] })

function targetFor(nodeID: string, pointType: string): Target {
  return { node_id: nodeID, point_type: pointType }
}

function cloneConfig(config: AllocatorConfig): AllocatorConfig {
  return JSON.parse(JSON.stringify(config)) as AllocatorConfig
}

export default function AllocatorsView({ token, nodes, groups, points, isAdmin, busy, run, onError }: {
  token: string
  nodes: Node[]
  groups: Node[]
  points: Node[]
  isAdmin: boolean
  busy: boolean
  run: Run
  onError: (message: string) => void
}) {
  const [nodeID, setNodeID] = useState(() => sessionStorage.getItem("easyems-allocator-node") || groups[0]?.id || "")
  const [saved, setSaved] = useState<AllocatorConfig>(emptyConfig)
  const [draft, setDraft] = useState<AllocatorConfig | null>(null)
  const selected = groups.find((group) => group.id === nodeID)
  const savedText = selected ? point(selected, "easyemsAllocator")?.text || "" : ""

  useEffect(() => {
    sessionStorage.setItem("easyems-allocator-node", nodeID)
  }, [nodeID])

  useEffect(() => {
    let cancelled = false
    setDraft(null)
    if (!savedText) {
      const initial = emptyConfig()
      setSaved(initial)
      setDraft(cloneConfig(initial))
      return
    }
    fetchAllocator(token, nodeID)
      .then((config) => {
        if (cancelled) return
        setSaved(config)
        setDraft(cloneConfig(config))
      })
      .catch((cause) => {
        if (!cancelled) onError(cause instanceof Error ? cause.message : "分配器读取失败")
      })
    return () => { cancelled = true }
  }, [token, nodeID, savedText, onError])

  const subtree = useMemo(() => {
    const byID = new Map(nodes.map((node) => [node.id, node]))
    const ids = new Set<string>()
    const visit = (id: string) => {
      if (ids.has(id)) return
      ids.add(id)
      for (const node of nodes) if (node.parent === id) visit(node.id)
    }
    if (nodeID) visit(nodeID)
    // A malformed parent link must not hide all points. byID also makes the
    // ancestry check explicit for nodes whose direct parent is a Modbus bus.
    if (!ids.size && byID.has(nodeID)) ids.add(nodeID)
    return ids
  }, [nodes, nodeID])

  const availablePoints = points.filter((item) => subtree.has(item.id))
  const writablePoints = availablePoints.filter(writable)
  const dirty = draft ? JSON.stringify(draft) !== JSON.stringify(saved) : false

  function update(index: number, patch: Partial<AllocatorMember>) {
    if (!draft) return
    setDraft({
      ...draft,
      members: draft.members.map((member, itemIndex) => itemIndex === index ? { ...member, ...patch } : member),
    })
  }

  function updateTarget(index: number, field: "target" | "available_discharge" | "available_charge" | "readback" | "fault", id: string) {
    if (!draft) return
    const member = draft.members[index]
    const next = { ...member }
    if (!id) {
      delete next[field]
    } else {
      next[field] = targetFor(id, field === "target" ? "valueSet" : "value")
    }
    update(index, next)
  }

  function addMember() {
    if (!draft || !writablePoints.length) return
    const number = draft.members.length + 1
    const target = targetFor(writablePoints[0].id, "valueSet")
    setDraft({
      ...draft,
      members: [...draft.members, {
        id: `CAB${String(number).padStart(2, "0")}`,
        target,
        max_discharge: 0,
        max_charge: 0,
        min_power: 0,
      }],
    })
  }

  function removeMember(index: number) {
    if (!draft) return
    setDraft({ ...draft, members: draft.members.filter((_, itemIndex) => itemIndex !== index) })
  }

  function validate() {
    if (!draft) return "配置尚未读取"
    if (!draft.members.length) return "至少添加一个 PCS 成员"
    if ((draft.grace_ms || 0) < 0 || (draft.timeout_ms || 0) < 0) return "宽限期和回读超时不能为负数"
    const ids = new Set<string>()
    const targets = new Set<string>()
    for (const member of draft.members) {
      if (!member.id.trim()) return "成员名称不能为空"
      if (ids.has(member.id.trim())) return `成员名称重复：${member.id}`
      ids.add(member.id.trim())
      if (!member.target?.node_id) return `${member.id} 还没有选择功率目标`
      if (targets.has(member.target.node_id)) return `功率目标不能重复：${member.target.node_id}`
      targets.add(member.target.node_id)
      if (member.max_discharge < 0 || member.max_charge < 0 || (member.min_power || 0) < 0) {
        return `${member.id} 的功率不能为负数`
      }
    }
    return ""
  }

  async function save() {
    if (!draft) return
    const problem = validate()
    if (problem) {
      onError(problem)
      return
    }
    const normalized = {
      ...draft,
      members: draft.members.map((member) => ({ ...member, id: member.id.trim() })),
    }
    const ok = await run(async () => {
      await saveAllocator(token, nodeID, normalized)
      setSaved(cloneConfig(normalized))
      setDraft(cloneConfig(normalized))
    })
    if (ok) setSaved(cloneConfig(normalized))
  }

  const validation = draft ? validate() : ""

  return <section className="allocator-page">
    <div className="panel allocator-toolbar">
      <label>分配器所属业务节点
        <select value={nodeID} onChange={(event) => setNodeID(event.target.value)}>
          {groups.map((group) => <option key={group.id} value={group.id}>{nameOf(group)}</option>)}
        </select>
      </label>
      <span className={dirty ? "allocator-unsaved" : "allocator-saved"}>{dirty ? "有未保存修改" : "配置已保存"}</span>
      <span className="allocator-count">{draft?.members.length || 0} 个 PCS 成员</span>
    </div>

    <div className="panel allocator-help">
      <strong>一个总目标，按各台 PCS 当前能力分配</strong>
      <p>保存只写配置，不会立即下发功率。决策表绑定这个业务节点后，按可用功率比例分配；每台回读成功后才操作下一台，超时停止后续。</p>
    </div>

    {draft && <>
      <div className="panel allocator-timing">
        <label>成员失联宽限期（毫秒）
          <input type="number" min="0" step="100" value={draft.grace_ms || 0} disabled={!isAdmin} onChange={(event) => setDraft({ ...draft, grace_ms: Number(event.target.value) })} />
        </label>
        <label>单台回读超时（毫秒）
          <input type="number" min="0" step="100" value={draft.timeout_ms || 0} disabled={!isAdmin} onChange={(event) => setDraft({ ...draft, timeout_ms: Number(event.target.value) })} />
        </label>
      </div>

      <div className="allocator-members">
        {draft.members.map((member, index) => <article className="panel allocator-member" key={`${member.id}-${index}`}>
          <div className="allocator-member-head">
            <h2>成员 {index + 1}</h2>
            {isAdmin && <button className="manage-button" onClick={() => removeMember(index)}>删除成员</button>}
          </div>
          <div className="allocator-grid">
            <label>成员名称
              <input value={member.id} disabled={!isAdmin} onChange={(event) => update(index, { id: event.target.value })} />
            </label>
            <PointSelect label="功率目标（写入）" value={member.target?.node_id || ""} options={writablePoints} disabled={!isAdmin} onChange={(value) => updateTarget(index, "target", value)} />
            <NumberField label="最大放电功率（kW）" value={member.max_discharge} disabled={!isAdmin} onChange={(value) => update(index, { max_discharge: value })} />
            <NumberField label="最大充电功率（kW）" value={member.max_charge} disabled={!isAdmin} onChange={(value) => update(index, { max_charge: value })} />
            <PointSelect label="可用放电功率（可选）" value={member.available_discharge?.node_id || ""} options={availablePoints} optional="使用最大放电功率" disabled={!isAdmin} onChange={(value) => updateTarget(index, "available_discharge", value)} />
            <PointSelect label="可用充电功率（可选）" value={member.available_charge?.node_id || ""} options={availablePoints} optional="使用最大充电功率" disabled={!isAdmin} onChange={(value) => updateTarget(index, "available_charge", value)} />
            <PointSelect label="实际功率回读（可选）" value={member.readback?.node_id || ""} options={availablePoints} optional="读取功率目标当前值" disabled={!isAdmin} onChange={(value) => updateTarget(index, "readback", value)} />
            <PointSelect label="故障输入（可选）" value={member.fault?.node_id || ""} options={availablePoints} optional="无独立故障点" disabled={!isAdmin} onChange={(value) => updateTarget(index, "fault", value)} />
            <NumberField label="最小参与功率（kW，可选）" value={member.min_power || 0} disabled={!isAdmin} onChange={(value) => update(index, { min_power: value })} />
          </div>
        </article>)}
      </div>

      {!draft.members.length && <div className="panel empty-state large">
        <strong>先添加一台 PCS</strong>
        <span>成员只属于当前业务节点；新增或更换设备时，只修改这里的绑定，不改决策表。</span>
      </div>}

      <div className="panel allocator-actions">
        {isAdmin && <button className="secondary-button" disabled={!writablePoints.length || busy} onClick={addMember}>+ 添加 PCS 成员</button>}
        {isAdmin && <button className="secondary-button" disabled={!dirty || busy} onClick={() => setDraft(cloneConfig(saved))}>恢复已保存</button>}
        {isAdmin && <button className="primary-button" disabled={busy || Boolean(validation)} onClick={() => void save()}>保存分配器</button>}
        {validation && <span className="allocator-validation">{validation}</span>}
        {!writablePoints.length && <span className="allocator-validation">当前节点下没有可写点位，请先接入设备。</span>}
      </div>
    </>}
  </section>
}

function PointSelect({ label, value, options, optional, disabled, onChange }: {
  label: string
  value: string
  options: Node[]
  optional?: string
  disabled?: boolean
  onChange: (value: string) => void
}) {
  return <label>{label}
    <select aria-label={label} value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)}>
      {optional !== undefined ? <option value="">{optional}</option> : <option value="">请选择点位</option>}
      {options.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}
    </select>
  </label>
}

function NumberField({ label, value, disabled, onChange }: {
  label: string
  value: number
  disabled?: boolean
  onChange: (value: number) => void
}) {
  return <label>{label}
    <input aria-label={label} type="number" min="0" step="any" value={value} disabled={disabled} onChange={(event) => onChange(Number(event.target.value))} />
  </label>
}
