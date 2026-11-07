import { chromium } from "playwright-core"
import { readFile, mkdir } from "node:fs/promises"
import path from "node:path"
const base=process.env.EASYEMS_URL||"http://127.0.0.1:62943"
const out=process.env.REVIEW_OUT||"C:/Users/chent/AppData/Local/Temp/ui-review"
await mkdir(out,{recursive:true})
const password=(await readFile(process.env.EASYEMS_PASSWORD_FILE||path.resolve("../data/preview/initial-password.txt"),"utf8")).match(/(?:密码|password)\s*[:：]\s*(\S+)/i)?.[1]
const pages=["工作台","运行画面","设备台账","点位表","通道","虚拟聚合","决策表","功率分配器","报警与事件","数据与追溯","设备数据查询","部署与验收","账号与权限","审计日志","交付检查"]
let browser; try{browser=await chromium.launch({channel:"chrome",headless:true})}catch{browser=await chromium.launch({channel:"msedge",headless:true})}
const page=await browser.newPage({viewport:{width:1669,height:1000}})
try{
 await page.goto(base); await page.fill('input[autocomplete="username"]',"admin"); await page.fill('input[autocomplete="current-password"]',password); await page.click("button.login-button"); await page.waitForSelector(".console-stage")
 const results={}
 for(const size of [{width:1669,height:1000},{width:1024,height:768}]){
  await page.setViewportSize(size); await page.waitForTimeout(300)
  for(const label of pages){
   await page.getByLabel("页面目录").click()
   await page.locator(".console-directory button").filter({hasText:new RegExp(`^${label}$`)}).click()
   await page.waitForTimeout(500)
   const metrics=await page.evaluate(()=>{const q=s=>document.querySelector(s);const over=e=>e?Math.max(0,e.scrollHeight-e.clientHeight):0;return {stage:over(q(".console-stage")),doc:over(document.scrollingElement),horizontal:Math.max(0,document.documentElement.scrollWidth-document.documentElement.clientWidth)}})
   results[`${label}-${size.width}`]=metrics
   const slug=label.replace(/[^\w\u4e00-\u9fa5]+/g,"-")
   await page.screenshot({path:`${out}/${slug}-${size.width}.png`})
  }
 }
 console.log(JSON.stringify(results,null,1))
}finally{await browser.close()}
