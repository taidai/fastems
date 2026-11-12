import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base=process.env.FASTIOT_URL||"http://127.0.0.1:62943"
const password=(await readFile(process.env.FASTIOT_PASSWORD_FILE||path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^审计日志$/}).click(); await page.waitForSelector(".audit-page"); await page.waitForTimeout(500)
 const rows=await page.locator(".audit-table tbody tr").count(); if(rows<1) throw new Error("no audit records")
 const text=await page.locator(".audit-page").innerText(); if(!text.includes("登录")) throw new Error("login audit missing")
 await page.screenshot({path:path.resolve("../evidence/audit-1669.png")})
 await page.locator(".audit-table tbody tr").first().click(); await page.waitForTimeout(250)
 await page.screenshot({path:path.resolve("../evidence/audit-detail-1669.png")})
 const downloadPromise=page.waitForEvent("download"); await page.getByRole("button",{name:"导出 CSV"}).click(); const download=await downloadPromise
 const csv=await readFile(await download.path(),"utf8"); if(!csv.includes("操作人")||!csv.includes("登录")) throw new Error("csv export missing audit rows")
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(350); await page.screenshot({path:path.resolve("../evidence/audit-1024.png")})
 const overflow=await page.evaluate(()=>{const over=s=>{const e=document.querySelector(s);return e?Math.max(0,e.scrollHeight-e.clientHeight):0};return {stage:over(".console-stage"),page:over(".audit-page"),doc:Math.max(0,(document.scrollingElement?.scrollHeight||0)-(document.scrollingElement?.clientHeight||0)),h:Math.max(0,document.documentElement.scrollWidth-document.documentElement.clientWidth)}})
 if(overflow.stage||overflow.page||overflow.doc||overflow.h) throw new Error("audit overflow: "+JSON.stringify(overflow))
 console.log("Audit UI verified rows="+rows+" overflow="+JSON.stringify(overflow))
}finally{await browser.close()}
