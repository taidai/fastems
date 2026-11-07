import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base="http://127.0.0.1:62943"; const password=(await readFile(path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/密码:\s*(\S+)/)[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^报警与事件$/}).click(); await page.waitForSelector(".alarm-page"); await page.waitForTimeout(700)
 const card=page.locator(".alarm-card").filter({hasText:"维护测试"}).first(); await card.waitFor(); await page.screenshot({path:path.resolve("../evidence/alarm-maintenance-1669.png")})
 await card.getByRole("button",{name:"维护",exact:true}).click(); await page.waitForSelector('.alarm-maintenance-modal'); await page.waitForTimeout(200)
 await page.getByLabel("维护原因").fill("现场检修")
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(250); await page.screenshot({path:path.resolve("../evidence/alarm-maintenance-modal-1024.png")})
 await page.getByRole("button",{name:"开始维护"}).click(); await page.waitForTimeout(900)
 await page.setViewportSize({width:1669,height:1000}); await page.waitForTimeout(300); await page.screenshot({path:path.resolve("../evidence/alarm-maintenance-active-1669.png")})
 const text=await page.locator(".alarm-card").filter({hasText:"维护测试"}).first().innerText(); if(!text.includes("维护中")) throw new Error("maintenance badge missing: "+text)
 console.log("Alarm maintenance UI verified")
}finally{await browser.close()}
