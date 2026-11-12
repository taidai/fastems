import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.FASTIOT_URL || "http://127.0.0.1:62943"
const dataDir = path.resolve(process.env.FASTIOT_DATA || "data/preview")
const evidence = path.resolve("evidence")
const initial = await readFile(path.join(dataDir, "initial-password.txt"), "utf8")
const password = initial.match(/密码:\s*(\S+)/)?.[1] || ""

let browser
try {
  browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
try {
  await page.goto(base)
  await page.getByRole("heading", { name: "登录工作台" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "easyems-lightglass-login.png") })
  await page.getByLabel("账号", { exact: true }).fill("admin")
  await page.getByLabel("密码", { exact: true }).fill(password)
  await page.getByRole("button", { name: "登录", exact: true }).click()
  await page.getByRole("heading", { name: "今天，设备如何运行？" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "easyems-lightglass-workbench.png") })
  await page.getByRole("button", { name: /^通道/ }).first().click()
  await page.getByRole("heading", { name: "每一条通信路径，都要看得见" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "easyems-lightglass-channels.png") })
  await page.getByRole("button", { name: /^决策表/ }).first().click()
  await page.getByText("有序决策表", { exact: true }).waitFor()
  await page.screenshot({ path: path.join(evidence, "easyems-lightglass-rules.png") })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.waitForTimeout(250)
  await page.getByRole("button", { name: /^工作台/ }).first().click()
  await page.getByRole("heading", { name: "今天，设备如何运行？" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "easyems-lightglass-mobile.png"), fullPage: true })
  console.log("Screenshots written to evidence/")
} finally {
  await browser.close()
}
