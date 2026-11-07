import { chromium } from "playwright-core"
import path from "node:path"
import { pathToFileURL } from "node:url"

const input = process.argv[2] || "docs/master-map.html"
let browser
try {
  browser = await chromium.launch({ channel: "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}
const errors = []
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
  page.on("pageerror", (error) => errors.push(error.message))
  page.on("requestfailed", (request) => errors.push(`asset failed: ${request.url()}`))
  await page.goto(pathToFileURL(path.resolve(input)).href)

  for (const shot of [
    { name: "master-map-desktop", width: 1440 },
    { name: "master-map-touch", width: 1280 },
    { name: "master-map-mobile", width: 390 },
  ]) {
    await page.setViewportSize({ width: shot.width, height: 900 })
    const width = await page.evaluate(() => document.documentElement.scrollWidth)
    if (width > shot.width) throw new Error(`horizontal overflow at ${shot.width}px: ${width}`)
    const height = await page.evaluate(() => document.documentElement.scrollHeight)
    await page.setViewportSize({ width: shot.width, height })
    await page.waitForTimeout(250)
    await page.screenshot({ path: path.resolve(`evidence/${shot.name}.png`) })
  }

  const background = await page.evaluate(() => getComputedStyle(document.querySelector(".hero")).backgroundImage)
  if (!background.includes("ems-site.jpg")) throw new Error("hero background image did not load")
  if (errors.length) throw new Error(errors.join(" | "))
  console.log("Master map rendered at 1440, 1280x800 baseline and 390; background loaded; no errors or overflow")
} finally {
  await browser.close()
}
