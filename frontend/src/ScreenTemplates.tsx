import { useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import type { ScreenConfig } from "./api"

export type ScreenTemplate = { id: string; name: string; config: ScreenConfig; at: string }
export type ScreenVersion = { id: string; config: ScreenConfig; at: string }

const TEMPLATES_KEY = "fastiot-screen-templates"
const versionKey = (nodeId: string) => `fastiot-screen-versions:${nodeId}`
const MAX_VERSIONS = 20

export function loadTemplates(): ScreenTemplate[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(TEMPLATES_KEY) || "[]")
    return Array.isArray(parsed) ? parsed.filter((item) => item && item.config && typeof item.name === "string") : []
  } catch {
    return []
  }
}

function saveTemplates(list: ScreenTemplate[]) {
  localStorage.setItem(TEMPLATES_KEY, JSON.stringify(list))
}

export function loadVersions(nodeId: string): ScreenVersion[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(versionKey(nodeId)) || "[]")
    return Array.isArray(parsed) ? parsed.filter((item) => item && item.config) : []
  } catch {
    return []
  }
}

function saveVersions(nodeId: string, list: ScreenVersion[]) {
  localStorage.setItem(versionKey(nodeId), JSON.stringify(list))
}

export function recordVersion(nodeId: string, config: ScreenConfig) {
  const list = loadVersions(nodeId)
  if (list.length && JSON.stringify(list[0].config) === JSON.stringify(config)) return
  const next = [{ id: `${Date.now()}-${Math.random().toString(16).slice(2)}`, config, at: new Date().toISOString() }, ...list].slice(0, MAX_VERSIONS)
  saveVersions(nodeId, next)
}

function timeText(at: string) {
  const date = new Date(at)
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString("zh-CN", { hour12: false })
}

export default function ScreenTemplates({ nodeId, draft, onApply, onClose }: {
  nodeId: string
  draft: ScreenConfig
  onApply: (config: ScreenConfig) => void
  onClose: () => void
}) {
  const [tab, setTab] = useState<"templates" | "versions">("templates")
  const [templates, setTemplates] = useState<ScreenTemplate[]>(loadTemplates)
  const [versions, setVersions] = useState<ScreenVersion[]>(() => loadVersions(nodeId))
  const [name, setName] = useState("")

  const sortedVersions = useMemo(() => [...versions].sort((a, b) => b.at.localeCompare(a.at)), [versions])

  function saveTemplate() {
    const trimmed = name.trim()
    if (!trimmed) return
    const next = [{ id: `${Date.now()}-${Math.random().toString(16).slice(2)}`, name: trimmed, config: draft, at: new Date().toISOString() }, ...templates]
    setTemplates(next)
    saveTemplates(next)
    setName("")
  }

  function removeTemplate(id: string) {
    const next = templates.filter((item) => item.id !== id)
    setTemplates(next)
    saveTemplates(next)
  }

  function apply(config: ScreenConfig) {
    onApply(config)
    onClose()
  }

  return createPortal(<div className="device-confirm-backdrop" onClick={onClose}>
    <section className="device-confirm screen-templates" role="dialog" aria-label="画面模板与版本" onClick={(event) => event.stopPropagation()}>
      <span className="section-kicker">本机保存</span>
      <h3>画面模板与版本</h3>
      <div className="screen-templates-tabs" role="group" aria-label="模板或版本">
        <button className={tab === "templates" ? "active" : ""} onClick={() => setTab("templates")}>模板</button>
        <button className={tab === "versions" ? "active" : ""} onClick={() => setTab("versions")}>版本快照</button>
      </div>

      {tab === "templates" && <>
        <div className="screen-template-save">
          <input aria-label="模板名称" value={name} onChange={(event) => setName(event.target.value)} placeholder="模板名称，例如：储能巡检" />
          <button className="primary-button" disabled={!name.trim()} onClick={saveTemplate}>保存当前布局为模板</button>
        </div>
        <div className="screen-template-list">
          {templates.length === 0 && <p className="empty-state">还没有模板。保存一个布局后可在任意画面套用。</p>}
          {templates.map((template) => <div className="screen-template-row" key={template.id}>
            <div><strong>{template.name}</strong><small>{template.config.point_ids.length} 点位 · {timeText(template.at)}</small></div>
            <button className="secondary-button" onClick={() => apply(template.config)}>套用</button>
            <button className="secondary-button" onClick={() => removeTemplate(template.id)}>删除</button>
          </div>)}
        </div>
      </>}

      {tab === "versions" && <div className="screen-template-list">
        {sortedVersions.length === 0 && <p className="empty-state">还没有版本快照。每次保存画面会自动记录一个版本。</p>}
        {sortedVersions.map((version) => <div className="screen-template-row" key={version.id}>
          <div><strong>版本 · {timeText(version.at)}</strong><small>{version.config.point_ids.length} 点位 · {version.config.columns} 列</small></div>
          <button className="secondary-button" onClick={() => apply(version.config)}>回滚到此版本</button>
        </div>)}
      </div>}

      <div className="device-confirm-actions">
        <button className="secondary-button" onClick={onClose}>关闭</button>
      </div>
    </section>
  </div>, document.body)
}
