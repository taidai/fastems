import { chromium } from "playwright-core"
import { readFile } from "node:fs/promises"
import path from "node:path"
const base="http://127.0.0.1:62943"; const password=(await readFile(path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/密码:\s*(\S+)/)[1]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1024,height:768}})
await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
const nav=async(label)=>{await page.getByLabel("页面目录").click();await page.locator(".console-directory button").filter({hasText:new RegExp(`^${label}$`)}).click();await page.waitForTimeout(400)}
await nav("工作台")
console.log("OVERVIEW",await page.evaluate(()=>{const r=s=>{const e=document.querySelector(s);if(!e)return null;const b=e.getBoundingClientRect(),c=getComputedStyle(e);return {w:Math.round(b.width),h:Math.round(b.height),font:c.fontSize}};return {kpi:r(".kpi-card"),value:r(".kpi-value"),label:r(".kpi-label"),head:r(".console-page-head h1")}}))
await nav("点位表")
console.log("POINTS",await page.evaluate(()=>{const w=document.querySelector(".point-table-wrap");const t=document.querySelector(".point-table");const th=document.querySelector(".point-table th");const b=th?.getBoundingClientRect();return {wrap:w?{cw:w.clientWidth,sw:w.scrollWidth}:null,table:t?{cw:t.clientWidth,sw:t.scrollWidth}:null,th:th?{font:getComputedStyle(th).fontSize,h:Math.round(b.height)}:null}}))
await nav("设备台账")
console.log("LEDGER",await page.evaluate(()=>{const s=document.querySelector(".ledger-summary");const cards=Array.from(document.querySelectorAll(".ledger-summary>div")).map(e=>{const b=e.getBoundingClientRect();return {w:Math.round(b.width),h:Math.round(b.height),text:e.textContent}});return {summary:s?{cw:s.clientWidth,sw:s.scrollWidth,ch:s.clientHeight,sh:s.scrollHeight}:null,cards}}))
await browser.close()
