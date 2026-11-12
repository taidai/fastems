// 无障碍检查：五个核心页面跑 axe-core（WCAG A/AA），输出违规清单。
// 运行：FASTEMS_NAV_URL=http://127.0.0.1:5173 FASTEMS_NAV_PASSWORD=... node a11y-check.mjs
import { chromium } from "playwright-core"
import AxeBuilder from "@axe-core/playwright"
import { mkdirSync, writeFileSync } from "node:fs"

const base = process.env.FASTEMS_NAV_URL || "http://127.0.0.1:4409"
const password = process.env.FASTEMS_NAV_PASSWORD || ""
if (!password) throw new Error("FASTEMS_NAV_PASSWORD is required")

const pages = [
  { key: "overview", title: "站点总览", go: async (page) => {
    await page.locator(".console-workbench button", { hasText: "客户" }).click()
    await page.locator(".console-subnav button", { hasText: "总览" }).click()
  } },
  { key: "delivery", title: "交付验收", go: async (page) => {
    await page.locator(".console-workbench button", { hasText: "交付" }).click()
  } },
  { key: "ledger", title: "设备台账与验证", go: async (page) => {
    await page.locator(".console-topnav .topnav-trigger", { hasText: "交付验收" }).click()
    await page.locator(".console-subnav button", { hasText: "设备验证" }).click()
  } },
  { key: "trace", title: "调度追溯", go: async (page) => {
    await page.locator(".console-workbench button", { hasText: "客户" }).click()
    await page.locator(".console-subnav button", { hasText: "总览" }).click()
    await page.getByRole("button", { name: /调度详情/ }).first().click()
  } },
  { key: "alarms", title: "报警与事件", go: async (page) => {
    await page.locator(".console-workbench button", { hasText: "客户" }).click()
    await page.locator(".console-subnav button", { hasText: "报警" }).click()
  } },
]

const login = await fetch(`${base}/api/login`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
}).then((response) => response.json())
if (!login.token) throw new Error("login failed")

const browser = await chromium.launch()
const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
await context.addInitScript((value) => sessionStorage.setItem("fastems-token", value), login.token)
const page = await context.newPage()
await page.goto(base)
await page.locator(".console-topbar").waitFor({ timeout: 15000 })

const results = []
for (const spec of pages) {
  await spec.go(page)
  await page.waitForFunction((title) => document.querySelector(".console-page-head h1")?.textContent?.trim() === title, spec.title, { timeout: 15000 }).catch(() => {})
  await page.waitForTimeout(900)
  const scan = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze()
  const violations = scan.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    nodes: violation.nodes.slice(0, 5).map((node) => ({ target: node.target.join(" "), html: node.html, failure: node.failureSummary })),
    count: violation.nodes.length,
  }))
  results.push({ page: spec.key, title: spec.title, violations })
  console.log(`${spec.key}: ${violations.length} violation types`)
  for (const violation of violations) console.log(`  [${violation.impact}] ${violation.id} ×${violation.count} — ${violation.help}`)
}
await browser.close()
mkdirSync("evidence/ui-review-2026-11-13", { recursive: true })
writeFileSync("evidence/ui-review-2026-11-13/a11y.json", JSON.stringify(results, null, 2))
