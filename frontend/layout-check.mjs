import { chromium } from "playwright-core"
import { readFile, writeFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.EASYEMS_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.EASYEMS_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
const password = (await readFile(passwordFile, "utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || ""
const evidenceDir = process.env.EASYEMS_EVIDENCE ? path.resolve(process.env.EASYEMS_EVIDENCE) : path.resolve("../evidence")
if (!password) throw new Error("password file is empty")

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) }
catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }
const errors = []
const overflows = []
const pages = ["工作台", "设备台账", "点位表", "通道", "虚拟聚合", "决策表", "报警与事件", "数据与追溯", "部署与验收", "功率分配器", "能源设备", "账号与权限", "审计日志", "运行画面"]
const navGroups = {
  "工作台": "总览", "运行画面": "总览",
  "设备台账": "设备", "点位表": "设备", "通道": "设备", "虚拟聚合": "设备",
  "决策表": "调度", "功率分配器": "调度", "能源设备": "调度",
  "报警与事件": "数据", "数据与追溯": "数据",
  "部署与验收": "配置", "账号与权限": "配置", "审计日志": "配置",
}
const sizes = [
  { width: 1920, height: 1080 },
  { width: 1600, height: 1000 },
  { width: 1280, height: 800 },
  { width: 1366, height: 768 },
  { width: 1024, height: 768 },
  { width: 390, height: 844 },
]
const results = {}
try {
  const page = await browser.newPage({ viewport: sizes[1] })
  page.on("pageerror", (error) => errors.push(error.message))
  page.on("console", (message) => {
    if (message.type() !== "error") return
    const url = message.location().url
    if (url.endsWith("/favicon.ico")) return
    errors.push(url ? `${message.text()} (${url})` : message.text())
  })
  page.on("response", (response) => { if (response.status() >= 500) errors.push(`${response.status()} ${response.url()}`) })
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-stage")

  async function go(label) {
    await page.locator(".console-topnav .topnav-trigger").filter({ hasText: navGroups[label] }).click()
    await page.locator(".topnav-dropdown").getByRole("menuitem", { name: label, exact: true }).click()
    await page.waitForTimeout(450)
  }

  async function measure() {
    return page.evaluate(() => {
      const q = (selector) => document.querySelector(selector)
      const over = (el) => el ? Math.max(0, el.scrollHeight - el.clientHeight) : 0
      const wide = (el) => el ? Math.max(0, el.scrollWidth - el.clientWidth) : 0
      const content = q("ion-content")
      const inner = content?.shadowRoot?.querySelector(".inner-scroll")
      const tableScrollers = [...document.querySelectorAll('[class*="table-wrap"]')].map((el) => {
        const overflowX = getComputedStyle(el).overflowX
        return { className: String(el.className), horizontal: wide(el), overflowX, contained: overflowX === "auto" || overflowX === "scroll" }
      }).filter((item) => item.horizontal > 1)
      return {
        inner: over(inner),
        stage: over(q(".console-stage")),
        doc: over(document.scrollingElement),
        documentHorizontal: wide(document.scrollingElement),
        shellHorizontal: wide(q(".console-shell")),
        stageHorizontal: wide(q(".console-stage")),
        horizontal: Math.max(wide(document.documentElement), wide(q(".console-shell")), wide(q(".console-stage"))),
        internalTables: tableScrollers,
        delivery: over(q(".delivery-panel")),
        deliveryList: over(q(".delivery-list")),
        points: over(q(".point-table-wrap")),
        rules: over(q(".rule-editor")),
        alarms: over(q(".alarm-list")),
        channels: over(q(".channel-grid")),
        channelWorkspace: over(q(".channel-workspace")),
        communication: over(q(".communication-page")),
        modbusTools: over(q(".modbus-tools-workspace")),
        query: over(q(".device-query-page")),
        queryMain: over(q(".query-main")),
        queryTable: over(q(".query-table-wrap")),
        queryTree: over(q(".query-tree-body")),
        ledger: over(q(".ledger-table-wrap")),
      }
    })
  }

  function checkHorizontal(size, label, metrics) {
    if (metrics.horizontal > 1) {
      overflows.push({ size, page: label, document: metrics.documentHorizontal, shell: metrics.shellHorizontal, stage: metrics.stageHorizontal })
    }
    for (const table of metrics.internalTables) {
      if (!table.contained) overflows.push({ size, page: label, uncontainedTable: table })
    }
  }

  for (const size of sizes) {
    await page.setViewportSize(size)
    await page.waitForTimeout(350)
    const key = `${size.width}x${size.height}`
    results[key] = {}
    for (const label of pages) {
      await go(label)
      results[key][label] = await measure()
      checkHorizontal(key, label, results[key][label])
      if (size.width === 1600 && label === "工作台") {
        await page.screenshot({ path: path.join(evidenceDir, `layout-overview-1600.png`) })
      }
    }
  }

  const channelTabs = {}
  for (const size of [{ width: 1600, height: 1000 }, { width: 1280, height: 800 }, { width: 1024, height: 768 }]) {
    await page.setViewportSize(size)
    await go("通道")
    const key = `${size.width}x${size.height}`
    channelTabs[key] = {}
    for (const label of ["运行状态", "端口配置", "点表与模板"]) {
      await page.getByRole("tab", { name: label, exact: true }).click()
      await page.waitForTimeout(400)
      const metrics = await measure()
      checkHorizontal(key, `通道/${label}`, metrics)
      channelTabs[key][label] = metrics
      if (metrics.stage || metrics.channelWorkspace || metrics.communication || metrics.modbusTools || metrics.doc || metrics.horizontal) {
        throw new Error(`channel workspace overflow ${key} ${label}: ${JSON.stringify(metrics)}`)
      }
      if (size.width === 1600) await page.screenshot({ path: path.join(evidenceDir, `channel-${label === "运行状态" ? "status" : label === "端口配置" ? "config" : "tools"}-1600.png`) })
    }
    await page.getByRole("tab", { name: "运行状态", exact: true }).click()
    await page.waitForTimeout(250)
  }

  await page.setViewportSize({ width: 1600, height: 1000 })
  await go("工作台")
  await page.waitForTimeout(500)
  await page.locator(".flow-tools button").filter({ hasText: "俯视" }).click()
  const cameraTop = await page.locator(".flow-canvas").evaluate((el) => el.className)
  await page.locator(".flow-tools button").filter({ hasText: "夜间显示" }).click()
  const night = await page.locator(".flow-canvas").evaluate((el) => el.className)
  await page.locator(".flow-tools button").filter({ hasText: "标签" }).click()
  const noLabels = await page.locator(".flow-canvas").evaluate((el) => el.className)
  if (!cameraTop.includes("camera-top") || !night.includes("night") || !noLabels.includes("hide-labels")) {
    throw new Error(`flow tools did not toggle: ${cameraTop} | ${night} | ${noLabels}`)
  }
  await page.locator(".flow-tools button").filter({ hasText: "标签" }).click()
  await page.locator(".flow-tools button").filter({ hasText: "等距" }).click()
  await page.locator(".flow-tools button").filter({ hasText: "夜间显示" }).click()
  await page.screenshot({ path: path.join(evidenceDir, "overview-tools-1600.png") })

  if (!(await page.locator(".assistant-panel").count())) await page.locator(".assistant-fab").click()
  await page.waitForSelector(".assistant-panel")
  await page.locator(".assistant-suggestions button").first().click()
  await page.waitForTimeout(300)
  const answers = await page.locator(".assistant-msg").count()
  const lastAnswer = await page.locator(".assistant-msg.assistant p").last().innerText()
  if (answers < 3 || !lastAnswer.includes("功率")) throw new Error(`assistant did not answer: ${answers} ${lastAnswer}`)
  const panelBox = await page.locator(".assistant-panel").boundingBox()
  const viewport = page.viewportSize()
  if (!panelBox || panelBox.y < 0 || panelBox.y + panelBox.height > viewport.height + 1 || panelBox.x + panelBox.width > viewport.width + 1) {
    throw new Error(`assistant panel outside viewport: ${JSON.stringify(panelBox)}`)
  }
  await page.screenshot({ path: path.join(evidenceDir, "assistant-1600.png") })
  await page.keyboard.press("Escape")
  if (await page.locator(".assistant-panel").count()) throw new Error("assistant did not close on Escape")

  await page.setViewportSize({ width: 1024, height: 768 })
  await page.waitForTimeout(350)
  await go("工作台")
  await page.locator(".assistant-fab").click()
  await page.waitForTimeout(250)
  const smallBox = await page.locator(".assistant-panel").boundingBox()
  if (!smallBox || smallBox.x < 0 || smallBox.x + smallBox.width > 1024 + 1 || smallBox.y < 0 || smallBox.y + smallBox.height > 768 + 1) {
    throw new Error(`assistant panel outside 1024 viewport: ${JSON.stringify(smallBox)}`)
  }
  await page.screenshot({ path: path.join(evidenceDir, "assistant-1024.png") })
  await page.keyboard.press("Escape")

  await writeFile(path.join(evidenceDir, "layout-check.json"), JSON.stringify({
    passed: errors.length === 0 && overflows.length === 0,
    sizes: results,
    queryTabs: {},
    channelTabs,
    flowTools: true,
    assistant: { answers, lastAnswer },
    horizontalOverflows: overflows,
    errors,
  }, null, 2) + "\n")
  if (errors.length) throw new Error(`browser errors: ${errors.join(" | ")}`)
  if (overflows.length) throw new Error(`horizontal overflow: ${JSON.stringify(overflows)}`)
  console.log("Layout check complete", JSON.stringify(results, null, 1))
} finally {
  await browser.close()
}
