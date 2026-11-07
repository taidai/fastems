import { chromium } from "playwright-core"
import { readFile, mkdir, writeFile } from "node:fs/promises"
import path from "node:path"
import { p0Steps } from "./p0-steps.mjs"

const base = process.env.EASYEMS_URL || "http://127.0.0.1:62943"
const dataDir = path.resolve(process.env.EASYEMS_DATA || "data/preview")
const evidence = path.resolve(process.env.EASYEMS_EVIDENCE || "evidence")
await mkdir(evidence, { recursive: true })
let password = process.env.EASYEMS_ADMIN_PASSWORD || ""
if (!password) {
  const initial = await readFile(path.join(dataDir, "initial-password.txt"), "utf8")
  password = initial.match(/密码:\s*(\S+)/)?.[1] || ""
}
if (!password) throw new Error("No local initial password available")

let browser
try {
  browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}
// Tablet width keeps the device tree visible; the desktop console hides it in favor of the points table.
const page = await browser.newPage({ viewport: { width: 900, height: 1000 } })
const consoleErrors = []
page.on("pageerror", (error) => consoleErrors.push(error.message))
page.on("console", (message) => {
  if (message.type() === "error") consoleErrors.push(message.text())
})

async function snapshot() {
  const token = await page.evaluate(() => sessionStorage.getItem("easyems-token"))
  const response = await page.request.get(`${base}/api/snapshot`, { headers: { Authorization: `Bearer ${token}` } })
  if (!response.ok()) throw new Error(`Snapshot failed: ${response.status()}`)
  return response.json()
}
function targetValue(data, name) {
  const node = data.nodes.find((item) => item.points.some((p) => p.type === "description" && p.text === name))
  const target = node?.points.find((p) => p.type === "valueSet")
  return target ? (target.value ?? 0) : undefined
}
async function waitTarget(name, value) {
  const deadline = Date.now() + 8000
  while (Date.now() < deadline) {
    if (targetValue(await snapshot(), name) === value) return
    await page.waitForTimeout(100)
  }
  throw new Error(`Target ${name} did not become ${value}`)
}
async function addNode(type, name, value, parent) {
  const existing = page.locator(".device-list .device-name strong").filter({ hasText: name })
  if (await existing.count()) return
  await page.getByRole("button", { name: "+ 添加", exact: true }).click()
  const form = page.locator(".add-node-form")
  await form.locator("select").nth(0).selectOption(type)
  await form.getByLabel("名称", { exact: true }).fill(name)
  if (parent) await form.locator("select").nth(1).selectOption({ label: parent })
  if (type === "variable") await form.getByLabel("初始值", { exact: true }).fill(String(value))
  await page.getByRole("button", { name: "添加到设备树" }).click()
  await page.locator(".device-list .device-name strong").filter({ hasText: name }).waitFor()
  await page.waitForTimeout(150)
}

try {
  await page.goto(base)
  await page.getByRole("heading", { name: "登录工作台" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "easyems-login.png"), fullPage: true })
  await page.getByLabel("账号", { exact: true }).fill("admin")
  await page.getByLabel("密码", { exact: true }).fill(password)
  await page.getByRole("button", { name: "登录", exact: true }).click()
  await page.getByRole("heading", { name: "站点总览", exact: true }).waitFor()
  const headingColor = await page.locator(".console-page-head h1").evaluate((element) => getComputedStyle(element).color)
  if (headingColor === "rgb(0, 0, 0)") throw new Error("Heading is unreadable on the dark background")

  await addNode("group", "演示电站")
  await addNode("variable", "温度", 35, "演示电站")
  await addNode("variable", "有人", 1, "演示电站")
  await addNode("variable", "风机", -1, "演示电站")
  await addNode("variable", "风阀", -1, "演示电站")

  await page.click(".console-chip")

  await page.click('.console-directory button:has-text("决策表")')
  await page.getByRole("button", { name: "+ 新建决策表" }).click()
  await page.getByText("有序决策表", { exact: true }).waitFor()
  await page.locator(".editor-heading h2").filter({ hasText: "新决策表" }).waitFor()
  await page.getByRole("button", { name: "+ 添加规则行" }).click()
  await page.getByLabel("输入点位").selectOption({ label: "温度" })
  await page.getByLabel("条件值").fill("30")
  await page.getByRole("button", { name: "+ 条件", exact: true }).click()
  await page.getByLabel("输入点位").nth(1).selectOption({ label: "有人" })
  await page.getByLabel("比较逻辑").nth(1).selectOption("eq")
  await page.getByLabel("条件值").nth(1).fill("1")
  await page.getByRole("button", { name: "+ 输出", exact: true }).click()
  await page.getByLabel("输出点位").selectOption({ label: "风机" })
  await page.getByLabel("输出值").fill("1")
  await page.getByRole("button", { name: "+ 输出", exact: true }).click()
  await page.getByLabel("输出点位").nth(1).selectOption({ label: "风阀" })
  await page.getByLabel("输出值").nth(1).fill("80")
  const beforeSimulation = await snapshot()
  await page.getByRole("button", { name: "开始模拟" }).click()
  await page.getByText(/命中第 1 行/).waitFor()
  const afterSimulation = await snapshot()
  if (targetValue(afterSimulation, "风机") !== targetValue(beforeSimulation, "风机") || targetValue(afterSimulation, "风阀") !== targetValue(beforeSimulation, "风阀")) {
    throw new Error("Simulation wrote device targets")
  }

  await page.getByRole("button", { name: "发布当前规则" }).click()
  await page.getByRole("button", { name: "确认发布" }).click()
  await page.getByText("已有发布版本", { exact: true }).waitFor()
  await page.locator("ion-toggle").click()
  await page.waitForFunction(() => {
    const text = document.querySelector(".last-match")?.textContent || ""
    return text.includes("最近命中") && !text.includes("无")
  })
  await page.waitForTimeout(250)
  await waitTarget("风机", 1)
  await waitTarget("风阀", 80)
  const afterPublish = await snapshot()
  if (targetValue(afterPublish, "风机") !== 1 || targetValue(afterPublish, "风阀") !== 80) {
    throw new Error("Published rule did not write both ordered outputs")
  }
  await page.screenshot({ path: path.join(evidence, "easyems-decision-table.png"), fullPage: true })
  await page.locator(".console-bottomnav button").filter({ hasText: "总览" }).click()
  await page.locator(".device-list .device-name strong").filter({ hasText: "风机" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "easyems-workbench.png"), fullPage: true })
  const fanLine = page.locator(".device-line").filter({ has: page.locator(".device-name strong").filter({ hasText: "风机" }) })
  await fanLine.getByRole("button", { name: "接管自动" }).click()
  await fanLine.getByText("人工接管", { exact: true }).waitFor()
  await fanLine.getByRole("spinbutton").fill("0")
  await fanLine.getByRole("button", { name: "下发目标值" }).click()
  await waitTarget("风机", 0)
  await page.waitForTimeout(300)
  if (targetValue(await snapshot(), "风机") !== 0) throw new Error("Manual target was overridden")
  await fanLine.getByRole("button", { name: "恢复自动" }).click()
  await waitTarget("风机", 1)
  if (targetValue(await snapshot(), "风机") !== 1) throw new Error("Resume did not restore the automatic output")
  await page.screenshot({ path: path.join(evidence, "easyems-workbench.png"), fullPage: true })
  const p0 = p0Steps({ page, snapshot, targetValue, waitTarget, addNode, evidence })
  await p0.rules()
  await p0.resources()
  await p0.modbus()
  await p0.delivery()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.waitForTimeout(200)
  await page.screenshot({ path: path.join(evidence, "easyems-mobile.png"), fullPage: true })
  const unexpectedErrors = consoleErrors.filter((message) => !message.includes("status of 409"))
  if (unexpectedErrors.length) throw new Error(`Browser console errors: ${unexpectedErrors.join(" | ")}`)
  await writeFile(path.join(evidence, "browser-smoke.json"), JSON.stringify({
    passed: true,
    checks: ["login", "group", "four points", "two-input AND", "two ordered outputs", "dry-run without writes", "publish", "enable", "manual takeover and resume", "rule copy/history/restore", "resource rename/disable/delete/move", "Modbus read/write/edit", "CSV import/export", "device template save/apply", "delivery check", "desktop", "mobile", "no console errors"],
  }, null, 2) + "\n")
  console.log("Browser smoke passed: login, points, decision table, dry-run, publish, outputs, mobile")
} catch (error) {
  await page.screenshot({ path: path.join(evidence, "easyems-browser-failure.png"), fullPage: true }).catch(() => {})
  throw error
} finally {
  await browser.close()
}
