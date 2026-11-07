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
 const card=page.locator(".channel-card").filter({hasText:"验收总线"}).first(); await card.waitFor(); await card.getByRole("button",{name:"报文",exact:true}).click()
 await page.waitForSelector(".frame-modal"); await page.waitForTimeout(2500)
 const rows=await page.locator(".frame-table tbody tr").count(); if(rows<1) throw new Error("no frames captured")
 console.log("frames",rows)
 await page.screenshot({path:path.resolve("../evidence/frames-1669.png")})
 await page.locator(".frame-segment button").filter({hasText:/^发$/}).click(); await page.waitForTimeout(200); await page.screenshot({path:path.resolve("../evidence/frames-tx-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(350); await page.screenshot({path:path.resolve("../evidence/frames-1024.png")})
 const box=await page.locator(".frame-modal").boundingBox(); const vp=page.viewportSize(); if(!box||box.x<0||box.y<0||box.x+box.width>vp.width+1||box.y+box.height>vp.height+1) throw new Error("frame modal outside viewport")
 console.log("Frame viewer verified")
}finally{await browser.close()}
