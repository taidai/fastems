// UI 评审取图：五个核心页面 × 四个断点，附带布局指标。
// 运行：FASTEMS_NAV_URL=http://127.0.0.1:4409 FASTEMS_NAV_PASSWORD=... node ui-review-capture.mjs
import { chromium } from "playwright-core"
import { mkdirSync, writeFileSync } from "node:fs"

const base = process.env.FASTEMS_NAV_URL || "http://127.0.0.1:4409"
const password = process.env.FASTEMS_NAV_PASSWORD || ""
if (!password) throw new Error("FASTEMS_NAV_PASSWORD is required")
const outDir = "evidence/ui-review-2026-11-13"
mkdirSync(outDir, { recursive: true })

const widths = [
  { width: 375, height: 812, name: "375" },
  { width: 768, height: 1024, name: "768" },
  { width: 1280, height: 800, name: "1280" },
  { width: 1632, height: 1000, name: "1632" },
]

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
const metrics = []
for (const size of widths) {
  const context = await browser.newContext({ viewport: { width: size.width, height: size.height } })
  await context.addInitScript((value) => sessionStorage.setItem("fastems-token", value), login.token)
  const page = await context.newPage()
  await page.goto(base)
  await page.locator(".console-topbar").waitFor({ timeout: 15000 })

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

    const measured = await page.evaluate(() => {
      const vw = window.innerWidth
      const topbar = document.querySelector(".console-topbar")?.getBoundingClientRect()
      const main = document.querySelector(".console-main")?.getBoundingClientRect()
      const h1 = document.querySelector(".console-page-head h1")?.textContent?.trim()
      const overflows = []
      for (const el of document.querySelectorAll(".console-main *, .console-topbar *")) {
        const rect = el.getBoundingClientRect()
        if (rect.width > 0 && rect.right > vw + 1 && rect.height > 0) {
          overflows.push({ tag: el.tagName.toLowerCase(), cls: String(el.className).slice(0, 40), right: Math.round(rect.right) })
          if (overflows.length >= 8) break
        }
      }
      return {
        vw,
        scrollW: document.documentElement.scrollWidth,
        topbarH: topbar ? Math.round(topbar.height) : null,
        mainW: main ? Math.round(main.width) : null,
        h1,
        overflows,
      }
    })
    const file = `${outDir}/${spec.key}-${size.name}.png`
    await page.screenshot({ path: file })
    metrics.push({ page: spec.key, width: size.name, ...measured })
    console.log(`${spec.key} @${size.name}: h1=${measured.h1} scrollW=${measured.scrollW} topbarH=${measured.topbarH} overflows=${measured.overflows.length}`)
  }
  await context.close()
}
await browser.close()
writeFileSync(`${outDir}/metrics.json`, JSON.stringify(metrics, null, 2))
