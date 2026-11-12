import { chromium } from "playwright-core"

const base = process.env.FASTIOT_NAV_URL || "http://127.0.0.1:4411"
const password = process.env.FASTIOT_NAV_PASSWORD || ""
const login = await fetch(`${base}/api/login`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email: "admin", password }) })
const { token } = await login.json()

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) } catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }
const context = await browser.newContext({ viewport: { width: 1632, height: 1000 } })
await context.addInitScript((value) => sessionStorage.setItem("fastiot-token", value), token)
const page = await context.newPage()
await page.goto(base, { waitUntil: "domcontentloaded" })
await page.waitForSelector(".console-topnav .topnav-trigger", { timeout: 15000 })
await page.waitForTimeout(800)

await page.getByRole("tab", { name: "验收进度" }).click()
await page.waitForTimeout(400)
await page.screenshot({ path: "evidence/workbench-acceptance.png", fullPage: false })

await page.getByRole("tab", { name: "版本与发布" }).click()
await page.waitForTimeout(500)
await page.screenshot({ path: "evidence/workbench-versions.png", fullPage: false })

await page.getByRole("button", { name: "策略与试算" }).click()
await page.waitForTimeout(800)
await page.screenshot({ path: "evidence/workbench-plan.png", fullPage: false })

await page.getByRole("button", { name: "点位与映射" }).click()
await page.waitForTimeout(700)
const applyChannel = page.getByLabel("Profile 通道")
if (await applyChannel.locator("option").count() > 1) {
  await applyChannel.selectOption(await applyChannel.locator("option").nth(1).getAttribute("value"))
  await page.getByLabel("目标 Profile").selectOption("enjo-pcs")
  await page.getByLabel("站号范围").fill("1")
  await page.getByRole("button", { name: "预览", exact: true }).click()
  await page.waitForTimeout(900)
}
await page.screenshot({ path: "evidence/workbench-profiles.png", fullPage: false })

await page.getByRole("tab", { name: "接入工具" }).click()
await page.waitForTimeout(600)
await page.screenshot({ path: "evidence/workbench-tools.png", fullPage: false })

await browser.close()
console.log("captured acceptance, versions, plan and tools tabs")
