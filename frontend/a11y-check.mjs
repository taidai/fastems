// 无障碍检查：五个核心页面跑 axe-core（WCAG A/AA），输出违规清单。
// 运行：FASTIOT_NAV_URL=http://127.0.0.1:4409 FASTIOT_NAV_PASSWORD=... node a11y-check.mjs
import { chromium } from "playwright-core"
import AxeBuilder from "@axe-core/playwright"
import { writeFileSync } from "node:fs"

const base = process.env.FASTIOT_NAV_URL || "http://127.0.0.1:4409"
const password = process.env.FASTIOT_NAV_PASSWORD || ""
if (!password) throw new Error("FASTIOT_NAV_PASSWORD is required")

const pages = [
  { key: "overview", workbench: "客户", nav: "运行", item: "运行总览", title: "站点总览" },
  { key: "delivery", workbench: "交付", nav: null, item: null, title: "交付工作台" },
  { key: "ledger", workbench: "交付", nav: "接入建模", item: "设备台账", title: "设备台账" },
  { key: "trace", workbench: "客户", nav: "运行", item: "调度追溯", title: "调度追溯" },
  { key: "alarms", workbench: "客户", nav: "运行", item: "报警与事件", title: "报警与事件" },
]

const login = await fetch(`${base}/api/login`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
}).then((response) => response.json())
if (!login.token) throw new Error("login failed")

const browser = await chromium.launch()
const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
await context.addInitScript((value) => sessionStorage.setItem("fastiot-token", value), login.token)
const page = await context.newPage()
await page.goto(base)
await page.locator(".console-topbar").waitFor({ timeout: 15000 })

const results = []
for (const spec of pages) {
  if (spec.workbench) {
    const button = page.locator(".console-workbench button", { hasText: spec.workbench })
    if (await button.count()) await button.first().click()
    await page.waitForTimeout(300)
  }
  if (spec.nav) {
    await page.locator(".console-topnav .topnav-trigger", { hasText: spec.nav }).click()
    await page.locator(".topnav-dropdown button", { hasText: spec.item }).click()
  }
  await page.waitForFunction((title) => document.querySelector(".console-page-head h1")?.textContent?.trim() === title, spec.title, { timeout: 15000 }).catch(() => {})
  await page.waitForTimeout(500)
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
writeFileSync("evidence/ui-review-2026-11-13/a11y.json", JSON.stringify(results, null, 2))
