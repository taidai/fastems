import { readFile } from "node:fs/promises"
import path from "node:path"

export function p0Steps({ page, snapshot, targetValue, waitTarget, addNode, evidence, go }) {
  async function openManager(name) {
    const line = page.locator(".device-line").filter({ has: page.locator(".device-name strong").getByText(name, { exact: true }) })
    await line.getByRole("button", { name: "管理", exact: true }).click()
    const drawer = page.locator(".node-drawer")
    await drawer.waitFor()
    return drawer
  }
  async function confirmMove(drawer) {
    const button = drawer.getByRole("button", { name: "移动位置" })
    if (await button.isDisabled()) throw new Error("Move button remained disabled after selecting a destination")
    const responsePromise = page.waitForResponse((response) => response.url().endsWith("/move"))
    await button.click()
    const response = await responsePromise
    if (!response.ok()) throw new Error(`Move failed: ${response.status()} ${await response.text()}`)
  }
  function nodeByName(data, name) {
    return data.nodes.find((node) => node.points.some((p) => p.type === "description" && p.text === name))
  }
  function currentValue(data, name) {
    const value = nodeByName(data, name)?.points.find((p) => p.type === "value")
    return value ? (value.value ?? 0) : undefined
  }
  async function waitBus(name) {
    const deadline = Date.now() + 10000
    while (Date.now() < deadline) {
      const node = nodeByName(await snapshot(), name)
      if (node?.points.some((point) => point.type === "lastGoodPoll" && point.time)) return
      await page.waitForTimeout(100)
    }
    throw new Error(`Bus ${name} did not complete a successful poll`)
  }
  async function waitHealthy(name) {
    const deadline = Date.now() + 10000
    while (Date.now() < deadline) {
      const node = nodeByName(await snapshot(), name)
      const health = node?.points.find((point) => point.type === "health")?.text
      if (health === "healthy") return
      await page.waitForTimeout(100)
    }
    throw new Error(`Point ${name} did not become healthy`)
  }
  async function waitCurrent(name, value) {
    const deadline = Date.now() + 10000
    while (Date.now() < deadline) {
      if (currentValue(await snapshot(), name) === value) return
      await page.waitForTimeout(100)
    }
    throw new Error(`Current ${name} did not become ${value}`)
  }

  async function rules() {
    await go("决策表")
    await page.getByRole("button", { name: "重命名", exact: true }).click()
    await page.getByLabel("规则名称").fill("舒适控制 P0")
    await page.getByRole("button", { name: "保存名称" }).click()
    await page.getByRole("heading", { name: "舒适控制 P0", exact: true }).waitFor()
    await page.getByRole("button", { name: "复制", exact: true }).click()
    await page.getByRole("heading", { name: "舒适控制 P0 副本", exact: true }).waitFor()
    const data = await snapshot()
    const copy = nodeByName(data, "舒适控制 P0 副本")
    if (!copy || copy.points.some((p) => p.type === "applied" && p.text)) throw new Error("Copied rule was published")
    await page.locator(".rule-management").getByRole("button", { name: "删除", exact: true }).click()
    await page.getByRole("button", { name: "确认删除" }).click()
    await page.getByRole("heading", { name: "舒适控制 P0", exact: true }).waitFor()
    await page.getByLabel("输出值").first().fill("0")
    await page.getByRole("button", { name: "发布当前规则" }).click()
    await page.getByRole("button", { name: "确认发布" }).click()
    await waitTarget("风机", 0)
    await page.getByRole("button", { name: "版本与历史", exact: true }).click()
    await page.getByRole("button", { name: "恢复此版本" }).first().waitFor()
    await page.screenshot({ path: path.join(evidence, "p0-rule-history.png"), fullPage: true })
    await page.getByRole("button", { name: "恢复此版本" }).first().click()
    await waitTarget("风机", 1)
    await page.getByRole("button", { name: "版本与历史", exact: true }).click()
  }

  async function resources() {
    await go("运行总览")
    let drawer = await openManager("风机")
    await drawer.getByLabel("名称", { exact: true }).fill("风机 P0")
    await drawer.getByRole("button", { name: "保存修改" }).click()
    await drawer.waitFor({ state: "detached" })
    drawer = await openManager("风机 P0")
    await drawer.getByRole("checkbox").check()
    await drawer.getByRole("button", { name: "保存修改" }).click()
    await drawer.waitFor({ state: "detached" })
    const disabledLine = page.locator(".device-line").filter({ hasText: "风机 P0" })
    await disabledLine.getByText("已停用", { exact: true }).waitFor()
    if (await disabledLine.getByRole("button", { name: "下发目标值" }).count()) throw new Error("Disabled point still accepts controls")
    drawer = await openManager("风机 P0")
    await drawer.getByRole("checkbox").uncheck()
    await drawer.getByRole("button", { name: "保存修改" }).click()
    await drawer.waitFor({ state: "detached" })
    drawer = await openManager("风机 P0")
    await drawer.getByLabel("名称", { exact: true }).fill("风机")
    await drawer.getByRole("button", { name: "保存修改" }).click()
    await drawer.waitFor({ state: "detached" })
    await page.screenshot({ path: path.join(evidence, "p0-resource-management.png"), fullPage: true })

    drawer = await openManager("风机")
    page.once("dialog", (dialog) => dialog.accept())
    const refusal = page.waitForResponse((response) => response.request().method() === "DELETE" && response.url().includes("/api/nodes/"))
    await drawer.locator(".danger-zone").getByRole("button", { name: "删除", exact: true }).click()
    const refused = await refusal
    if (refused.status() !== 409) throw new Error(`Referenced node delete was not refused: ${refused.status()}`)
    await drawer.getByRole("button", { name: "关闭管理" }).click()
    const closeError = page.locator(".global-error").getByRole("button", { name: "关闭", exact: true })
    await closeError.click({ timeout: 1000 }).catch(() => {})
    await addNode("variable", "临时点位", 0, "演示电站")
    drawer = await openManager("临时点位")
    page.once("dialog", (dialog) => dialog.accept())
    await drawer.locator(".danger-zone").getByRole("button", { name: "删除", exact: true }).click()
    await drawer.waitFor({ state: "detached" })
    await page.locator(".device-list").getByText("临时点位", { exact: true }).waitFor({ state: "detached" })

    await addNode("group", "备用区域", 0, "根节点")
    drawer = await openManager("演示电站")
    await drawer.locator(".move-section select").selectOption({ label: "备用区域" })
    await confirmMove(drawer)
    await drawer.waitFor({ state: "detached" })
    let data = await snapshot()
    const backup = nodeByName(data, "备用区域")
    if (nodeByName(data, "演示电站")?.parent !== backup.id) throw new Error("Group was not moved")
    drawer = await openManager("演示电站")
    await drawer.locator(".move-section select").selectOption({ label: "根节点" })
    await confirmMove(drawer)
    await drawer.waitFor({ state: "detached" })
    data = await snapshot()
    if (nodeByName(data, "演示电站")?.parent !== data.roots[0]) throw new Error("Group was not moved back")
  }

  async function modbus() {
    await go("通道")
    await page.waitForSelector(".channel-workspace")
    await page.getByRole("tab", { name: "端口配置", exact: true }).click()

    await page.locator(".communication-actions button").click()
    let modal = page.locator('.comm-modal[aria-label="端口配置"]')
    await modal.getByLabel("端口名称").fill("验收总线")
    await modal.getByLabel("目标地址").fill("127.0.0.1:1502")
    await modal.getByLabel("站号", { exact: true }).fill("1")
    await modal.getByLabel("地址", { exact: true }).fill("10")
    await modal.getByRole("button", { name: "测试只读连接" }).click()
    await modal.locator(".success-message").getByText(/253/).waitFor()
    await modal.getByRole("button", { name: "保存端口" }).click()
    await page.locator(".communication-table").getByText("验收总线", { exact: true }).waitFor()

    async function addPoint(name, address, type) {
      await page.locator(".communication-units-head button").click()
      modal = page.locator('.comm-modal[aria-label="新增单元点位"]')
      await modal.getByLabel("点位名称").fill(name)
      await modal.getByLabel("站号", { exact: true }).fill("1")
      await modal.getByLabel("地址", { exact: true }).fill(String(address))
      if (type) await modal.getByLabel("类型").selectOption(type)
      if (name === "Modbus 可写") await modal.getByRole("checkbox").uncheck()
      if (name === "Modbus 温度") {
        await modal.getByLabel("缩放").fill("0.1")
        await modal.getByLabel("单位").fill("°C")
      }
      await modal.getByRole("button", { name: "保存点位" }).click()
      await modal.waitFor({ state: "detached" })
    }
    await addPoint("Modbus 温度", 10)
    await waitCurrent("Modbus 温度", 25.3)
    await addPoint("Modbus 可写", 0)
    await waitHealthy("Modbus 可写")

    await go("运行总览")
    const writeLine = page.locator(".device-line").filter({ hasText: "Modbus 可写" })
    await writeLine.waitFor()
    await writeLine.getByRole("spinbutton").fill("0")
    await writeLine.getByRole("button", { name: "下发目标值" }).click()
    await page.getByRole("button", { name: "确认下发" }).click()
    await writeLine.getByText("已回读", { exact: true }).waitFor()

    let drawer = await openManager("Modbus 温度")
    await drawer.getByLabel("缩放", { exact: true }).fill("0.2")
    await drawer.getByRole("button", { name: "保存修改" }).click()
    await drawer.waitFor({ state: "detached" })
    await waitCurrent("Modbus 温度", 50.6)
    drawer = await openManager("验收总线")
    await drawer.getByLabel("轮询（毫秒）", { exact: true }).fill("100")
    await drawer.getByRole("button", { name: "保存修改" }).click()
    await drawer.waitFor({ state: "detached" })

    await go("通道")
    await page.getByRole("tab", { name: "点表与模板", exact: true }).click()

    await page.locator('.csv-import input[type="file"]').setInputFiles({
      name: "points.csv", mimeType: "text/csv",
      buffer: Buffer.from(["名称,站号,地址,类型,数据格式,只读,缩放,偏移,单位", "CSV 温度,1,10,输入寄存器,uint16,是,0.1,0,°C"].join(String.fromCharCode(10))),
    })
    await page.getByRole("button", { name: "预检点表" }).click()
    await page.locator(".csv-import").getByText(/预检通过：1 个点位/).waitFor()
    await page.getByRole("button", { name: "确认导入" }).click()
    await page.locator(".csv-import").getByText("已导入 1 个点位").waitFor()
    await waitCurrent("CSV 温度", 25.3)
    const downloadEvent = page.waitForEvent("download")
    await page.getByRole("button", { name: "导出当前点表" }).click()
    const download = await downloadEvent
    const exported = await readFile(await download.path(), "utf8")
    if (!exported.includes("Modbus 温度") || !exported.includes("CSV 温度")) throw new Error("CSV export missing points")

    await page.getByLabel("模板名称", { exact: true }).fill("标准温控设备")
    await page.getByRole("button", { name: "保存为型号模板" }).click()
    await page.getByLabel("型号模板").waitFor()

    await page.getByRole("tab", { name: "端口配置", exact: true }).click()
    await page.locator(".communication-actions button").click()
    modal = page.locator('.comm-modal[aria-label="端口配置"]')
    await modal.getByLabel("端口名称").fill("模板总线")
    await modal.getByLabel("目标地址").fill("127.0.0.1:1502")
    await modal.getByRole("button", { name: "保存端口" }).click()
    await page.locator(".communication-table").getByText("模板总线", { exact: true }).waitFor()

    await page.getByRole("tab", { name: "点表与模板", exact: true }).click()
    await page.getByLabel("应用到连接").selectOption({ label: "模板总线" })
    await page.getByRole("button", { name: "应用模板", exact: true }).click()
    const deadline = Date.now() + 10000
    let applied = false
    while (Date.now() < deadline) {
      const data = await snapshot()
      const targetBus = nodeByName(data, "模板总线")
      if (data.nodes.filter((node) => node.parent === targetBus?.id).length === 3) {
        applied = true
        break
      }
      await page.waitForTimeout(100)
    }
    if (!applied) throw new Error("Template did not create all points")
    await waitBus("模板总线")
    await page.screenshot({ path: path.join(evidence, "p0-modbus-management.png"), fullPage: true })
  }

  async function delivery() {
    await go("部署与验收")
    await page.getByRole("button", { name: "交付检查", exact: true }).click()
    await page.getByRole("heading", { name: "交付检查", exact: true, level: 2 }).waitFor()
    await page.locator(".delivery-item").filter({ hasText: "规则发布与运行" }).waitFor()
    await page.locator(".delivery-item").filter({ hasText: "Modbus 连接" }).waitFor()
    await page.screenshot({ path: path.join(evidence, "p0-delivery-check.png"), fullPage: true })
  }

  return { rules, resources, modbus, delivery }
}
