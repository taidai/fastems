// 全界面清单截图：按角色枚举可见工作台、导航与页面，逐页截图并输出清单。
// 运行：FASTEMS_URL=http://127.0.0.1:4409 node ui-inventory-capture.mjs
import { chromium } from "playwright-core"
import { mkdirSync, writeFileSync } from "node:fs"

const base = process.env.FASTEMS_URL || "http://127.0.0.1:4409"
const adminPassword = process.env.FASTEMS_ADMIN_PASSWORD || "demo-local-2026"
const outRoot = "evidence/ui-inventory-2026-11-13"
mkdirSync(outRoot, { recursive: true })

const roles = [
  { key: "admin", email: "admin", password: adminPassword },
  { key: "implementer", email: "implementer@example.test", password: adminPassword },
  { key: "operator", email: "operator@example.test", password: adminPassword },
  { key: "viewer", email: "nav-viewer@example.test", password: "nav-viewer-pass-123" },
]

async function login(email, password) {
  const result = await fetch(`${base}/api/login`, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  }).then((response) => response.json())
  if (!result.token) throw new Error(`login failed: ${email}`)
  return result.token
}

const safe = (text) => text.replace(/[\\/:*?"<>|▾\s]+/g, "-").replace(/^-|-$/g, "").slice(0, 40)
const browser = await chromium.launch()
const inventory = []

// 登录页只截一次（所有角色共用同一入口）。
{
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const page = await context.newPage()
  await page.goto(base)
  await page.locator("input[type=password]").waitFor({ timeout: 15000 })
  const file = `${outRoot}/00-login.png`
  await page.screenshot({ path: file })
  inventory.push({ role: "-", workbench: "-", tab: "-", item: "登录页", title: "登录", file })
  await context.close()
}

for (const role of roles) {
  const dir = `${outRoot}/${role.key}`
  mkdirSync(dir, { recursive: true })
  const token = await login(role.email, role.password)
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  await context.addInitScript((value) => sessionStorage.setItem("fastems-token", value), token)
  const page = await context.newPage()
  await page.goto(base)
  await page.locator(".console-topbar").waitFor({ timeout: 15000 })
  await page.waitForTimeout(800)

  const workbenchButtons = await page.locator(".console-workbench button").allTextContents()
  const scopes = workbenchButtons.length ? workbenchButtons : ["客户"]
  for (const scope of scopes) {
    if (workbenchButtons.length) {
      await page.locator(".console-workbench button", { hasText: scope }).first().click()
      await page.waitForTimeout(400)
    }
    const groups = await page.locator(".console-topnav .topnav-trigger").allTextContents()
    for (const groupText of groups) {
      const group = groupText.trim()
      const trigger = () => page.locator(".console-topnav .topnav-trigger", { hasText: group }).first()
      if (!(await trigger().count())) continue
      await trigger().click()
      await page.waitForTimeout(250)
      const tabs = await page.locator(".console-subnav button").allTextContents()
      const labels = tabs.length ? tabs.map((text) => text.trim()) : [null]
      for (const item of labels) {
        if (item !== null) {
          await page.locator(".console-subnav button", { hasText: item }).first().click()
          await page.waitForTimeout(300)
        }
        await page.waitForTimeout(600)
        const title = (await page.locator(".console-page-head h1").first().textContent().catch(() => ""))?.trim() || group
        const name = `${role.key}-${safe(scope)}-${safe(group)}${item ? "-" + safe(item) : ""}.png`
        const file = `${dir}/${name}`
        await page.screenshot({ path: file })
        inventory.push({ role: role.key, workbench: scope, tab: group, item: item || group, title, file })
        console.log(`${role.key} | ${scope} | ${group}${item ? " / " + item : ""} -> ${title}`)
      }
    }

    // 查证页不进一级导航，但仍是可访问界面：从运行总览的链接进入并留档。
    const customerButton = page.locator(".console-workbench button", { hasText: "客户" })
    if (await customerButton.count()) {
      await customerButton.first().click()
      await page.waitForTimeout(300)
      const links = [
        { trigger: () => page.locator(".flow-trend").first(), label: "历史与追溯" },
        { trigger: () => page.getByRole("button", { name: /调度详情/ }).first(), label: "调度追溯" },
        { trigger: () => page.getByRole("button", { name: /查看健康/ }).first(), label: "通信与健康" },
      ]
      for (const link of links) {
        if (!(await link.trigger().count())) continue
        await link.trigger().click()
        await page.waitForTimeout(700)
        const title = (await page.locator(".console-page-head h1").first().textContent().catch(() => ""))?.trim() || link.label
        const file = `${dir}/${role.key}-客户-查证-${safe(link.label)}.png`
        await page.screenshot({ path: file })
        inventory.push({ role: role.key, workbench: "客户", tab: "查证", item: link.label, title, file })
        console.log(`${role.key} | 客户 | 查证 / ${link.label} -> ${title}`)
        // 回到总览，再点下一个链接。
        const back = page.locator(".console-subnav button", { hasText: "总览" })
        if (await back.count()) { await back.first().click(); await page.waitForTimeout(400) }
      }
    }
  }
  await context.close()
}

await browser.close()
writeFileSync(`${outRoot}/inventory.json`, JSON.stringify(inventory, null, 2))
console.log(`\n${inventory.length} screens captured`)
