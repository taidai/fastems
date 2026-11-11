import { chromium } from "playwright-core"
import path from "node:path"
import { pathToFileURL } from "node:url"

const input = process.argv[2] || "../docs/acceptance-map.html"
const output = process.argv[3] || "../docs/assets/acceptance-map.png"

let browser
try {
  browser = await chromium.launch({ channel: "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}

try {
  const page = await browser.newPage({ viewport: { width: 2400, height: 1200 } })
  const errors = []
  page.on("pageerror", (e) => errors.push(e.message))
  await page.goto(pathToFileURL(path.resolve(input)).href)
  await page.waitForTimeout(300)

  const report = await page.evaluate(() => {
    const pageEl = document.querySelector(".page")
    const body = document.body
    const rect = pageEl.getBoundingClientRect()
    const width = Math.round(Math.max(rect.width, body.scrollWidth))
    const height = Math.round(Math.max(rect.height, body.scrollHeight))
    const clipped = []
    for (const el of document.querySelectorAll("body *")) {
      const r = el.getBoundingClientRect()
      if (r.width === 0 || r.height === 0) continue
      if (r.right > width + 0.5 || r.bottom > height + 0.5) {
        clipped.push({ tag: el.tagName, cls: el.className?.toString().slice(0, 40), right: Math.round(r.right), bottom: Math.round(r.bottom) })
      }
      if (el.scrollHeight > el.clientHeight + 2 && getComputedStyle(el).overflow !== "visible") {
        clipped.push({ tag: el.tagName, cls: el.className?.toString().slice(0, 40), overflowY: el.scrollHeight - el.clientHeight, text: (el.textContent || "").trim().slice(0, 30) })
      }
      if (el.scrollWidth > el.clientWidth + 2 && getComputedStyle(el).overflow !== "visible") {
        clipped.push({ tag: el.tagName, cls: el.className?.toString().slice(0, 40), overflowX: el.scrollWidth - el.clientWidth, text: (el.textContent || "").trim().slice(0, 30) })
      }
    }
    return { width, height, clipped: clipped.slice(0, 20) }
  })

  await page.setViewportSize({ width: report.width, height: report.height })
  await page.waitForTimeout(200)
  await page.screenshot({ path: path.resolve(output), fullPage: true })

  console.log(JSON.stringify({ ...report, errors }, null, 2))
} finally {
  await browser.close()
}
