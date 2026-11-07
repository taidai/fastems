import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.EASYEMS_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.EASYEMS_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
const password = (await readFile(passwordFile, "utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || ""
if (!password) throw new Error("password file is empty")

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) }
catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }

const page = await browser.newPage({ viewport: { width: 1669, height: 1000 } })
const errors = []
page.on("pageerror", (error) => errors.push(error.message))
page.on("console", (message) => { if (message.type() === "error") errors.push(message.text()) })
try {
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-stage")
  await page.getByLabel("页面目录").click()
  await page.locator(".console-directory button").filter({ hasText: /^通道$/ }).click()
  await page.waitForSelector(".channel-workspace")
  await page.locator(".channel-workspace-switch button").filter({ hasText: /^端口配置$/ }).click()

  await page.locator(".communication-actions button").click()
  const modal = page.locator('.comm-modal[aria-label="端口配置"]')
  await modal.getByLabel("端口名称").fill("串口参数验收")
  await modal.getByLabel("协议").selectOption("RTU")
  await modal.getByLabel("串口", { exact: true }).fill("COM3")
  await modal.getByLabel("波特率").fill("19200")
  await modal.getByLabel("数据位").selectOption("7")
  await modal.getByLabel("校验位").selectOption("E")
  await modal.getByLabel("停止位").selectOption("2")
  await page.screenshot({ path: path.resolve("../evidence/serial-params-1669.png") })

  await page.setViewportSize({ width: 1024, height: 768 })
  await page.waitForTimeout(250)
  const box = await modal.boundingBox()
  if (!box || box.x < 0 || box.y < 0 || box.x + box.width > 1024 + 1 || box.y + box.height > 768 + 1) {
    throw new Error(`serial modal outside 1024 viewport: ${JSON.stringify(box)}`)
  }
  await page.screenshot({ path: path.resolve("../evidence/serial-params-modal-1024.png") })

  await modal.getByRole("button", { name: "保存端口" }).click()
  const row = page.locator(".communication-table tbody tr").filter({ hasText: "串口参数验收" })
  await row.waitFor()
  await row.getByRole("button", { name: "查看", exact: true }).click()
  await page.locator(".communication-tabs button").filter({ hasText: /^串口配置$/ }).click()
  const detail = await page.locator(".communication-fields").innerText()
  if (!detail.includes("7 / E / 2")) throw new Error(`serial detail missing framing: ${detail}`)
  await page.screenshot({ path: path.resolve("../evidence/serial-params-detail-1024.png") })

  page.once("dialog", (dialog) => dialog.accept())
  await page.getByRole("button", { name: "删除", exact: true }).click()
  await page.locator(".communication-table").getByText("串口参数验收", { exact: true }).waitFor({ state: "detached" })
  if (errors.length) throw new Error(errors.join(" | "))
  console.log("Serial parameter UI verified and test port removed")
} finally {
  await browser.close()
}
