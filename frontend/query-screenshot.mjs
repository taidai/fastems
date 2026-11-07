import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.EASYEMS_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.EASYEMS_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
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
  await page.locator(".console-directory button").filter({ hasText: /^设备数据查询$/ }).click()
  await page.waitForSelector(".device-query-page")
  await page.waitForTimeout(600)
  await page.fill(".query-tree input", "演示")
  await page.waitForTimeout(250)
  const filteredTree = await page.locator(".query-tree-body").innerText()
  await page.fill(".query-tree input", "")
  await page.waitForTimeout(250)
  if (!filteredTree.includes("演示电站") || filteredTree.includes("备用区域")) {
    throw new Error(`device tree search did not filter: ${filteredTree}`)
  }
  await page.screenshot({ path: path.resolve("../evidence/device-query-1669.png") })

  await page.locator(".query-tabs button").filter({ hasText: /^实时数据$/ }).click()
  await page.waitForTimeout(350)
  await page.screenshot({ path: path.resolve("../evidence/device-query-realtime-1669.png") })

  await page.locator(".query-table tbody tr").first().click()
  await page.waitForTimeout(900)
  await page.screenshot({ path: path.resolve("../evidence/device-query-history-1669.png") })

  await page.setViewportSize({ width: 1024, height: 768 })
  await page.locator(".query-tabs button").filter({ hasText: /^概览$/ }).click()
  await page.waitForTimeout(500)
  await page.screenshot({ path: path.resolve("../evidence/device-query-1024.png") })

  const overflow = await page.evaluate(() => {
    const over = (selector) => { const el = document.querySelector(selector); return el ? Math.max(0, el.scrollHeight - el.clientHeight) : 0 }
    return {
      stage: over(".console-stage"),
      page: over(".device-query-page"),
      main: over(".query-main"),
      doc: Math.max(0, (document.scrollingElement?.scrollHeight || 0) - (document.scrollingElement?.clientHeight || 0)),
      horizontal: Math.max(0, document.documentElement.scrollWidth - document.documentElement.clientWidth),
    }
  })
  if (errors.length) throw new Error(errors.join(" | "))
  if (overflow.stage || overflow.page || overflow.main || overflow.doc || overflow.horizontal) {
    throw new Error(`device query overflow at 1024: ${JSON.stringify(overflow)}`)
  }
  console.log("Device query screenshots written", JSON.stringify(overflow))
} finally {
  await browser.close()
}
