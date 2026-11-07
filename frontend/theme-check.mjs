import { chromium } from "playwright-core"
import { readFile, writeFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.EASYEMS_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.EASYEMS_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
const password = (await readFile(passwordFile, "utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || ""
const evidenceDir = process.env.EASYEMS_EVIDENCE ? path.resolve(process.env.EASYEMS_EVIDENCE) : path.resolve("../evidence")
if (!password) throw new Error("password file is empty")

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) }
catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }
const failures = []
const report = {}
const page = await browser.newPage({ viewport: { width: 1669, height: 1026 } })
try {
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-stage")
  await page.waitForTimeout(600)

  const check = (name, ok, detail = "") => { if (!ok) failures.push(`${name}${detail ? `: ${detail}` : ""}`) }
  const values = await page.evaluate(() => {
    const css = (selector, pseudo) => getComputedStyle(document.querySelector(selector), pseudo)
    const before = css("body", "::before")
    const after = css("body", "::after")
    const shell = css(".console-shell")
    const card = css(".kpi-card")
    const panel = css(".flow-panel")
    const value = css(".kpi-value")
    const unit = css(".kpi-value small")
    const note = css(".kpi-note")
    const strip = css(".kpi-strip")
    const energy = css(".kpi-card.energy .kpi-icon")
    const revenue = css(".kpi-card.revenue .kpi-icon")
    const accent = css(".kpi-card.energy", "::before")
    const heading = css(".console-page-head h1")
    const shellBox = document.querySelector(".console-shell").getBoundingClientRect()
    const model = document.querySelector(".flow-photo")
    const canvas = document.querySelector(".flow-canvas").getBoundingClientRect()
    return {
      before: { filter: before.filter, image: before.backgroundImage },
      after: after.backgroundImage,
      shell: { backdrop: shell.backdropFilter, x: shellBox.x, y: shellBox.y, width: shellBox.width, height: shellBox.height },
      card: { background: card.backgroundImage, border: card.border, radius: card.borderRadius, blur: card.backdropFilter, shadow: card.boxShadow },
      panel: { background: panel.backgroundImage, border: panel.border, radius: panel.borderRadius, blur: panel.backdropFilter },
      value: { size: value.fontSize, weight: value.fontWeight, spacing: value.letterSpacing },
      unit: unit.fontSize,
      note: note.fontSize,
      stripGap: strip.gap,
      icons: { energy: energy.color, revenue: revenue.color },
      accentDisplay: accent.display,
      heading: heading.fontSize,
      labels: [...document.querySelectorAll(".kpi-label")].map((el) => el.textContent),
      activeView: document.querySelector(".flow-views button.active")?.textContent,
      model: { src: model?.getAttribute("src"), loaded: Boolean(model?.complete && model.naturalWidth), height: model?.getBoundingClientRect().height },
      canvas: { width: canvas.width, height: canvas.height },
      nodes: [...document.querySelectorAll(".flow-canvas .flow-node")].map((el) => {
        const box = el.getBoundingClientRect()
        return { className: el.className, inside: box.left >= canvas.left && box.right <= canvas.right + 1 && box.top >= canvas.top - 1 && box.bottom <= canvas.bottom + 1 }
      }),
      assistantOpen: Boolean(document.querySelector(".assistant-panel")),
      mockText: document.querySelector(".console-stage")?.textContent?.includes("Mock"),
      dispatchText: document.querySelector(".flow-status")?.textContent || "",
      eventText: document.querySelector(".flow-event")?.textContent || "",
      healthDots: document.querySelectorAll(".flow-state").length,
    }
  })
  report.values = values

  check("背景无整页模糊", !/blur/i.test(values.before.filter), values.before.filter)
  check("背景使用图1", values.before.image.includes("ems-site.jpg"))
  check("背景叠加指定渐变", values.after.includes("rgba(236, 245, 255, 0.55)") && values.after.includes("rgba(226, 238, 250, 0.18) 40%"))
  check("外壳不模糊背景", values.shell.backdrop === "none")
  check("参考尺寸外边距", Math.abs(values.shell.x - 4) < 1 && Math.abs(values.shell.y - 4) < 1, JSON.stringify(values.shell))
  check("KPI 130° 双层渐变", values.card.background.includes("rgba(249, 252, 255, 0.86)") && values.card.background.includes("rgba(230, 239, 252, 0.62)"), values.card.background)
  check("KPI 24px 玻璃", values.card.blur.includes("blur(24px)") && values.card.blur.includes("saturate(1.4)"))
  check("KPI 圆角 9px", values.card.radius === "9px")
  check("能流面板 125° 双层渐变", values.panel.background.includes("rgba(246, 251, 255, 0.86)") && values.panel.background.includes("rgba(225, 237, 250, 0.62)"))
  check("能流面板 1px 半透白边", values.panel.border.includes("rgba(255, 255, 255, 0.34)"))
  check("能流面板圆角 10px", values.panel.radius === "10px")
  check("KPI 主值 29px/560", values.value.size === "29px" && values.value.weight === "560")
  check("KPI 字距 -1.3px", values.value.spacing === "-1.3px")
  check("KPI 单位 17px", values.unit === "17px")
  check("KPI 说明 14px", values.note === "14px")
  check("KPI 间距 6px", values.stripGap === "6px")
  check("光伏图标绿色", values.icons.energy === "rgb(43, 138, 87)")
  check("无额外左上色块", values.accentDisplay === "none")
  check("页头 32px", values.heading === "32px")
  check("8 项指标与图2一致", values.labels.join("|") === "总收益|储能收益|转换效率|今日电费|储能吞吐|光伏发电量|今日用电|最大需量", values.labels.join("|"))
  check("默认 3D 能流", values.activeView === "3D能流", values.activeView)
  check("使用原始等距场站素材", values.model.src === "/station-model.png" && values.model.loaded)
  check("五个能流标签都在画布内", values.nodes.length === 5 && values.nodes.every((node) => node.inside), JSON.stringify(values.nodes))
  check("AI 默认收起", !values.assistantOpen)
  check("没有 Mock 文案", !values.mockText)
  check("首页显示当前调度", values.dispatchText.includes("调度") && values.dispatchText.includes("调度详情"), values.dispatchText)
  check("今日事件显示最近评估", values.eventText.includes("最近评估") || values.eventText.includes("暂无可核实"), values.eventText)
  check("五个能流节点均有状态标记", values.healthDots === 5, String(values.healthDots))

  await page.getByRole("button", { name: "趋势 + 预测" }).click()
  await page.waitForSelector(".history-page")
  check("趋势入口可用", (await page.locator(".console-page-head h1").innerText()) === "数据与追溯")
  await page.locator(".console-bottomnav button").filter({ hasText: "总览" }).click()
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
    check(`${w}x${h} 无页面/舞台/横向滚动`, m.stage === 0 && m.inner === 0 && m.horizontal === 0, JSON.stringify(m))
  }

  report.passed = failures.length === 0
  report.failures = failures
  await writeFile(path.join(evidenceDir, "reference-replica-check.json"), JSON.stringify(report, null, 2) + "\n")
  if (!report.passed) throw new Error(failures.join(" | "))
  console.log("Reference replica check passed", JSON.stringify(report.layout))
} finally {
  await browser.close()
}
