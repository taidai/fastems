import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base="http://127.0.0.1:62943"; const password=(await readFile(path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/密码:\s*(\S+)/)[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
const errors=[]; page.on("pageerror",e=>errors.push(e.message)); page.on("console",m=>{if(m.type()==="error")errors.push(m.text())})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 await page.getByLabel("页面目录").click(); await page.locator(".console-directory button").filter({hasText:/^报警与事件$/}).click(); await page.waitForSelector(".alarm-page")
 await page.locator(".alarm-view-tabs button").filter({hasText:/^统计与提醒$/}).click(); await page.waitForSelector(".alarm-stats-page"); await page.waitForTimeout(500)
 const stats=await page.locator(".alarm-stats-page").innerText(); if(!stats.includes("今日告警")||!stats.includes("告警趋势")) throw new Error("stats missing")
 await page.screenshot({path:path.resolve("../evidence/alarm-stats-1669.png")})
 await page.getByRole("button",{name:"试听",exact:true}).click(); await page.waitForTimeout(300)
 await page.getByLabel("静默时段").check(); await page.waitForTimeout(150)
 await page.screenshot({path:path.resolve("../evidence/alarm-sound-1669.png")})
 await page.setViewportSize({width:1024,height:768}); await page.waitForTimeout(350); await page.screenshot({path:path.resolve("../evidence/alarm-stats-1024.png")})
 const overflow=await page.evaluate(()=>{const over=s=>{const e=document.querySelector(s);return e?Math.max(0,e.scrollHeight-e.clientHeight):0};return {stage:over(".console-stage"),page:over(".alarm-stats-page"),doc:Math.max(0,(document.scrollingElement?.scrollHeight||0)-(document.scrollingElement?.clientHeight||0)),h:Math.max(0,document.documentElement.scrollWidth-document.documentElement.clientWidth)}})
 if(overflow.stage||overflow.page||overflow.doc||overflow.h) throw new Error("stats overflow: "+JSON.stringify(overflow))
 if(errors.length) throw new Error(errors.join(" | "))
 console.log("Alarm stats/sound UI verified", overflow)
}finally{await browser.close()}
