import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"

const base = "http://127.0.0.1:62943"
const initial = await readFile("data/preview/initial-password.txt", "utf8")
const password = initial.match(/密码:\s*(\S+)/)?.[1] || ""
let browser
try {
  browser = await chromium.launch({ channel: "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
try {
  await page.goto(base)
  await page.getByLabel("账号", { exact: true }).fill("admin")
  await page.getByLabel("密码", { exact: true }).fill(password)
  await page.getByRole("button", { name: "登录", exact: true }).click()
  await page.getByRole("heading", { name: "站点总览", exact: true }).waitFor()
  const token = await page.evaluate(() => sessionStorage.getItem("fastems-token"))
  const response = await page.request.get(`${base}/api/snapshot`, { headers: { Authorization: `Bearer ${token}` } })
  const data = await response.json()
  for (const name of ["验收总线", "模板总线"]) {
    const bus = data.nodes.find((node) => node.points.some((point) => point.type === "description" && point.text === name))
    if (!bus?.points.some((point) => point.type === "lastGoodPoll" && point.time)) throw new Error(`${name} has no successful poll`)
  }
  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("交付检查")')
  await page.getByRole("heading", { name: "交付检查", exact: true, level: 1 }).waitFor()
  const modbus = page.locator(".delivery-item").filter({ hasText: "Modbus 连接" })
  if (!(await modbus.locator(".delivery-status").textContent())?.includes("通过")) {
    await page.getByRole("button", { name: "重新检查" }).click()
  }
  await page.waitForFunction(() => {
    const items = [...document.querySelectorAll(".delivery-item")]
    const item = items.find((entry) => entry.textContent?.includes("Modbus 连接"))
    return item?.textContent?.includes("通过")
  })
  await page.screenshot({ path: path.resolve("evidence/p0-delivery-check.png"), fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.waitForTimeout(200)
  await page.screenshot({ path: path.resolve("evidence/easyems-mobile.png"), fullPage: true })
  console.log("Final delivery evidence passed: healthy Modbus poll and responsive layout")
} finally {
  await browser.close()
}
