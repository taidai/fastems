// 一致性检查：导航是否按任务组织、页面是否都可渲染、token 是否只有一处。
// 运行：cd frontend && npm run check:consistency
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { groupDefs, pageDefs } from "./src/navigation.ts"

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8")

// 1. 设计 token 只能来自 tokens.css；其他样式文件不得再声明 :root 设计块。
const tokenFiles = ["src/styles.css", "src/reference.css", "src/tailwind.css", "src/deviceWorkspace.css"]
for (const file of tokenFiles) {
  assert.ok(!/^:root\s*\{/m.test(read(file)), `${file} 仍在声明 :root 设计 token，应移到 tokens.css`)
}
const tokens = read("src/tokens.css")
for (const name of ["--ink", "--muted", "--faint", "--line", "--accent", "--mint", "--blue", "--amber", "--danger"]) {
  assert.ok(tokens.includes(`${name}:`), `tokens.css 缺少 ${name}`)
}
const main = read("src/main.tsx")
assert.ok(main.indexOf('"./tokens.css"') >= 0 && main.indexOf('"./tokens.css"') < main.indexOf('"./styles.css"'), "tokens.css 必须先于 styles.css 引入")

// 2. 所有注册页面都必须在 App 中渲染，避免“导航有入口、页面不存在”。
const app = read("src/App.tsx")
for (const def of pageDefs) {
  assert.ok(app.includes(`page === "${def.key}"`), `App.tsx 缺少页面渲染：${def.key}`)
}

// 3. 导航必须是任务 + 页签；不再保留下拉菜单。
assert.ok(!/topnav-dropdown/.test(app), "App.tsx 仍在使用旧的下拉导航")
assert.ok(!/topnav-dropdown/.test(read("src/reference.css")), "reference.css 仍保留下拉导航样式")
assert.ok(app.includes("navGroupsFor"), "App.tsx 未使用任务导航 navGroupsFor")

// 4. 每个任务都在任务合同里有归属。
const jobs = read("../docs/jobs.md")
for (const group of groupDefs) {
  assert.ok(jobs.includes(group.label), `docs/jobs.md 缺少任务：${group.label}`)
}
assert.ok(jobs.includes("功能准入"), "docs/jobs.md 缺少功能准入规则")

// 5. 查证页可从页面内到达（隐藏不等于不可达）。
for (const hidden of ["history", "dispatchTrace", "health"]) {
  assert.ok(new RegExp(`(?:setPage|openPage)\\([^)]*"${hidden}"`).test(app), `隐藏页 ${hidden} 没有页面内入口`)
}

// 6. 报警数据只有 App 一处轮询；报警页与画面页接收同一份数据。
const alarmOwners = ["src/App.tsx", "src/AlarmsView.tsx", "src/ScreenView.tsx"].filter((file) => read(file).includes("fetchAlarms"))
assert.deepEqual(alarmOwners, ["src/App.tsx"], `报警轮询应收敛到 App，当前：${alarmOwners.join(", ")}`)

console.log("consistency-check passed:", {
  pages: pageDefs.length,
  groups: groupDefs.length,
  tokenSources: 1,
  alarmPollers: alarmOwners.length,
})
