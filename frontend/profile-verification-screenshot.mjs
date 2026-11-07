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
 const row=page.locator(".communication-table tbody tr").filter({hasText:"验证记录测试"}).first(); await row.waitFor(); await row.getByRole("button",{name:"查看",exact:true}).click()
 const unit=page.locator(".communication-table-wrap.units tbody tr").filter({hasText:"站号 1"}).first(); await unit.waitFor(); await unit.getByRole("button",{name:"设备模型"}).click()
 await page.waitForSelector(".unit-model-modal"); await page.waitForTimeout(400)
 const verifier=await page.getByLabel("验证人").inputValue(); const serial=await page.getByLabel("设备序列号").inputValue(); if(verifier!=="张工"||serial!=="PCS-SN-0001") throw new Error("verification fields missing: "+verifier+" / "+serial)
 await page.screenshot({path:path.resolve("../evidence/profile-verification-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(300); await page.screenshot({path:path.resolve("../evidence/profile-verification-1024.png")})
 const downloadPromise=page.waitForEvent("download"); await page.getByRole("button",{name:"导出验证记录"}).click(); const download=await downloadPromise
 const csv=await readFile(await download.path(),"utf8"); if(!csv.includes("PCS-SN-0001")||!csv.includes("张工")) throw new Error("verification CSV missing record: "+csv.slice(0,300))
 console.log("Profile verification UI/exports verified")
}finally{await browser.close()}
