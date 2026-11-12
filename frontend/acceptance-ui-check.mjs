// 前端验收：真正驱动界面完成交付链、发布回退、复用预览、运行页与角色只读检查。
// 运行：FASTEMS_NAV_URL=http://127.0.0.1:4409 FASTEMS_NAV_PASSWORD=... node acceptance-ui-check.mjs
import { chromium } from "playwright-core"
import { mkdirSync } from "node:fs"

const base = process.env.FASTEMS_NAV_URL || "http://127.0.0.1:4409"
const password = process.env.FASTEMS_NAV_PASSWORD || ""
const viewerEmail = process.env.FASTEMS_ACCEPT_VIEWER || "nav-viewer@example.test"
const viewerPassword = process.env.FASTEMS_ACCEPT_VIEWER_PASSWORD || "nav-viewer-pass-123"
if (!password) throw new Error("FASTEMS_NAV_PASSWORD is required")
const outDir = "evidence/acceptance-ui"
mkdirSync(outDir, { recursive: true })

const results = []
const ok = (condition, label) => {
  results.push({ label, pass: Boolean(condition) })
  console.log(`${condition ? "PASS" : "FAIL"}  ${label}`)
}
const h1 = async (page) => (await page.locator(".console-page-head h1").first().textContent())?.trim() || ""
const login = await fetch(`${base}/api/login`, {
  method: "POST", headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
}).then((r) => r.json())
if (!login.token) throw new Error("admin login failed")

const browser = await chromium.launch()
const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
await context.addInitScript((value) => sessionStorage.setItem("fastems-token", value), login.token)
const page = await context.newPage()
await page.goto(base)
await page.locator(".console-topbar").waitFor({ timeout: 15000 })

// 1. 默认落在交付验收，只有一个项目/站点。
await page.waitForFunction(() => document.querySelector(".console-page-head h1")?.textContent?.trim() === "交付验收", null, { timeout: 10000 }).catch(() => {})
ok((await h1(page)) === "交付验收", "管理员登录后落在交付验收")
ok(await page.locator(".console-station-label").count() === 0, "单一项目时不显示重复的项目名")
const siteCountText = await page.locator(".site-pill").first().textContent()
ok(/站点 1 \/ 1/.test(siteCountText || ""), `站点计数应为一站（实际 ${siteCountText?.trim()}）`)

// 2. 七步主链与状态真实性。
await page.waitForTimeout(600)
const stepTexts = await page.locator(".workbench-steps button").allTextContents()
ok(stepTexts.some((text) => text.includes("站点") && text.includes("✓")), "站点步骤已完成")
ok(stepTexts.some((text) => text.includes("通道") && text.includes("进行中")), "通道步骤未启动时显示进行中")
ok(stepTexts.some((text) => text.includes("点位与映射") && text.includes("部分")), "有警告的步骤显示部分而不是完成")
await page.screenshot({ path: `${outDir}/01-steps.png` })

// 3. 运行预检并确认快照落库。
await page.getByRole("button", { name: "运行预检" }).click()
await page.waitForFunction(() => document.body.innerText.includes("最近预检"), null, { timeout: 15000 }).catch(() => {})
const gateText = await page.locator("main").textContent()
ok(gateText.includes("最近预检"), "预检后显示最近一次快照")
ok(gateText.includes("通道") && gateText.includes("阻断"), "通道门槛阻断并说明下一步")
const preflightStatus = await fetch(`${base}/api/delivery/preflight`, { headers: { Authorization: `Bearer ${login.token}` } }).then((r) => r.json())
ok(Boolean(preflightStatus.latest?.id), "预检快照已写入并可读取")
await page.screenshot({ path: `${outDir}/02-preflight.png` })

// 4. 冻结版本（预检阻断时二次确认）。
await page.getByRole("tab", { name: "版本与发布" }).click()
await page.waitForTimeout(500)
await page.getByRole("button", { name: "冻结版本" }).click()
await page.waitForFunction(() => document.body.innerText.includes("已冻结版本") || document.body.innerText.includes("预检未通过"), null, { timeout: 10000 }).catch(() => {})
const confirmCount = await page.getByRole("button", { name: /预检未通过，仍要冻结版本/ }).count()
if (confirmCount > 0) await page.getByRole("button", { name: /预检未通过，仍要冻结版本/ }).first().click()
await page.waitForFunction(() => document.body.innerText.includes("已冻结版本"), null, { timeout: 10000 }).catch(() => {})
ok((await page.locator("main").textContent()).includes("已冻结版本"), "预检阻断时确认后成功冻结版本")
await page.waitForFunction(() => document.body.innerText.includes("回退到此版本"), null, { timeout: 8000 }).catch(() => {})
ok(await page.getByRole("button", { name: "回退到此版本" }).count() > 0, "版本列表出现回退入口")
await page.screenshot({ path: `${outDir}/03-versions.png` })

// 5. 回退两步确认。
await page.getByRole("button", { name: "回退到此版本" }).first().click()
await page.waitForTimeout(400)
ok(await page.getByRole("button", { name: /确认回退（历史保留）/ }).count() === 1, "回退需要二次确认")
await page.getByRole("button", { name: /确认回退（历史保留）/ }).first().click()
await page.waitForFunction(() => document.body.innerText.includes("已回退到"), null, { timeout: 10000 }).catch(() => {})
ok((await page.locator("main").textContent()).includes("已回退到"), "回退完成并提示历史未回滚")

// 6. 复用预览（不创建，避免污染数据）。
const siteSelect = page.getByLabel("源项目")
await page.locator(".workbench-steps button", { hasText: "站点" }).first().click()
await page.waitForFunction(() => document.querySelectorAll('select[aria-label="源项目"] option').length > 1, null, { timeout: 8000 }).catch(() => {})
ok(await siteSelect.locator("option").count() > 1, "演示站作为已发布复用源可选")
await siteSelect.selectOption(await siteSelect.locator("option").nth(1).getAttribute("value"))
await page.getByLabel("新站名称").fill("清理后验收预览")
await page.getByRole("button", { name: "预览复用" }).click()
await page.waitForFunction(() => document.body.innerText.includes("待确认"), null, { timeout: 10000 }).catch(() => {})
ok((await page.locator("main").textContent()).includes("待确认"), "复用预览给出待确认清单")
await page.screenshot({ path: `${outDir}/04-reuse.png` })

// 7. 交付验收任务内的设备验证页：保留接入入口。
await page.locator(".console-topnav .topnav-trigger", { hasText: "交付验收" }).click()
await page.locator(".console-subnav button", { hasText: "设备验证" }).click()
await page.waitForFunction(() => document.querySelector(".console-page-head h1")?.textContent?.trim() === "设备台账与验证", null, { timeout: 8000 }).catch(() => {})
ok(await page.getByRole("button", { name: /接入新设备/ }).count() === 1, "交付侧设备台账保留接入入口")
await page.screenshot({ path: `${outDir}/05-ledger.png` })

// 8. 运行侧：报警与追溯可用（追溯从总览进入）。
await page.locator(".console-workbench button", { hasText: "客户" }).click()
await page.waitForTimeout(500)
await page.locator(".console-subnav button", { hasText: "报警" }).click()
await page.waitForFunction(() => document.querySelector(".console-page-head h1")?.textContent?.trim() === "报警与事件", null, { timeout: 8000 }).catch(() => {})
const alarmText = await page.locator("main").textContent()
ok(alarmText.includes("活动报警"), "报警页显示活动报警统计")
ok(alarmText.includes("连接中断") || alarmText.includes("报警"), "报警记录可见")
const ackButton = page.getByRole("button", { name: "确认接手" })
if (await ackButton.count()) {
  await ackButton.first().click()
  await page.waitForTimeout(300)
  ok((await page.locator("body").textContent()).includes("处理说明"), "确认报警会打开处理说明对话框")
  await page.getByRole("button", { name: "取消" }).first().click()
}
await page.screenshot({ path: `${outDir}/06-alarms.png` })

await page.locator(".console-subnav button", { hasText: "总览" }).click()
await page.waitForTimeout(400)
await page.getByRole("button", { name: /调度详情/ }).first().click()
await page.waitForFunction(() => document.querySelector(".console-page-head h1")?.textContent?.trim() === "调度追溯", null, { timeout: 8000 }).catch(() => {})
ok((await page.locator("main").textContent()).includes("调度追溯"), "调度追溯可从总览进入")
await page.screenshot({ path: `${outDir}/07-trace.png` })

// 9. 窄屏：步骤条两行网格，无页面横向溢出。
await page.setViewportSize({ width: 375, height: 812 })
await page.locator(".console-workbench button", { hasText: "交付" }).click()
await page.waitForFunction(() => document.querySelector(".console-page-head h1")?.textContent?.trim() === "交付验收", null, { timeout: 8000 }).catch(() => {})
await page.waitForTimeout(500)
const mobile = await page.evaluate(() => {
  const rail = document.querySelector(".workbench-steps")
  const buttons = [...document.querySelectorAll(".workbench-steps button")]
  return {
    display: rail ? getComputedStyle(rail).display : "",
    buttons: buttons.length,
    inside: buttons.every((b) => { const r = b.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth + 1 }),
    scrollW: document.documentElement.scrollWidth,
    vw: innerWidth,
  }
})
ok(mobile.display === "grid", "窄屏步骤条改为网格布局")
ok(mobile.buttons === 7, `窄屏可见全部 7 个步骤（实际 ${mobile.buttons}）`)
ok(mobile.inside, "窄屏步骤按钮都在视口内")
ok(mobile.scrollW === mobile.vw, "窄屏页面无横向溢出")
await page.screenshot({ path: `${outDir}/08-mobile-steps.png` })
await context.close()

// 10. 查看者：只读运行总览，没有配置入口。
const viewerLogin = await fetch(`${base}/api/login`, {
  method: "POST", headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: viewerEmail, password: viewerPassword }),
}).then((r) => r.json())
if (!viewerLogin.token) throw new Error("viewer login failed")
const viewerContext = await browser.newContext({ viewport: { width: 1280, height: 800 } })
await viewerContext.addInitScript((value) => sessionStorage.setItem("fastems-token", value), viewerLogin.token)
const viewerPage = await viewerContext.newPage()
await viewerPage.goto(base)
await viewerPage.locator(".console-topbar").waitFor({ timeout: 15000 })
ok(await viewerPage.locator(".console-workbench").count() === 0, "查看者没有工作台切换")
ok((await viewerPage.locator(".console-topnav .topnav-trigger").allTextContents()).join("|") === "运行", "查看者只有运行任务")
ok(await viewerPage.getByRole("button", { name: /接入新设备/ }).count() === 0, "查看者总览没有接入口")
ok(await viewerPage.getByRole("button", { name: /通道配置/ }).count() === 0, "查看者总览没有通道配置")
await viewerPage.screenshot({ path: `${outDir}/09-viewer-overview.png` })
await viewerContext.close()

await browser.close()
const failed = results.filter((item) => !item.pass)
console.log(`\nacceptance: ${results.length - failed.length}/${results.length} passed`)
if (failed.length) {
  console.log("failed:")
  for (const item of failed) console.log(" -", item.label)
  process.exitCode = 1
}
