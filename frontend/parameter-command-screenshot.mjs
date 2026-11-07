import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base="http://127.0.0.1:62943"; const password=(await readFile(path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/密码:\s*(\S+)/)[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^通道$/}).click(); await page.waitForSelector(".channel-workspace")
 await page.locator(".channel-workspace-switch button").filter({hasText:/^端口配置$/}).click()
 const row=page.locator(".communication-table tbody tr").filter({hasText:"参数命令测试"}).first(); await row.waitFor(); await row.getByRole("button",{name:"查看",exact:true}).click()
 const unit=page.locator(".communication-table-wrap.units tbody tr").filter({hasText:"站号 1"}).first(); await unit.waitFor(); await unit.getByRole("button",{name:"设备模型"}).click()
 await page.waitForSelector(".unit-model-modal"); await page.locator(".unit-model-tabs button").filter({hasText:/^命令执行$/}).click(); await page.waitForTimeout(300)
 const input=page.getByLabel("目标百分比"); await input.fill("42"); await page.screenshot({path:path.resolve("../evidence/parameter-command-1669.png")})
 page.once("dialog",d=>d.accept()); await page.getByRole("button",{name:"执行",exact:true}).click(); await page.locator(".command-result.confirmed").waitFor({timeout:15000})
 await page.screenshot({path:path.resolve("../evidence/parameter-command-confirmed-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(300); await page.screenshot({path:path.resolve("../evidence/parameter-command-1024.png")})
 console.log("Parameterized command UI verified")
}finally{await browser.close()}
