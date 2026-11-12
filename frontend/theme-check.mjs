import { chromium } from "playwright-core"
import { readFile, writeFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.FASTEMS_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.FASTEMS_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
const password = (await readFile(passwordFile, "utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || ""
const evidenceDir = process.env.FASTEMS_EVIDENCE ? path.resolve(process.env.FASTEMS_EVIDENCE) : path.resolve("../evidence")
if (!password) throw new Error("password file is empty")

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) }
catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }
const failures = []
const report = {}

function parseColor(value) {
  const parts = String(value || "").match(/[\d.]+/g)?.map(Number) || []
  if (parts.length < 3 || parts.slice(0, 3).some((part) => !Number.isFinite(part))) return null
  return { rgb: parts.slice(0, 3), alpha: parts.length > 3 ? parts[3] : 1 }
}
function isLightGlass(value) {
  const color = parseColor(value)
  return Boolean(color && color.alpha > 0.3 && color.alpha < 0.98 && color.rgb.every((channel) => channel >= 180))
}
function contrast(foreground, background) {
  const fg = parseColor(foreground)
  const bg = parseColor(background)
  if (!fg || !bg) return 0
  const base = [228, 235, 239]
  const rgb = bg.rgb.map((channel, index) => channel * bg.alpha + base[index] * (1 - bg.alpha))
  const luminance = (channels) => {
    const linear = channels.map((channel) => {
      const value = channel / 255
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
    })
    return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722
  }
  const text = luminance(fg.rgb)
  const surface = luminance(rgb)
  return (Math.max(text, surface) + 0.05) / (Math.min(text, surface) + 0.05)
}

const page = await browser.newPage({ viewport: { width: 1669, height: 1026 } })
try {
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-stage")
  const customer = page.locator(".console-workbench button", { hasText: "客户" })
  if (await customer.count()) await customer.first().click()
  const overview = page.locator(".console-subnav button", { hasText: "总览" })
  if (await overview.count()) await overview.first().click()
  await page.waitForTimeout(600)

  const check = (name, ok, detail = "") => { if (!ok) failures.push(`${name}${detail ? `: ${detail}` : ""}`) }
  const values = await page.evaluate(() => {
    const css = (selector, pseudo) => getComputedStyle(document.querySelector(selector), pseudo)
    const before = css("body", "::before")
    const after = css("body", "::after")
    const card = css(".kpi-card")
    const panel = css(".flow-panel")
    const heading = css(".console-page-head h1")
    const model = document.querySelector(".flow-photo")
    const canvas = document.querySelector(".flow-canvas").getBoundingClientRect()
    return {
      colorScheme: css(":root").colorScheme,
      before: { filter: before.filter, image: before.backgroundImage },
      after: after.backgroundImage,
      card: { background: card.backgroundColor, border: card.border, blur: card.backdropFilter, shadow: card.boxShadow },
      panel: { background: panel.backgroundColor, border: panel.border, blur: panel.backdropFilter },
      readability: {
        heading: heading.color,
        label: css(".kpi-label").color,
        value: css(".kpi-value").color,
        note: css(".kpi-note").color,
      },
      labels: [...document.querySelectorAll(".kpi-label")].map((el) => el.textContent?.trim()),
      activeView: document.querySelector(".flow-views button.active")?.textContent,
      model: { src: model?.getAttribute("src"), loaded: Boolean(model?.complete && model.naturalWidth) },
      nodes: [...document.querySelectorAll(".flow-canvas .flow-node")].map((el) => {
        const box = el.getBoundingClientRect()
        const panel = document.querySelector(".flow-panel").getBoundingClientRect()
        return { className: el.className, inside: box.left >= panel.left - 1 && box.right <= panel.right + 1 && box.top >= panel.top - 1 && box.bottom <= panel.bottom + 1 }
      }),
      assistantOpen: Boolean(document.querySelector(".assistant-panel")),
      mockText: document.querySelector(".console-stage")?.textContent?.includes("Mock"),
      dispatchText: document.querySelector(".flow-status")?.textContent || "",
      eventText: document.querySelector(".flow-event")?.textContent || "",
      healthDots: document.querySelectorAll(".flow-state").length,
    }
  })
  report.values = values

  check("使用浅色主题", values.colorScheme === "light", values.colorScheme)
  check("背景使用站场照片", values.before.image.includes("ems-site.jpg"), values.before.image)
  check("背景叠加浅色渐变", values.after.includes("linear-gradient"), values.after)
  check("KPI 使用浅色半透明玻璃", isLightGlass(values.card.background), values.card.background)
  check("KPI 卡片保留背景模糊", /blur\(/i.test(values.card.blur), values.card.blur)
  check("KPI 卡片有描边与阴影", values.card.border !== "none" && values.card.shadow !== "none")
  check("能流面板使用浅色半透明玻璃", isLightGlass(values.panel.background), values.panel.background)
  check("能流面板保留背景模糊", /blur\(/i.test(values.panel.blur), values.panel.blur)
  check("页头对比度可读", contrast(values.readability.heading, "rgb(228, 235, 239)") >= 4.5, values.readability.heading)
  check("KPI 标签对比度可读", contrast(values.readability.label, values.card.background) >= 4.5, values.readability.label)
  check("KPI 数值对比度可读", contrast(values.readability.value, values.card.background) >= 4.5, values.readability.value)
  check("KPI 说明对比度可读", contrast(values.readability.note, values.card.background) >= 4.5, values.readability.note)
  check("KPI 卡片有有效标签", values.labels.length > 0 && values.labels.every(Boolean), values.labels.join("|"))
  check("默认 3D 能流", values.activeView === "3D能流", values.activeView)
  check("使用原始等距场站素材", values.model.src === "/station-model.png" && values.model.loaded)
  check("五个能流标签都在画布内", values.nodes.length === 5 && values.nodes.every((node) => node.inside), JSON.stringify(values.nodes))
  check("AI 默认收起", !values.assistantOpen)
  check("没有 Mock 文案", !values.mockText)
  check("首页显示当前调度", values.dispatchText.includes("调度") && values.dispatchText.includes("调度详情"), values.dispatchText)
  check("今日事件显示最近评估", values.eventText.includes("最近评估") || values.eventText.includes("暂无可核实"), values.eventText)
  check("五个能流节点均有状态标记", values.healthDots === 5, String(values.healthDots))

  await page.locator('.console-icon[aria-label="显示设置"]').click()
  const displayPanel = page.getByRole("dialog", { name: "显示设置", exact: true })
  check("显示设置可打开", await displayPanel.isVisible())
  const ranges = displayPanel.locator('input[type="range"]')
  check("显示设置包含背景与玻璃滑杆", await ranges.count() === 2)
  const initialDisplay = await ranges.evaluateAll((inputs) => inputs.map((input) => Number(input.value)))
  await ranges.nth(0).focus()
  await page.keyboard.press("ArrowLeft")
  await ranges.nth(1).focus()
  await page.keyboard.press("ArrowLeft")
  const expectedDisplay = initialDisplay.map((value) => value - 1)
  await page.waitForFunction(([background, glass]) => {
    const saved = JSON.parse(localStorage.getItem("fastems-display") || "{}")
    return saved.background === background && saved.glass === glass
  }, expectedDisplay)
  report.display = await page.evaluate(() => ({
    saved: JSON.parse(localStorage.getItem("fastems-display") || "{}"),
    brightness: document.documentElement.style.getPropertyValue("--bg-brightness"),
    glass: document.documentElement.style.getPropertyValue("--glass-a"),
  }))
  check("显示设置联动 CSS 变量", Number(report.display.brightness) === expectedDisplay[0] / 100 && Number(report.display.glass) === expectedDisplay[1] / 100, JSON.stringify(report.display))
  await displayPanel.getByRole("button", { name: "恢复默认", exact: true }).click()
  await page.waitForFunction(() => {
    const saved = JSON.parse(localStorage.getItem("fastems-display") || "{}")
    return saved.background === 78 && saved.glass === 62
  })
  check("显示设置恢复默认并收起", await displayPanel.count() === 0)

  await page.getByRole("button", { name: "趋势 + 预测" }).click({ force: true })
  await page.waitForSelector(".history-page")
  check("趋势入口可用", (await page.locator(".console-page-head h1").innerText()) === "历史与追溯")
  await page.locator(".console-subnav button", { hasText: "总览" }).click()
  await page.waitForSelector(".flow-panel")

  async function layout(width, height) {
    await page.setViewportSize({ width, height })
    await page.waitForTimeout(400)
    return page.evaluate(() => {
      const stage = document.querySelector(".console-stage")
      const content = document.querySelector("ion-content")
      const inner = content?.shadowRoot?.querySelector(".inner-scroll")
      return {
        stage: Math.max(0, stage.scrollHeight - stage.clientHeight),
        inner: inner ? Math.max(0, inner.scrollHeight - inner.clientHeight) : 0,
        horizontal: Math.max(0, document.documentElement.scrollWidth - document.documentElement.clientWidth),
      }
    })
  }
  report.layout = {}
  for (const [w, h] of [[1669, 1026], [1600, 1000], [1280, 800], [1366, 768], [1024, 768]]) {
    const m = await layout(w, h)
    report.layout[`${w}x${h}`] = m
    check(`${w}x${h} 无页面/横向滚动（舞台可纵向滚动）`, m.inner === 0 && m.horizontal === 0, JSON.stringify(m))
  }

  report.passed = failures.length === 0
  report.failures = failures
  await writeFile(path.join(evidenceDir, "reference-replica-check.json"), JSON.stringify(report, null, 2) + "\n")
  if (!report.passed) throw new Error(failures.join(" | "))
  console.log("Reference replica check passed", JSON.stringify(report.layout))
} finally {
  await browser.close()
}
