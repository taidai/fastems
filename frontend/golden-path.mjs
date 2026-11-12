// 黄金路径：在隔离实例上按 J1→J5 驱动真实界面，记录每步耗时与预检门槛。
// 由 scripts/golden-path.sh 启动全新实例后调用；不连接演示数据。
// 运行：FASTEMS_GOLDEN_URL=http://127.0.0.1:4412 FASTEMS_GOLDEN_PASSWORD=... node golden-path.mjs
import { chromium } from "playwright-core"
import { mkdirSync, writeFileSync } from "node:fs"

const base = process.env.FASTEMS_GOLDEN_URL || "http://127.0.0.1:4412"
const password = process.env.FASTEMS_GOLDEN_PASSWORD || ""
if (!password) throw new Error("FASTEMS_GOLDEN_PASSWORD is required")
mkdirSync("../evidence", { recursive: true })

const runID = new Date().toISOString().replace(/[-:.TZ]/g, "").slice(0, 14)
const steps = []
const gates = []
let failed = false

async function api(token, path, method = "GET", body) {
  const response = await fetch(`${base}/api${path}`, {
    method,
    headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}), ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await response.text()
  let data = {}
  try { data = text ? JSON.parse(text) : {} } catch { data = { raw: text } }
  return { status: response.status, data }
}

async function timed(key, label, action, required = true) {
  const started = performance.now()
  let ok = true
  let note = ""
  try {
    note = await action() || ""
  } catch (error) {
    ok = false
    note = error instanceof Error ? error.message : String(error)
  }
  const ms = Math.round(performance.now() - started)
  steps.push({ key, label, ms, ok, note })
  console.log(`${ok ? "PASS" : "FAIL"} ${label} ${ms}ms${note ? ` — ${note}` : ""}`)
  if (!ok && required) failed = true
  return ok
}

const login = await api(null, "/login", "POST", { email: "admin", password })
if (!login.data.token) throw new Error(`login failed: ${login.status} ${JSON.stringify(login.data)}`)
const token = login.data.token

// 准备：等演示站落库并发布，作为复用源；这是环境准备，不计入任务耗时。
let site = null
for (let attempt = 0; attempt < 60 && !site; attempt++) {
  const snapshot = await api(token, "/snapshot")
  site = (snapshot.data.nodes || []).find((node) => node.type === "site") || null
  if (!site) await new Promise((resolve) => setTimeout(resolve, 1000))
}
if (!site) throw new Error("seeded instance has no site after 60s")
const publish = await api(token, "/projects/publish", "POST", { site: site.id, note: "黄金路径基线", confirm: true })
if (![200, 201].includes(publish.status)) throw new Error(`baseline publish failed: ${publish.status} ${JSON.stringify(publish.data)}`)

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) } catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }
const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
await context.addInitScript((value) => sessionStorage.setItem("fastems-token", value), token)
const page = await context.newPage()
await page.goto(base)
await page.locator(".console-topbar").waitFor({ timeout: 20000 })

const title = async () => (await page.locator(".console-page-head h1").first().textContent())?.trim() || ""
const waitTitle = async (expected) => {
  await page.waitForFunction((value) => document.querySelector(".console-page-head h1")?.textContent?.trim() === value, expected, { timeout: 15000 })
  const actual = await title()
  if (actual !== expected) throw new Error(`expected ${expected}, got ${actual}`)
}

await timed("landing", "登录后进入交付验收", async () => {
  await waitTitle("交付验收")
  return "交付验收任务清单"
})

await timed("preflight", "运行预检并读取门槛", async () => {
  await page.getByRole("button", { name: "运行预检" }).click()
  await page.waitForFunction(() => document.body.innerText.includes("最近预检"), null, { timeout: 20000 })
  // 以站点为单位再跑一次，确保快照归属当前站点；返回值就是本次门槛。
  const report = await api(token, `/delivery/preflight?site=${site.id}`, "POST")
  if (report.status !== 200) throw new Error(`preflight failed: ${report.status}`)
  for (const gate of report.data.gates || []) {
    gates.push({ key: gate.key, title: gate.title, status: gate.status, detail: gate.detail })
  }
  return `${gates.filter((gate) => gate.status === "fail").length} 项阻断`
})

await timed("reuse", "复用预览（列出待确认项）", async () => {
  const source = page.getByLabel("源项目")
  await page.waitForFunction(() => document.querySelectorAll('select[aria-label="源项目"] option').length > 1, null, { timeout: 10000 })
  await source.selectOption(await source.locator("option").nth(1).getAttribute("value"))
  await page.getByLabel("新站名称").fill(`黄金路径-${runID}`)
  await page.getByRole("button", { name: "预览复用" }).click()
  await page.waitForFunction(() => document.body.innerText.includes("待确认"), null, { timeout: 15000 })
  return "待确认清单已生成"
})

await timed("access", "接入：通道与点位表", async () => {
  await page.locator(".console-topnav .topnav-trigger", { hasText: "接入" }).click()
  await waitTitle("通道")
  await page.locator(".console-subnav button", { hasText: "点位表" }).click()
  await waitTitle("点位表")
  return "通道、点位表可到达"
})

await timed("assets", "能源设备：储能柜", async () => {
  await page.locator(".console-topnav .topnav-trigger", { hasText: "能源设备" }).click()
  await waitTitle("储能柜与策略")
  return "储能柜页可到达"
})

await timed("strategy", "策略：决策表", async () => {
  await page.locator(".console-topnav .topnav-trigger", { hasText: "策略" }).click()
  await waitTitle("决策表")
  return "决策表可到达"
})

await timed("acceptance", "回到交付验收", async () => {
  await page.locator(".console-topnav .topnav-trigger", { hasText: "交付验收" }).click()
  await waitTitle("交付验收")
  return "验收清单可到达"
})

await timed("freeze", "冻结版本并留下证据", async () => {
  await page.getByRole("tab", { name: "版本与发布" }).click()
  await page.waitForTimeout(400)
  await page.getByRole("button", { name: "冻结版本" }).click()
  await page.waitForFunction(() => document.body.innerText.includes("已冻结版本") || document.body.innerText.includes("预检未通过"), null, { timeout: 15000 })
  const confirm = page.getByRole("button", { name: /预检未通过，仍要冻结版本/ })
  if (await confirm.count()) await confirm.first().click()
  await page.waitForFunction(() => document.body.innerText.includes("已冻结版本"), null, { timeout: 15000 })
  return "版本已冻结"
})

await page.screenshot({ path: `../evidence/golden-path-${runID}.png` })
await context.close()
await browser.close()

const blocking = gates.filter((gate) => gate.status === "fail")
const report = {
  run: runID,
  base,
  generatedAt: new Date().toISOString(),
  totalMs: steps.reduce((sum, step) => sum + step.ms, 0),
  steps,
  gates,
  blockingGates: blocking.length,
  deliveryReady: blocking.length === 0,
  status: failed ? "fail" : "pass",
}
writeFileSync("../evidence/golden-path.json", JSON.stringify(report, null, 2))
console.log(`\ngolden path ${report.status}: ${steps.length} steps, ${report.totalMs}ms, ${report.blockingGates} blocking gates, deliveryReady=${report.deliveryReady}`)
if (failed) process.exit(1)
