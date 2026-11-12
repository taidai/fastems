import { chromium } from "playwright-core"
import { readFile, writeFile } from "node:fs/promises"
import path from "node:path"

const base = process.env.FASTEMS_URL || "http://127.0.0.1:62943"
const passwordFile = process.env.FASTEMS_PASSWORD_FILE || path.resolve("../data/preview/initial-password.txt")
const password = (await readFile(passwordFile, "utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || ""
if (!password) throw new Error("password file is empty")

let browser
try { browser = await chromium.launch({ channel: "chrome", headless: true }) }
catch { browser = await chromium.launch({ channel: "msedge", headless: true }) }
const errors = []
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, acceptDownloads: true })
  page.on("pageerror", (error) => errors.push(error.message))
  page.on("console", (message) => { if (message.type() === "error") errors.push(message.text()) })
  await page.goto(base)
  await page.fill('input[autocomplete="username"]', "admin")
  await page.fill('input[autocomplete="current-password"]', password)
  await page.click("button.login-button")
  await page.waitForSelector(".console-stage")
  await page.locator(".console-bottomnav button").filter({ hasText: "设备" }).click()
  await page.waitForSelector(".points-page")
  await page.waitForTimeout(700)
  const total = await page.locator(".point-table tbody tr").count()
  if (total === 0) throw new Error("point table is empty")
  const stats = await page.locator(".point-stats").innerText()
  if (!stats.includes("全部") || !stats.includes("正常")) throw new Error("point summary is incomplete")

  await page.getByLabel("搜索点位").fill("Modbus")
  await page.waitForTimeout(250)
  const filtered = await page.locator(".point-table tbody tr").count()
  if (filtered > total) throw new Error("search increased the row count")
  await page.getByLabel("搜索点位").fill("")
  await page.waitForTimeout(250)

  await page.locator(".point-table tbody tr").first().click()
  await page.waitForSelector(".point-drawer")
  const drawerTitle = await page.locator(".point-drawer h2").innerText()
  const drawerLive = await page.locator(".point-drawer-live").innerText()
  if (!drawerTitle || !drawerLive) throw new Error("point drawer is empty")
  await page.screenshot({ path: path.resolve("../evidence/points-detail.png") })
  await page.locator(".point-drawer-head .icon-button").click()

  await page.locator(".point-table tbody tr").nth(0).locator('input[type="checkbox"]').check()
  await page.locator(".point-table tbody tr").nth(1).locator('input[type="checkbox"]').check()
  const selectedText = await page.locator(".point-batchbar").innerText()
  if (!selectedText.includes("已选 2")) throw new Error(`batch selection missing: ${selectedText}`)
  await page.getByRole("button", { name: "批量改缩放/偏移" }).click()
  await page.waitForSelector(".point-batch-edit")
  if (!(await page.getByLabel("批量缩放").count()) || !(await page.getByLabel("批量偏移").count())) throw new Error("batch scale/offset fields are missing")
  await page.screenshot({ path: path.resolve("../evidence/points-batch-edit.png") })
  await page.locator(".point-batch-edit .secondary-button").click()
  await page.getByRole("button", { name: "+ 批量新增" }).click()
  await page.waitForSelector(".point-create")
  if (!(await page.getByLabel("名称前缀").count()) || !(await page.getByLabel("数量").count())) throw new Error("batch create fields are missing")
  await page.getByRole("button", { name: "取消" }).click()
  const downloadPromise = page.waitForEvent("download")
  await page.getByRole("button", { name: "导出 CSV" }).click()
  const download = await downloadPromise

  await page.setViewportSize({ width: 1024, height: 768 })
  await page.waitForTimeout(400)
  await page.screenshot({ path: path.resolve("../evidence/points-1024.png") })
  const metrics = await page.evaluate(() => {
    const ic = document.querySelector("ion-content")
    const inner = ic?.shadowRoot?.querySelector(".inner-scroll")
    const stage = document.querySelector(".console-stage")
    const wrap = document.querySelector(".point-table-wrap")
    const table = document.querySelector(".point-table")
    return {
      inner: (inner?.scrollHeight || 0) - (inner?.clientHeight || 0),
      stage: (stage?.scrollHeight || 0) - (stage?.clientHeight || 0),
      table: (wrap?.scrollHeight || 0) - (wrap?.clientHeight || 0),
      tableWidth: (table?.scrollWidth || 0) - (table?.clientWidth || 0),
    }
  })
  if (metrics.inner > 0 || metrics.stage > 0 || metrics.tableWidth > 0) throw new Error(`layout overflow: ${JSON.stringify(metrics)}`)

  const report = {
    passed: errors.length === 0,
    url: base,
    rows: total,
    filteredRows: filtered,
    drawerTitle,
    selected: 2,
    batchScaleEdit: true,
    batchCreateForm: true,
    download: download.suggestedFilename(),
    metrics,
    errors,
    note: "只读工作台自检；批量停用/启用/删除与写值回读另行实测。",
  }
  await writeFile(path.resolve("../evidence/points-workbench.json"), JSON.stringify(report, null, 2) + "\n")
  if (!report.passed) throw new Error(`console errors: ${errors.join(" | ")}`)
  console.log("Points workbench smoke passed", JSON.stringify({ rows: total, filtered, drawerTitle, download: report.download, metrics }))
} finally {
  await browser.close()
}
