import { chromium } from "playwright-core"
import path from "node:path"

const base = process.argv[2] || "http://127.0.0.1:62951"
const password = process.env.FASTIOT_PASSWORD || ""
let browser
try {
  browser = await chromium.launch({ channel: "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}
const errors = []
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
page.on("pageerror", (error) => errors.push(error.message))
page.on("console", (message) => { if (message.type() === "error") errors.push(message.text()) })
try {
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-shell")

  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("运行画面")')
  await page.waitForTimeout(1400)
  await page.screenshot({ path: path.resolve("evidence/m3-screen.png"), fullPage: true })

  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("数据与追溯")')
  await page.waitForTimeout(1500)
  await page.screenshot({ path: path.resolve("evidence/m3-history.png"), fullPage: true })

  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("报警与事件")')
  await page.waitForTimeout(1200)
  await page.screenshot({ path: path.resolve("evidence/m3-alarms.png"), fullPage: true })

  await page.setViewportSize({ width: 390, height: 844 })
  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("数据与追溯")')
  await page.waitForTimeout(700)
  await page.mouse.move(195, 640)
  await page.mouse.wheel(0, 780)
  await page.waitForTimeout(600)
  await page.screenshot({ path: path.resolve("evidence/m3-history-mobile.png") })

  if (errors.length) throw new Error(errors.join(" | "))
  console.log("M3 screenshots written: history, alarms, mobile history")
} finally {
  await browser.close()
}
