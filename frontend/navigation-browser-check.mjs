// 导航分组与角色可见性的浏览器检查：真实登录、真实角色，验证任务导航与页面集合。
// 运行：FASTEMS_NAV_URL=http://127.0.0.1:5173 FASTEMS_NAV_PASSWORD=... node navigation-browser-check.mjs
// 导航规则见 docs/jobs.md：一级是任务，二级是页签；查证页由页面内链接进入。
import { chromium } from "playwright-core"

const base = process.env.FASTEMS_NAV_URL || "http://127.0.0.1:4411"
const password = process.env.FASTEMS_NAV_PASSWORD || ""
if (!password) throw new Error("FASTEMS_NAV_PASSWORD is required")

const viewerEmail = process.env.FASTEMS_NAV_VIEWER || "nav-viewer@example.test"
const viewerPassword = "nav-viewer-pass-123"

async function api(token, path, method = "GET", body) {
  const response = await fetch(`${base}/api${path}`, {
    method,
    headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}), ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await response.text()
  let data = {}
  try { data = text ? JSON.parse(text) : {} } catch { data = { raw: text } }
  return { status: response.status, data }
}

const login = await api(null, "/login", "POST", { email: "admin", password })
if (login.status !== 200 || !login.data.token) throw new Error(`admin login failed: ${login.status} ${JSON.stringify(login.data)}`)
const adminToken = login.data.token

const snapshot = await api(adminToken, "/snapshot")
if (snapshot.status !== 200 || !snapshot.data.roots?.length) throw new Error(`snapshot failed: ${snapshot.status}`)
const root = snapshot.data.roots[0]
const siteNode = snapshot.data.nodes?.find((node) => node.type === "site")
if (siteNode) {
  const baseline = await api(adminToken, "/projects/publish", "POST", { site: siteNode.id, note: "验收基线（脚本预置）", confirm: true })
  if (![200, 201].includes(baseline.status)) throw new Error(`baseline publish failed: ${baseline.status} ${JSON.stringify(baseline.data)}`)
}

// 查看者账号：已存在则复用（409）。
const created = await api(adminToken, "/users", "POST", { parent: root, email: viewerEmail, password: viewerPassword, role: "viewer", actions: [] })
if (![200, 201, 409].includes(created.status)) throw new Error(`create viewer failed: ${created.status} ${JSON.stringify(created.data)}`)

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) } catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }

const failures = []
const ok = (condition, message) => { if (!condition) failures.push(message) }

async function openAs(token) {
  const context = await browser.newContext({ viewport: { width: 1632, height: 1000 } })
  await context.addInitScript((value) => sessionStorage.setItem("fastems-token", value), token)
  const page = await context.newPage()
  await page.goto(base, { waitUntil: "domcontentloaded" })
  await page.waitForSelector(".console-topnav .topnav-trigger", { timeout: 15000 })
  await page.waitForTimeout(600)
  return { context, page }
}

const navLabels = async (page) => (await page.locator(".console-topnav .topnav-trigger").allTextContents()).map((text) => text.trim())
const tabLabels = async (page) => (await page.locator(".console-subnav button").allTextContents()).map((text) => text.trim())
const title = async (page) => (await page.locator(".console-page-head h1").first().textContent())?.trim() || ""
async function openTask(page, group, tab) {
  await page.locator(".console-topnav .topnav-trigger", { hasText: group }).first().click()
  if (tab) {
    await page.locator(".console-subnav button", { hasText: tab }).first().click()
  }
  await page.waitForTimeout(400)
}

// 管理员：首次登录按角色落在交付验收；一级导航是任务，页签在任务内。
{
  const { context, page } = await openAs(adminToken)
  const switchButtons = page.locator(".console-workbench button")
  ok(await switchButtons.count() === 2, `管理员应看到两个工作台按钮，实际 ${await switchButtons.count()}`)
  ok((await switchButtons.allTextContents()).join("/") === "客户/交付", "工作台按钮文案应为 客户/交付")
  ok((await title(page)) === "交付验收", `管理员首次应落在交付验收，实际 ${await title(page)}`)
  let labels = await navLabels(page)
  ok(labels.join("|") === "接入|能源设备|策略|交付验收|系统", `交付工作台一级导航应为四个任务加系统，实际 ${labels.join("|")}`)
  ok(!labels.includes("运行"), `交付工作台不应出现 运行：${labels.join("|")}`)
  const tabs = await tabLabels(page)
  ok(tabs.join("|") === "验收清单|设备验证", `交付验收页签应为 验收清单/设备验证，实际 ${tabs.join("|")}`)
  const workbenchText = await page.locator("main").textContent()
  ok(workbenchText.includes("投运门槛") && workbenchText.includes("点位与映射"), "交付验收应显示步骤与投运门槛")

  // 项目复用：预览并创建新站。
  const siteSelect = page.getByLabel("源项目")
  await page.waitForFunction(() => document.querySelectorAll('select[aria-label="源项目"] option').length > 1, null, { timeout: 8000 }).catch(() => {})
  ok(await siteSelect.locator("option").count() > 1, "应至少有一个已交付站点可选")
  const sourceValue = await siteSelect.locator("option").nth(1).getAttribute("value")
  await siteSelect.selectOption(sourceValue)
  await page.getByLabel("新站名称").fill("二期验收")
  await page.getByRole("button", { name: "预览复用" }).click()
  await page.waitForFunction(() => document.body.innerText.includes("待确认"), null, { timeout: 8000 }).catch(() => {})
  const previewText = await page.locator("main").textContent()
  ok(previewText.includes("复用") && previewText.includes("待确认"), "复用预览应显示复用/变化/待确认")
  // 只预览不创建：脚本反复运行不应向演示数据里堆测试站点。

  // 96 点计划：保存一个时段并看到确认。
  await page.getByRole("button", { name: "策略与试算" }).click()
  await page.waitForTimeout(700)
  await page.getByLabel("计划时段 00:00").fill("50")
  await page.getByRole("button", { name: "保存 96 点计划" }).click()
  await page.waitForFunction(() => document.body.innerText.includes("计划已保存"), null, { timeout: 8000 }).catch(() => {})
  ok((await page.locator("main").textContent()).includes("计划已保存"), "保存计划后应提示已保存")

  // 批量换厂商：预览逐点判定并应用。
  await page.getByRole("button", { name: "点位与映射" }).click()
  await page.waitForTimeout(700)
  const applyChannel = page.getByLabel("Profile 通道")
  if (await applyChannel.locator("option").count() > 1) {
    await applyChannel.selectOption(await applyChannel.locator("option").nth(1).getAttribute("value"))
    await page.getByLabel("目标 Profile").selectOption("enjo-pcs")
    await page.getByLabel("站号范围").fill("1")
    await page.getByRole("button", { name: "预览", exact: true }).click()
    await page.waitForFunction(() => document.body.innerText.includes("缺失"), null, { timeout: 8000 }).catch(() => {})
    const applyText = await page.locator("main").textContent()
    ok(applyText.includes("映射") && (applyText.includes("缺失") || applyText.includes("阻断")), "批量应用预览应显示映射与缺失/阻断")
    await page.getByRole("button", { name: "应用此 Profile", exact: true }).click()
    await page.waitForFunction(() => document.body.innerText.includes("已应用"), null, { timeout: 8000 }).catch(() => {})
    ok((await page.locator("main").textContent()).includes("已应用"), "应用后应提示已应用")
  }

  // 版本与发布：发布当前配置并出现可回退的版本。
  await page.getByRole("tab", { name: "版本与发布" }).click()
  await page.waitForTimeout(400)
  await page.getByRole("button", { name: "冻结版本" }).click()
  await page.waitForFunction(() => document.body.innerText.includes("已冻结版本") || document.body.innerText.includes("预检未通过"), null, { timeout: 8000 }).catch(() => {})
  const blocked = await page.getByRole("button", { name: /预检未通过，仍要冻结版本/ }).count()
  if (blocked > 0) {
    await page.getByRole("button", { name: /预检未通过，仍要冻结版本/ }).first().click()
    await page.waitForFunction(() => document.body.innerText.includes("已冻结版本"), null, { timeout: 8000 }).catch(() => {})
  }
  const versionText = await page.locator("main").textContent()
  ok(versionText.includes("已冻结版本"), "冻结后应显示版本号")
  await page.waitForFunction(() => document.body.innerText.includes("回退到此版本"), null, { timeout: 8000 }).catch(() => {})
  ok(await page.getByRole("button", { name: "回退到此版本" }).count() > 0, "版本列表应出现回退按钮")

  // 回到投运门槛，运行预检。
  await page.getByRole("tab", { name: "投运门槛" }).click()
  await page.waitForTimeout(300)
  await page.getByRole("button", { name: "运行预检" }).click()
  await page.waitForTimeout(1500)
  const afterPreflight = await page.locator("main").textContent()
  ok(afterPreflight.includes("最近预检"), "预检后应显示最近一次快照")
  await page.screenshot({ path: "evidence/navigation-delivery.png" })

  // 设备验证是交付验收任务内的页签，保留接入入口。
  await openTask(page, "交付验收", "设备验证")
  ok((await title(page)) === "设备台账与验证", `交付验收应能打开设备台账，实际 ${await title(page)}`)
  ok(await page.getByRole("button", { name: /接入新设备/ }).count() === 1, "交付侧设备台账应保留“接入新设备”")

  // 切到客户工作台：落到运行总览，只显示三个运行页签。
  await page.locator(".console-workbench button", { hasText: "客户" }).click()
  await page.waitForTimeout(500)
  labels = await navLabels(page)
  ok(labels.join("|") === "运行", `客户工作台只应有 运行，实际 ${labels.join("|")}`)
  ok((await title(page)) === "站点总览", `切到客户工作台应落在站点总览，实际 ${await title(page)}`)
  ok((await tabLabels(page)).join("|") === "总览|画面|报警", `客户运行页签应为 总览/画面/报警，实际 ${(await tabLabels(page)).join("|")}`)

  // 跨工作台链接：总览的“通道配置”应切到交付工作台并打开通道，而不是被守卫弹回。
  await page.getByRole("button", { name: "通道配置" }).click()
  await page.waitForTimeout(600)
  ok((await title(page)) === "通道", `跨工作台链接应打开通道，实际 ${await title(page)}`)
  ok(await page.locator(".console-workbench button.active", { hasText: "交付" }).count() === 1, "跨工作台链接应切到交付工作台")
  await context.close()
}

// 查看者：没有工作台切换，只有运行任务；查证页由链接进入。
{
  const viewerLogin = await api(null, "/login", "POST", { email: viewerEmail, password: viewerPassword })
  if (viewerLogin.status !== 200 || !viewerLogin.data.token) throw new Error(`viewer login failed: ${viewerLogin.status}`)
  const { context, page } = await openAs(viewerLogin.data.token)
  ok(await page.locator(".console-workbench").count() === 0, "查看者不应看到工作台切换")
  const labels = await navLabels(page)
  ok(labels.join("|") === "运行", `查看者只应有 运行，实际 ${labels.join("|")}`)
  ok((await title(page)) === "站点总览", `查看者应落在站点总览，实际 ${await title(page)}`)
  const navText = await page.locator(".console-topnav").textContent()
  for (const hidden of ["通道", "点位", "能源设备", "决策表", "交付验收"]) {
    ok(!navText.includes(hidden), `查看者导航不应出现 ${hidden}`)
  }
  await page.screenshot({ path: "evidence/navigation-viewer.png" })

  // 通信与健康：从总览的健康摘要进入，只有窗口指标，没有配置动作。
  await page.getByRole("button", { name: /查看健康/ }).click()
  await page.waitForTimeout(600)
  ok((await title(page)) === "通信与健康", `查看者应能打开通信与健康，实际 ${await title(page)}`)
  const healthText = await page.locator("main").textContent()
  ok(healthText.includes("成功率") && healthText.includes("延迟"), "通信与健康页应显示成功率与延迟")
  ok(!healthText.includes("接入新设备") && !healthText.includes("绑定 Profile"), "通信与健康页不应出现配置动作")
  await page.screenshot({ path: "evidence/navigation-viewer-health.png" })

  // 回到运行总览：页签仍然可用，且没有交付配置入口。
  await page.locator(".console-subnav button", { hasText: "总览" }).click()
  await page.waitForTimeout(400)
  ok((await title(page)) === "站点总览", "运行页签应可回到总览")
  ok(await page.getByRole("button", { name: /接入新设备/ }).count() === 0, "查看者的总览不应出现接入入口")
  await context.close()
}

await browser.close()
if (failures.length) {
  console.error("navigation browser check FAILED:\n- " + failures.join("\n- "))
  process.exit(1)
}
console.log("navigation browser check passed: 任务导航、页签与角色可见性正常")
