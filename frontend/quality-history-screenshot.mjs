import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base=process.env.FASTEMS_URL||"http://127.0.0.1:62943"
const password=(await readFile(process.env.FASTEMS_PASSWORD_FILE||path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^通道$/}).click(); await page.waitForSelector(".channel-workspace"); await page.waitForTimeout(2500)
 const quality=await page.locator(".channel-card").filter({hasText:"模板总线"}).locator(".channel-quality").innerText(); if(!quality.includes("成功率")) throw new Error("quality line missing: "+quality)
 await page.screenshot({path:path.resolve("../evidence/quality-1669.png")})
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^数据与追溯$/}).click(); await page.waitForSelector(".history-page"); await page.waitForTimeout(500)
 const limit=page.getByLabel("历史保留条数"); if(!(await limit.count())) throw new Error("history limit missing")
 const before=await limit.inputValue(); await limit.fill("26000"); await page.getByRole("button",{name:"保存",exact:true}).click(); await page.waitForTimeout(500)
 const notice=await page.locator(".history-limit small").innerText(); if(!notice.includes("已保存")) throw new Error("history save failed")
 await page.screenshot({path:path.resolve("../evidence/history-limit-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(350); await page.screenshot({path:path.resolve("../evidence/history-limit-1024.png")})
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^通道$/}).click(); await page.waitForTimeout(2200); await page.screenshot({path:path.resolve("../evidence/quality-1024.png")})
 console.log("Quality/history verified; before="+before+" quality="+quality.replace(/\n/g," "))
}finally{await browser.close()}
