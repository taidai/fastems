import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base="http://127.0.0.1:62943"; const password=(await readFile(path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/密码:\s*(\S+)/)[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^设备台账$/}).click(); await page.waitForSelector(".device-ledger-page"); await page.waitForTimeout(800)
 const rows=await page.locator(".ledger-table tbody tr").count(); if(rows<1) throw new Error("ledger empty")
 console.log("rows",rows,"summary",(await page.locator(".ledger-summary").innerText()).replace(/\n/g," "))
 await page.screenshot({path:path.resolve("../evidence/device-ledger-1669.png")})
 await page.locator(".ledger-table tbody tr").first().getByRole("button",{name:"详情"}).click(); await page.waitForSelector(".device-ledger-modal"); await page.waitForTimeout(250); await page.screenshot({path:path.resolve("../evidence/device-ledger-detail-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(300); await page.screenshot({path:path.resolve("../evidence/device-ledger-1024.png")})
 const overflow=await page.evaluate(()=>{const over=s=>{const e=document.querySelector(s);return e?Math.max(0,e.scrollHeight-e.clientHeight):0};return {stage:over(".console-stage"),page:over(".device-ledger-page"),doc:Math.max(0,(document.scrollingElement?.scrollHeight||0)-(document.scrollingElement?.clientHeight||0)),h:Math.max(0,document.documentElement.scrollWidth-document.documentElement.clientWidth)}})
 if(overflow.stage||overflow.page||overflow.doc||overflow.h) throw new Error("ledger overflow: "+JSON.stringify(overflow))
 console.log("ledger verified",overflow)
}finally{await browser.close()}
