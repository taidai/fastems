import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base=process.env.EASYEMS_URL||"http://127.0.0.1:62943"
const password=(await readFile(process.env.EASYEMS_PASSWORD_FILE||path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^决策表$/}).click(); await page.waitForSelector(".rules-page"); await page.waitForTimeout(1200)
 const ruleTab=page.getByRole("button",{name:"舒适控制 P0",exact:true}); if(await ruleTab.count()){ await ruleTab.click(); await page.waitForTimeout(800) }
 const detail=page.getByRole("button",{name:"详细视图"}); if(await detail.count()) { await detail.click(); await page.waitForTimeout(300) }
 await page.getByRole("button",{name:"+ 输出"}).first().click(); await page.waitForTimeout(200)
 const selects=page.locator('select[aria-label="输出点位"]'); const select=selects.last()
 console.log("OPTIONS", await select.locator("option").allInnerTexts())
 console.log("SNAP", await page.evaluate(async()=>{const t=sessionStorage.getItem("easyems-token");const r=await fetch("/api/snapshot",{headers:{Authorization:"Bearer "+t}});const d=await r.json();return d.nodes.filter(n=>n.type==="group").map(n=>({id:n.id,desc:(n.points.find(p=>p.type==="description")||{}).text,alloc:(n.points.find(p=>p.type==="easyemsAllocator")||{}).text}))}))
 await select.selectOption({label:"分配器验收组（分配器）"})
 await page.waitForTimeout(200)
 await page.screenshot({path:path.resolve("../evidence/allocator-rule-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(300); await page.screenshot({path:path.resolve("../evidence/allocator-rule-1024.png")})
 console.log("Allocator rule target verified")
}finally{await browser.close()}
