// 导航分组与角色可见性的浏览器检查：真实登录、真实角色，验证工作台切换与页面集合。
// 运行：FASTIOT_NAV_URL=http://127.0.0.1:4411 FASTIOT_NAV_PASSWORD=... node navigation-browser-check.mjs
import { chromium } from "playwright-core"

const base = process.env.FASTIOT_NAV_URL || "http://127.0.0.1:4411"
const password = process.env.FASTIOT_NAV_PASSWORD || ""
if (!password) throw new Error("FASTIOT_NAV_PASSWORD is required")

const viewerEmail = process.env.FASTIOT_NAV_VIEWER || "nav-viewer@example.test"
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
  await context.addInitScript((value) => sessionStorage.setItem("fastiot-token", value), token)
  const page = await context.newPage()
  await page.goto(base, { waitUntil: "domcontentloaded" })
  await page.waitForSelector(".console-topnav .topnav-trigger", { timeout: 15000 })
  await page.waitForTimeout(600)
  return { context, page }
}

const navLabels = async (page) => (await page.locator(".console-topnav .topnav-trigger").allTextContents()).map((text) => text.replace(/▾/g, "").trim())
const title = async (page) => (await page.locator(".console-page-head h1").first().textContent())?.trim() || ""

// 管理员：首次登录按角色落在交付工作台；可切到客户工作台。
{
  const { context, page } = await openAs(adminToken)
  const switchButtons = page.locator(".console-workbench button")
  ok(await switchButtons.count() === 2, `管理员应看到两个工作台按钮，实际 ${await switchButtons.count()}`)
  ok((await switchButtons.allTextContents()).join("/") === "客户/交付", "工作台按钮文案应为 客户/交付")
  ok((await title(page)) === "交付工作台", `管理员首次应落在交付工作台，实际 ${await title(page)}`)
  let labels = await navLabels(page)
  ok(labels.includes("接入建模") && labels.includes("EMS 业务") && labels.includes("系统运维"), `交付工作台导航不符：${labels.join("|")}`)
  ok(!labels.includes("运行"), `交付工作台不应出现 运行：${labels.join("|")}`)
  const workbenchText = await page.locator("main").textContent()
  ok(workbenchText.includes("投运门槛") && workbenchText.includes("点位与映射"), "交付工作台应显示步骤与投运门槛")

  // 项目复用：预览并创建新站。
  const siteSelect = page.getByLabel("源项目")
  await page.waitForFunction(() => document.querySelectorAll('select[aria-label="源项目"] option').length > 1, null, { timeout: 8000 }).catch(() => {})
  ok(await siteSelect.locator("option").count() > 1, "应至少有一个已交付站点可选")
  const sourceValue = await siteSelect.locator("option").nth(1).getAttribute("value")
  await siteSelect.selectOption(sourceValue)
  await page.getByLabel("新站名称").fill("二期验收")
  await page.getByRole("button", { name: "预览复用" }).click()
  await page.waitForTimeout(900)
  const previewText = await page.locator("main").textContent()
  ok(previewText.includes("复用") && previewText.includes("待确认"), "复用预览应显示复用/变化/待确认")
  await page.getByRole("button", { name: "创建新站" }).click()
  await page.waitForTimeout(1600)
  ok((await page.locator("main").textContent()).includes("已创建"), "创建新站后应显示结果摘要")

  // 96 点计划：保存一个时段并看到确认。
  await page.getByRole("button", { name: "策略与试算" }).click()
  await page.waitForTimeout(700)
  await page.getByLabel("计划时段 00:00").fill("50")
  await page.getByRole("button", { name: "保存 96 点计划" }).click()
  await page.waitForTimeout(900)
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
    await page.waitForTimeout(900)
    const applyText = await page.locator("main").textContent()
    ok(applyText.includes("映射") && (applyText.includes("缺失") || applyText.includes("阻断")), "批量应用预览应显示映射与缺失/阻断")
    await page.getByRole("button", { name: "应用此 Profile", exact: true }).click()
    await page.waitForTimeout(900)
    ok((await page.locator("main").textContent()).includes("已应用"), "应用后应提示已应用")
  }

  // 版本与发布：发布当前配置并出现可回退的版本。
  await page.getByRole("tab", { name: "版本与发布" }).click()
  await page.waitForTimeout(400)
  await page.getByRole("button", { name: "冻结版本" }).click()
  await page.waitForTimeout(900)
  const blocked = await page.getByRole("button", { name: /预检未通过，仍要冻结版本/ }).count()
  if (blocked > 0) {
    await page.getByRole("button", { name: /预检未通过，仍要冻结版本/ }).first().click()
    await page.waitForTimeout(1000)
  }
  const versionText = await page.locator("main").textContent()
  ok(versionText.includes("已冻结版本"), "冻结后应显示版本号")
  ok(await page.getByRole("button", { name: "回退到此版本" }).count() > 0, "版本列表应出现回退按钮")

  // 回到投运门槛，运行预检。
  await page.getByRole("tab", { name: "投运门槛" }).click()
  await page.waitForTimeout(300)

  // 预检：生成快照并显示最近一次记录。
  await page.getByRole("button", { name: "运行预检" }).click()
  await page.waitForTimeout(1500)
  const afterPreflight = await page.locator("main").textContent()
  ok(afterPreflight.includes("最近预检"), "预检后应显示最近一次快照")
  await page.screenshot({ path: "evidence/navigation-delivery.png" })

  // 交付工作台的设备台账保留接入入口。
  await page.locator(".console-topnav .topnav-trigger", { hasText: "接入建模" }).click()
  await page.locator(".topnav-dropdown button", { hasText: "设备台账" }).click()
  await page.waitForTimeout(400)
  ok((await title(page)) === "设备台账", `交付工作台应能打开设备台账，实际 ${await title(page)}`)
  ok(await page.getByRole("button", { name: /接入新设备/ }).count() === 1, "交付工作台的设备台账应保留“接入新设备”")

  // 切到客户工作台：共享页保留，并立即变成只读。
  await page.locator(".console-workbench button", { hasText: "客户" }).click()
  await page.waitForTimeout(500)
  labels = await navLabels(page)
  ok(labels.join("|") === "运行", `客户工作台只应有 运行，实际 ${labels.join("|")}`)
  ok((await title(page)) === "设备台账", `共享页在切换工作台后应保留，实际 ${await title(page)}`)
  ok(await page.getByRole("button", { name: /接入新设备/ }).count() === 0, "切到客户工作台后设备台账应为只读")
  await context.close()
}

// 查看者：没有工作台切换，只有客户页面。
{
  const viewerLogin = await api(null, "/login", "POST", { email: viewerEmail, password: viewerPassword })
  if (viewerLogin.status !== 200 || !viewerLogin.data.token) throw new Error(`viewer login failed: ${viewerLogin.status}`)
  const { context, page } = await openAs(viewerLogin.data.token)
  ok(await page.locator(".console-workbench").count() === 0, "查看者不应看到工作台切换")
  const labels = await navLabels(page)
  ok(labels.join("|") === "运行", `查看者只应有 运行，实际 ${labels.join("|")}`)
  ok((await title(page)) === "站点总览", `查看者应落在站点总览，实际 ${await title(page)}`)
  const navText = await page.locator(".console-topnav").textContent()
  for (const hidden of ["通道", "点位", "能源设备", "决策表", "部署"]) {
    ok(!navText.includes(hidden), `查看者导航不应出现 ${hidden}`)
  }
  await page.screenshot({ path: "evidence/navigation-viewer.png" })

  // 查看者能打开只读设备台账，但没有接入/绑定入口。
  await page.locator(".console-topnav .topnav-trigger").first().click()
  await page.locator(".topnav-dropdown button", { hasText: "设备台账" }).click()
  await page.waitForTimeout(400)
  ok((await title(page)) === "设备台账", `查看者应能打开设备台账，实际 ${await title(page)}`)
  ok(await page.getByRole("button", { name: /接入新设备/ }).count() === 0, "查看者的设备台账不应出现“接入新设备”")
  ok(await page.getByRole("button", { name: "查看", exact: true }).count() > 0, "只读台账的行操作应为“查看”")
  await page.screenshot({ path: "evidence/navigation-viewer-ledger.png" })

  // 通信与健康：客户可读的通信质量页，只有窗口指标，没有配置动作。
  await page.locator(".console-topnav .topnav-trigger").first().click()
  await page.locator(".topnav-dropdown button", { hasText: "通信与健康" }).click()
  await page.waitForTimeout(600)
  ok((await title(page)) === "通信与健康", `查看者应能打开通信与健康，实际 ${await title(page)}`)
  const healthText = await page.locator("main").textContent()
  ok(healthText.includes("成功率") && healthText.includes("延迟"), "通信与健康页应显示成功率与延迟")
  ok(!healthText.includes("接入新设备") && !healthText.includes("绑定 Profile"), "通信与健康页不应出现配置动作")
  await page.screenshot({ path: "evidence/navigation-viewer-health.png" })
  await context.close()
}

await browser.close()
if (failures.length) {
  console.error("navigation browser check FAILED:\n- " + failures.join("\n- "))
  process.exit(1)
}
console.log("navigation browser check passed: admin 两工作台切换正常，查看者仅客户工作台")
