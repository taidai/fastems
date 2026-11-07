import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base=process.env.EASYEMS_URL||"http://127.0.0.1:62943"
const password=(await readFile(process.env.EASYEMS_PASSWORD_FILE||path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^通道$/}).click(); await page.waitForSelector(".channel-workspace")
 await page.locator(".channel-workspace-switch button").filter({hasText:/^端口配置$/}).click()
 const portRow=page.locator(".communication-table tbody tr").filter({hasText:"命令验收"}).first(); await portRow.waitFor(); await portRow.getByRole("button",{name:"查看",exact:true}).click()
 const unitRow=page.locator(".communication-table-wrap.units tbody tr").filter({hasText:"站号 1"}).first(); await unitRow.waitFor(); await unitRow.getByRole("button",{name:"设备模型"}).click()
 await page.waitForSelector(".unit-model-modal")
 await page.waitForTimeout(500)
 console.log("MODEL TEXT\n"+(await page.locator(".unit-model-modal").innerText()).slice(0,2500))
 await page.screenshot({path:path.resolve("../evidence/unit-model-1669.png")})
 await page.locator(".unit-model-tabs button").filter({hasText:/^命令执行$/}).click(); await page.waitForTimeout(300)
 await page.screenshot({path:path.resolve("../evidence/unit-commands-1669.png")})
 page.once("dialog",d=>d.accept())
 await page.locator(".unit-command-card").first().getByRole("button",{name:"执行",exact:true}).click()
 await page.locator(".command-result.confirmed").first().waitFor({timeout:15000})
 await page.screenshot({path:path.resolve("../evidence/unit-commands-confirmed-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(350)
 await page.screenshot({path:path.resolve("../evidence/unit-commands-1024.png")})
 const box=await page.locator(".unit-model-modal").boundingBox(); const vp=page.viewportSize()
 if(!box||box.x<0||box.y<0||box.x+box.width>vp.width+1||box.y+box.height>vp.height+1) throw new Error("modal outside 1024 viewport: "+JSON.stringify(box))
 console.log("Unit model UI verified")
}finally{await browser.close()}
