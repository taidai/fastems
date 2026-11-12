import { chromium } from "playwright-core"
import { mkdir, readFile, writeFile } from "node:fs/promises"
import path from "node:path"

// Feature check for the delivery P2 items: point-table diff, deploy wizard and
// evidence package. Requires the modbus simulator on 127.0.0.1:1502.

const base = process.env.FASTEMS_URL || "http://127.0.0.1:62943"
const dataDir = path.resolve(process.env.FASTEMS_DATA || "data/preview")
const evidence = path.resolve(process.env.FASTEMS_EVIDENCE || "evidence")
await mkdir(evidence, { recursive: true })
let password = process.env.FASTEMS_ADMIN_PASSWORD || ""
if (!password) {
  const initial = await readFile(path.join(dataDir, "initial-password.txt"), "utf8")
  password = initial.match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1] || ""
}
if (!password) throw new Error("No admin password available")

let browser
try {
  browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || "chrome", headless: true })
} catch {
  browser = await chromium.launch({ channel: "msedge", headless: true })
}
const page = await browser.newPage({ viewport: { width: 1600, height: 1000 } })
const consoleErrors = []
page.on("pageerror", (error) => consoleErrors.push(error.message))
page.on("console", (message) => { if (message.type() === "error") consoleErrors.push(message.text()) })

async function api(url, method = "GET", body, raw = false) {
  const token = await page.evaluate(() => sessionStorage.getItem("fastems-token"))
  const response = await page.request.fetch(`${base}/api${url}`, {
    method,
    headers: { Authorization: `Bearer ${token}`, ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
    data: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok()) throw new Error(`${method} ${url}: ${response.status()} ${await response.text()}`)
  return raw ? response.text() : response.json()
}

try {
  await page.goto(base)
  await page.getByLabel("账号", { exact: true }).fill("admin")
  await page.getByLabel("密码", { exact: true }).fill(password)
  await page.getByRole("button", { name: "登录", exact: true }).click()
  await page.waitForSelector(".console-stage")

  // A dedicated bus against the simulator, with one profile-named point.
  const snapshot = await api("/snapshot")
  const root = snapshot.roots[0]
  const bus = await api("/modbus", "POST", {
    parent: root, description: "向导总线", protocol: "TCP",
    uri: "127.0.0.1:1502", poll_period: 200, timeout: 500,
  })
  await api(`/modbus/${bus.id}/io`, "POST", {
    description: "交流输出总有功功率", server_id: 1, address: 10, io_type: "modbusHoldingRegister",
    data_format: "uint16", read_only: true, scale: 0.1, offset: 0, units: "kW",
  })
  const deadline = Date.now() + 10000
  for (;;) {
    const { channels } = await api("/ems/channels")
    if (channels.some((channel) => channel.id === bus.id && (channel.devices || []).length > 0)) break
    if (Date.now() > deadline) throw new Error("runtime channel did not appear")
    await page.waitForTimeout(200)
  }

  await page.getByLabel("页面目录").click()
  await page.locator(".console-directory button").filter({ hasText: /^部署与验收$/ }).click()
  await page.waitForSelector(".deploy-page")

  // --- Point-table diff: modify scale, add one point, confirm, then apply ---
  await page.locator(".point-table-tools select").selectOption(bus.id)
  await page.locator('.point-table-tools input[type="file"]').setInputFiles({
    name: "diff.csv", mimeType: "text/csv",
    buffer: Buffer.from([
      "名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位",
      "交流输出总有功功率,1,10,保持寄存器,uint16,是,0.2,0,kW",
      "向导新增,1,11,保持寄存器,uint16,是,1,0,",
    ].join(String.fromCharCode(10))),
  })
  await page.getByRole("button", { name: "查看差异" }).click()
  await page.locator(".csv-diff-counts .add").filter({ hasText: "新增 1" }).waitFor()
  await page.locator(".csv-diff-counts .change").filter({ hasText: "修改 1" }).waitFor()
  await page.locator(".csv-diff-row.change").filter({ hasText: "缩放 0.1 → 0.2" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "deploy-csv-diff-1600.png") })
  const beforeApply = await api(`/modbus/${bus.id}/io/export`, "GET", undefined, true)
  if (beforeApply.includes("向导新增")) throw new Error("diff preview wrote points")
  await page.getByRole("button", { name: "确认应用" }).click()
  await page.locator(".csv-result").filter({ hasText: "新增 1 · 修改 1 · 删除 0" }).waitFor()
  const afterApply = await api(`/modbus/${bus.id}/io/export`, "GET", undefined, true)
  if (!afterApply.includes("向导新增") || !afterApply.includes("0.2")) throw new Error("diff apply missing changes")

  // --- Deploy wizard: scan-or-pick, bind, map, read-only check, verifier ---
  await page.getByRole("button", { name: "接入向导" }).click()
  const wizard = page.locator('.deploy-wizard[aria-label="接入向导"]')
  await wizard.waitFor()
  await wizard.locator("select").first().selectOption(bus.id)
  await wizard.locator(".wizard-pick-row").filter({ hasText: "站号 1" }).first().click()
  await page.screenshot({ path: path.join(evidence, "deploy-wizard-step1-1600.png") })
  await wizard.getByRole("button", { name: "下一步", exact: true }).click()
  await wizard.getByText("绑定设备 Profile", { exact: false }).waitFor()
  await wizard.locator(".comm-form-grid select").selectOption("enjo-pcs")
  await wizard.locator(".wizard-profile-hint").filter({ hasText: "自动匹配 1 个" }).waitFor()
  await page.screenshot({ path: path.join(evidence, "deploy-wizard-step2-1600.png") })
  await wizard.getByRole("button", { name: "保存绑定，下一步" }).click()
  await wizard.locator(".unit-map-table").waitFor()
  const mappedState = await wizard.locator(".unit-map-table tbody tr").evaluateAll((rows) => {
    const row = rows.find((candidate) => candidate.querySelector("td strong")?.textContent?.trim() === "交流输出总有功功率")
    return row?.querySelector(".unit-map-state")?.textContent?.trim() || ""
  })
  if (!mappedState.includes("已映射")) throw new Error(`auto map failed: ${mappedState}`)
  await page.screenshot({ path: path.join(evidence, "deploy-wizard-step3-1600.png") })
  await wizard.getByRole("button", { name: "保存映射，下一步" }).click()
  await wizard.locator(".wizard-check-summary").waitFor()
  await wizard.locator(".unit-map-table td").filter({ hasText: "50.6" }).first().waitFor({ timeout: 10000 })
  await wizard.locator(".device-quality.good").first().waitFor()
  await page.screenshot({ path: path.join(evidence, "deploy-wizard-step4-1600.png") })
  await wizard.getByRole("button", { name: "核对完成，下一步" }).click()
  await wizard.getByPlaceholder("现场实施人员").fill("现场甲")
  await wizard.locator('input[type="date"]').fill("2026-11-07")
  await wizard.getByPlaceholder("铭牌序列号").fill("SN-向导-001")
  await wizard.locator('.comm-check input[type="checkbox"]').check()
  await wizard.getByRole("button", { name: "保存验证记录" }).click()
  await wizard.getByText("接入完成", { exact: true }).waitFor()
  await page.screenshot({ path: path.join(evidence, "deploy-wizard-done-1600.png") })
  await wizard.getByRole("button", { name: "完成" }).click()
  const verifications = await api("/profile-verifications")
  const record = (verifications.verifications || []).find((item) => item.channel_id === bus.id && item.server_id === 1)
  if (!record || !record.verified || record.verifier !== "现场甲" || record.device_serial !== "SN-向导-001") {
    throw new Error(`verification record missing: ${JSON.stringify(record)}`)
  }

  // --- Evidence package download ---
  const downloadEvent = page.waitForEvent("download")
  await page.getByRole("button", { name: "导出证据包" }).click()
  const download = await downloadEvent
  const file = await download.path()
  const payload = await readFile(file)
  if (!download.suggestedFilename().startsWith("fastems-evidence-") || payload.length < 500) {
    throw new Error(`evidence download invalid: ${download.suggestedFilename()} ${payload.length}B`)
  }
  if (payload.includes(Buffer.from(password))) throw new Error("evidence zip contains the admin password")

  const unexpected = consoleErrors.filter((message) => !message.includes("status of 409"))
  if (unexpected.length) throw new Error(`console errors: ${unexpected.join(" | ")}`)
  await writeFile(path.join(evidence, "deploy-wizard-check.json"), JSON.stringify({
    passed: true,
    checks: ["csv diff preview without writes", "diff apply add+update", "wizard scan/pick", "wizard bind with automap", "wizard mapping save", "wizard read-only check live value", "wizard verifier record", "evidence zip download without secrets"],
  }, null, 2) + "\n")
  console.log("Deploy wizard / diff / evidence check passed")
} catch (error) {
  await page.screenshot({ path: path.join(evidence, "deploy-wizard-failure.png"), fullPage: true }).catch(() => {})
  throw error
} finally {
  await browser.close()
}
