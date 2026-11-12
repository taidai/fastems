import { chromium } from "playwright-core"
import { readFile, writeFile, mkdir } from "node:fs/promises"
import path from "node:path"

const base = process.env.FASTEMS_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.FASTEMS_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
const passwordText = await readFile(passwordFile, "utf8")
const password = passwordText.match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || passwordText.trim().split(/\s+/).pop() || ""
if (!password) throw new Error("password file is empty")
const evidence = path.resolve("../evidence")
await mkdir(evidence, { recursive: true })

const loginResponse = await fetch(`${base}/api/login`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
})
if (!loginResponse.ok) throw new Error(`login ${loginResponse.status}`)
const { token } = await loginResponse.json()
const snapshot = await (await fetch(`${base}/api/snapshot`, { headers: { Authorization: `Bearer ${token}` } })).json()

let browser
try {
  browser = await chromium.launch({ channel: "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}

const errors = []
const badResponses = []
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
  page.on("pageerror", (error) => errors.push(error.message))
  page.on("console", (message) => { if (message.type() === "error") errors.push(message.text()) })
  page.on("response", (response) => {
    if (response.status() >= 400 && !response.url().includes("/api/ems/screen/")) badResponses.push(`${response.status()} ${response.url()}`)
  })

  await page.goto(base)
  await page.getByRole("heading", { name: "登录工作台" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "glass-console-login.png"), fullPage: true })
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-shell")
  await page.waitForSelector(".kpi-strip")
  await page.waitForSelector(".flow-panel")
  await page.waitForTimeout(1200)

  await page.screenshot({ path: path.join(evidence, "glass-console-desktop.png"), fullPage: true })
  const desktop = await page.evaluate(() => ({
    kpis: document.querySelectorAll(".kpi-card").length,
    flowNodes: document.querySelectorAll(".flow-node").length,
    tabs: document.querySelectorAll(".console-bottomnav button").length,
    overflow: document.documentElement.scrollWidth - window.innerWidth,
    background: getComputedStyle(document.body).backgroundColor,
    kpiBackground: getComputedStyle(document.querySelector(".kpi-card")).backgroundImage,
    navBackground: getComputedStyle(document.querySelector(".console-bottomnav")).backgroundColor,
  }))

  await page.click(".console-chip")
  await page.waitForSelector(".console-directory")
  await page.screenshot({ path: path.join(evidence, "glass-console-directory.png"), fullPage: false })
  await page.click('.console-directory button:has-text("报警与事件")')
  await page.waitForSelector(".alarm-page")
  await page.waitForTimeout(500)
  await page.screenshot({ path: path.join(evidence, "glass-console-alarms.png"), fullPage: true })

  await page.locator(".console-bottomnav button").filter({ hasText: "报表" }).click()
  await page.waitForTimeout(600)
  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("数据与追溯")')
  await page.waitForSelector(".history-page")
  await page.waitForTimeout(700)
  await page.screenshot({ path: path.join(evidence, "glass-console-history.png"), fullPage: true })

  await page.locator(".console-bottomnav button").filter({ hasText: "总览" }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.waitForTimeout(900)
  const mobile = await page.evaluate(() => ({
    overflow: document.documentElement.scrollWidth - window.innerWidth,
    kpis: document.querySelectorAll(".kpi-card").length,
    kpisPerRow: getComputedStyle(document.querySelector(".kpi-strip")).gridTemplateColumns.split(" ").length,
  }))
  await page.screenshot({ path: path.join(evidence, "glass-console-mobile.png"), fullPage: true })

  const report = {
    passed: errors.length === 0 && badResponses.length === 0 && desktop.overflow <= 0 && mobile.overflow <= 0,
    base,
    snapshot: { roots: snapshot.roots?.length, nodes: snapshot.nodes?.length },
    desktop,
    mobile,
    errors,
    badResponses,
    screenshots: ["glass-console-login.png", "glass-console-desktop.png", "glass-console-directory.png", "glass-console-alarms.png", "glass-console-history.png", "glass-console-mobile.png"],
  }
  await writeFile(path.join(evidence, "glass-console.json"), JSON.stringify(report, null, 2) + "\n")
  if (!report.passed) throw new Error(`glass console check failed: ${JSON.stringify({ errors, badResponses, desktop, mobile })}`)
  console.log("Glass console smoke passed", JSON.stringify({ snapshot: report.snapshot, desktop, mobile }))
} finally {
  await browser.close()
}
