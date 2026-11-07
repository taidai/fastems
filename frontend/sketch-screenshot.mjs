import { chromium } from "playwright-core"
import path from "node:path"
import { pathToFileURL } from "node:url"

const input = process.argv[2] || "docs/ui-sketch.html"
const prefix = process.argv[3] || "sketch"
let browser
try {
  browser = await chromium.launch({ channel: "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
const errors = []
page.on("pageerror", (error) => errors.push(error.message))
try {
  await page.goto(pathToFileURL(path.resolve(input)).href)
  for (const name of ["overview", "devices", "rules", "data"]) {
    if (name !== "overview") await page.locator(`.nav button[data-page="${name}"]`).click()
    await page.locator(`#${name}`).waitFor({ state: "visible" })
    await page.screenshot({ path: path.resolve(`evidence/${prefix}-${name}.png`), fullPage: true })
  }
  await page.locator('.nav button[data-page="overview"]').click()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.screenshot({ path: path.resolve(`evidence/${prefix}-mobile.png`), fullPage: true })
  if (errors.length) throw new Error(errors.join(" | "))
  console.log("UI sketch rendered: 4 desktop pages and mobile; no browser errors")
} finally {
  await browser.close()
}
