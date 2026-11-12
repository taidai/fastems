import { useEffect, useState } from "react"
import { downloadCSV, importCSV, nameOf, Node, request } from "./api"

type Run = (action: () => Promise<unknown>) => Promise<boolean>

export default function ModbusTools({ token, buses, templates, busy, run }: {
  token: string
  buses: Node[]
  templates: Node[]
  busy: boolean
  run: Run
}) {
  const [busID, setBusID] = useState("")
  const [csvFile, setCSVFile] = useState<File | null>(null)
  const [csvResult, setCSVResult] = useState("")
  const [csvReady, setCSVReady] = useState(false)
  const [sourceBusID, setSourceBusID] = useState("")
  const [templateName, setTemplateName] = useState("")
  const [templateID, setTemplateID] = useState("")
  const [targetBusID, setTargetBusID] = useState("")

  useEffect(() => {
    if (!buses.some((bus) => bus.id === busID)) setBusID(buses[0]?.id || "")
    if (!buses.some((bus) => bus.id === sourceBusID)) setSourceBusID(buses[0]?.id || "")
    if (!buses.some((bus) => bus.id === targetBusID)) setTargetBusID(buses[0]?.id || "")
    if (!templates.some((template) => template.id === templateID)) setTemplateID(templates[0]?.id || "")
  }, [buses, templates, busID, sourceBusID, targetBusID, templateID])

  const bus = buses.find((item) => item.id === busID) || buses[0]
  const sourceBus = buses.find((item) => item.id === sourceBusID) || buses[0]
  const targetBus = buses.find((item) => item.id === targetBusID) || buses[0]
  const template = templates.find((item) => item.id === templateID) || templates[0]

  return <section className="modbus-tools-workspace">
    <div className="panel modbus-tool-panel">
      <div className="query-panel-head"><div><span className="section-kicker">批量点表</span><h2>CSV 导入与导出</h2></div><span className="query-note">{buses.length} 个端口</span></div>
      <div className="modbus-tool-fields">
        <label>连接<select value={bus?.id || ""} onChange={(event) => setBusID(event.target.value)}>{buses.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}</select></label>
      </div>
      <div className="csv-import">
        <span>UTF-8 CSV；类型可填写保持寄存器、输入寄存器、线圈或离散输入。导入前先预检，确认后才写入。</span>
        <div>
          <button type="button" className="text-button" onClick={() => {
            const blob = new Blob(["﻿名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位\n柜内温度,1,10,输入寄存器,uint16,是,0.1,0,°C\n"], { type: "text/csv;charset=utf-8" })
            const url = URL.createObjectURL(blob)
            const link = document.createElement("a")
            link.href = url
            link.download = "fastiot-modbus-template.csv"
            link.click()
            URL.revokeObjectURL(url)
          }}>下载 CSV 模板</button>
          <input aria-label="CSV 文件" type="file" accept=".csv,text/csv" onChange={(event) => { setCSVFile(event.target.files?.[0] || null); setCSVResult(""); setCSVReady(false) }} />
          <button type="button" className="secondary-button" disabled={busy || !bus || !csvFile} onClick={() => void run(async () => {
            if (!bus || !csvFile) return
            const result = await importCSV(token, `/modbus/${bus.id}/io/import?preview=true`, await csvFile.text())
            setCSVReady(result.preview === true)
            setCSVResult(`预检通过：${result.count || 0} 个点位，确认后写入`)
          })}>预检点表</button>
          <button type="button" className="primary-button" disabled={busy || !bus || !csvFile || !csvReady} onClick={() => void run(async () => {
            if (!bus || !csvFile) return
            const result = await importCSV(token, `/modbus/${bus.id}/io/import`, await csvFile.text())
            setCSVReady(false)
            setCSVResult(`已导入 ${result.imported || 0} 个点位`)
          })}>确认导入</button>
          <button type="button" className="secondary-button" disabled={busy || !bus} onClick={() => void run(() => downloadCSV(token, `/modbus/${bus.id}/io/export`))}>导出当前点表</button>
        </div>
        {csvResult && <div className="success-message">{csvResult}</div>}
      </div>
    </div>

    <div className="panel modbus-tool-panel">
      <div className="query-panel-head"><div><span className="section-kicker">复用点表</span><h2>型号模板</h2></div><span className="query-note">模板不包含业务 ID 或连接凭据</span></div>
      <div className="modbus-tool-fields">
        <label>模板来源<select value={sourceBus?.id || ""} onChange={(event) => setSourceBusID(event.target.value)}>{buses.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}</select></label>
        <label>模板名称<input value={templateName} onChange={(event) => setTemplateName(event.target.value)} placeholder="例如：标准温控柜" /></label>
      </div>
      <div className="template-actions">
        <button type="button" className="secondary-button" disabled={busy || !sourceBus || !templateName} onClick={() => void run(() => request(token, "/templates", "POST", { parent: sourceBus.parent, description: templateName, source_bus_id: sourceBus.id }))}>保存为型号模板</button>
      </div>
      {templates.length > 0 && <>
        <div className="modbus-tool-fields">
          <label>型号模板<select value={template?.id || ""} onChange={(event) => setTemplateID(event.target.value)}>{templates.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}</select></label>
          <label>应用到连接<select value={targetBus?.id || ""} onChange={(event) => setTargetBusID(event.target.value)}>{buses.map((item) => <option key={item.id} value={item.id}>{nameOf(item)}</option>)}</select></label>
        </div>
        <div className="template-actions">
          <button type="button" className="primary-button" disabled={busy || !template || !targetBus} onClick={() => void run(() => request(token, `/templates/${template.id}/apply`, "POST", { bus_id: targetBus.id }))}>应用模板</button>
          <button type="button" className="text-button danger-text" disabled={busy || !template} onClick={() => {
            if (template && window.confirm(`删除型号模板“${nameOf(template)}”？`)) void run(() => request(token, `/nodes/${template.id}`, "DELETE", { parent: template.parent }))
          }}>删除模板</button>
        </div>
      </>}
      {templates.length === 0 && <div className="empty-state"><span>还没有型号模板。先选择一个端口，保存为模板后可在其他端口复用。</span></div>}
    </div>
  </section>
}
