import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.FASTIOT_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.FASTIOT_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
const password = (await readFile(passwordFile, "utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || ""
if (!password) throw new Error("password file is empty")

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) }
catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }

const page = await browser.newPage({ viewport: { width: 1669, height: 1000 } })
const errors = []
page.on("pageerror", (error) => errors.push(error.message))
page.on("console", (message) => { if (message.type() === "error") errors.push(message.text()) })
try {
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-stage")

  await page.getByLabel("页面目录").click()
  const labels = await page.locator(".console-directory button").allInnerTexts()
  if (labels.some((label) => label.includes("通讯组态"))) throw new Error(`communication entry still present: ${labels.join(",")}`)
  await page.locator(".console-directory button").filter({ hasText: /^通道$/ }).click()
  await page.waitForSelector(".channel-workspace")
  await page.waitForTimeout(500)
  await page.screenshot({ path: path.resolve("../evidence/channel-1669.png") })

  await page.locator(".channel-summary > button").click()
  await page.waitForSelector(".communication-page")
  await page.waitForTimeout(400)
  await page.screenshot({ path: path.resolve("../evidence/channel-config-1669.png") })

  await page.locator(".channel-workspace-switch button").filter({ hasText: /^点表与模板$/ }).click()
  await page.waitForSelector(".modbus-tools-workspace")
  await page.waitForTimeout(350)
  await page.screenshot({ path: path.resolve("../evidence/channel-tools-1669.png") })

  await page.locator(".channel-workspace-switch button").filter({ hasText: /^运行状态$/ }).click()
  await page.locator(".channel-card-foot button").first().click()
  await page.waitForSelector(".communication-page")
  await page.waitForTimeout(350)

  await page.setViewportSize({ width: 1024, height: 768 })
  await page.waitForTimeout(400)
  await page.screenshot({ path: path.resolve("../evidence/channel-config-1024.png") })

  await page.locator(".communication-actions button").click()
  await page.waitForSelector(".comm-modal")
  const portModal = await page.locator(".comm-modal").boundingBox()
  const viewport = page.viewportSize()
  if (!portModal || portModal.x < 0 || portModal.y < 0 || portModal.x + portModal.width > viewport.width + 1 || portModal.y + portModal.height > viewport.height + 1) {
    throw new Error(`port modal outside viewport: ${JSON.stringify(portModal)}`)
  }
  await page.screenshot({ path: path.resolve("../evidence/channel-add-port-1024.png") })
  await page.locator('.comm-modal button[aria-label="关闭"]').click()
  await page.locator(".communication-units-head button").click()
  await page.waitForSelector('.comm-modal[aria-label="新增单元点位"]')
  await page.locator('.comm-modal button[aria-label="关闭"]').click()

  await page.locator(".channel-workspace-switch button").filter({ hasText: /^点表与模板$/ }).click()
  await page.waitForTimeout(350)
  await page.screenshot({ path: path.resolve("../evidence/channel-tools-1024.png") })

  const overflow = await page.evaluate(() => {
    const over = (selector) => { const el = document.querySelector(selector); return el ? Math.max(0, el.scrollHeight - el.clientHeight) : 0 }
    return {
      stage: over(".console-stage"),
      workspace: over(".channel-workspace"),
      config: over(".communication-page"),
      tools: over(".modbus-tools-workspace"),
      doc: Math.max(0, (document.scrollingElement?.scrollHeight || 0) - (document.scrollingElement?.clientHeight || 0)),
      horizontal: Math.max(0, document.documentElement.scrollWidth - document.documentElement.clientWidth),
    }
  })
  if (errors.length) throw new Error(errors.join(" | "))
  if (overflow.stage || overflow.workspace || overflow.config || overflow.tools || overflow.doc || overflow.horizontal) {
    throw new Error(`channel workspace overflow at 1024: ${JSON.stringify(overflow)}`)
  }
  console.log("Channel workspace screenshots written", JSON.stringify(overflow))
} finally {
  await browser.close()
}
