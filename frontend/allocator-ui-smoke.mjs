import { chromium } from "playwright-core"
import { writeFile } from "node:fs/promises"

const base = process.env.FASTEMS_URL || "http://127.0.0.1:62951"
const password = process.env.FASTEMS_ADMIN_PASSWORD || ""
if (!password) throw new Error("FASTEMS_ADMIN_PASSWORD is required")

const loginResponse = await fetch(`${base}/api/login`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ email: "admin", password }),
})
if (!loginResponse.ok) throw new Error(`login ${loginResponse.status}`)
const { token } = await loginResponse.json()

async function api(path, method = "GET", body) {
  const response = await fetch(`${base}/api${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${await response.text()}`)
  return response.json()
}

const root = (await api("/snapshot")).roots[0]
const group = (await api("/nodes", "POST", { parent: root, type: "group", description: "UI分配器储能系统" })).id
const power = (await api("/nodes", "POST", { parent: group, type: "variable", description: "UI PCS功率", value: 0 })).id
const cap = (await api("/nodes", "POST", { parent: group, type: "variable", description: "UI BMS允许功率", value: 200 })).id
const actual = (await api("/nodes", "POST", { parent: group, type: "variable", description: "UI PCS实际功率", value: 0 })).id

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
    if (response.status() >= 400) badResponses.push(`${response.status()} ${response.url()}`)
  })
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-shell")
  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("功率分配器")')
  await page.waitForSelector(".allocator-page")
  await page.selectOption(".allocator-toolbar select", group)
  await page.click('button:has-text("+ 添加 PCS 成员")')
  await page.getByLabel("最大放电功率（kW）").fill("200")
  await page.getByLabel("最大充电功率（kW）").fill("200")
  await page.getByLabel("可用放电功率（可选）").selectOption(cap)
  await page.getByLabel("可用充电功率（可选）").selectOption(cap)
  await page.getByLabel("实际功率回读（可选）").selectOption(actual)
  await page.getByRole("button", { name: "保存分配器" }).click()
  await page.waitForTimeout(1500)

  const saved = await api(`/ems/allocators/${group}`)
  if (saved.members.length !== 1 || saved.members[0].target.node_id !== power ||
    saved.members[0].available_discharge.node_id !== cap || saved.members[0].readback.node_id !== actual) {
    throw new Error(`saved allocator mismatch: ${JSON.stringify(saved)}`)
  }
  await page.reload()
  await page.waitForSelector(".console-shell")
  await page.click(".console-chip")
  await page.click('.console-directory button:has-text("功率分配器")')
  await page.waitForSelector(".allocator-member")
  if (await page.getByLabel("最大放电功率（kW）").inputValue() !== "200") {
    throw new Error("allocator did not restore after reload")
  }
  await page.click('button:has-text("+ 添加 PCS 成员")')
  await page.waitForSelector(".allocator-validation")
  const duplicate = await page.locator(".allocator-validation").first().textContent()
  if (!duplicate?.includes("功率目标不能重复")) {
    throw new Error(`duplicate target was not rejected: ${duplicate}`)
  }
  await page.click('button:has-text("恢复已保存")')
  await page.waitForSelector(".allocator-member")
  await page.screenshot({ path: "evidence/allocator-ui.png", fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator(".allocator-member").scrollIntoViewIfNeeded()
  await page.screenshot({ path: "evidence/allocator-ui-mobile.png" })
  if (errors.length || badResponses.length) {
    throw new Error(`browser errors: ${JSON.stringify({ errors, badResponses })}`)
  }
  const result = { saved: true, reload_restored: true, duplicate_target_rejected: true, console_errors: [], http_errors: [] }
  await writeFile("evidence/allocator-ui.json", JSON.stringify(result, null, 2))
  console.log("ALLOCATOR UI PASSED: create, bind, save, reload; no console or HTTP errors")
} finally {
  await browser.close()
}
