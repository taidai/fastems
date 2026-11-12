// 导航分组与角色可见性检查：不依赖浏览器，直接验证规则本身。
// 运行：cd frontend && node navigation-check.mjs
// 规则见 docs/jobs.md：一级导航是交付任务，任务内是页签；隐藏页只由页面内链接进入。
import assert from "node:assert/strict"
import { navGroupsFor, pageDefs, visiblePagesFor } from "./src/navigation.ts"

const keys = (list) => list.map((def) => def.key)

// 注册表完整性：页面不重复，每个页面都有分组。
const allKeys = keys(pageDefs)
assert.equal(new Set(allKeys).size, allKeys.length, "页面注册表存在重复 key")
assert.ok(pageDefs.every((def) => Boolean(def.group)), "存在没有任务分组的页面")
assert.equal(keys(visiblePagesFor("customer", false, false))[0], "devices", "客户工作台应默认落在运行总览")
assert.equal(keys(visiblePagesFor("delivery", true, true))[0], "deploy", "交付工作台应默认落在交付验收")

// 查看者/操作员：只有客户工作台，且没有交付页面。
const customerPages = ["devices", "screen", "alarms", "history", "dispatchTrace", "health"]
for (const [role, admin, canConfigure] of [
  ["viewer", false, false],
  ["operator", false, false],
]) {
  assert.deepEqual(keys(visiblePagesFor("customer", admin, canConfigure)), customerPages, `${role} 的客户工作台页面不符`)
  assert.deepEqual(visiblePagesFor("delivery", admin, canConfigure), [], `${role} 不应看到交付工作台`)
  const groups = navGroupsFor("customer", admin, canConfigure)
  assert.deepEqual(groups.map((group) => group.key), ["run"], `${role} 的客户一级导航应为运行`)
  assert.deepEqual(keys(groups[0].pages), ["devices", "screen", "alarms"], `${role} 的运行页签不符`)
}

// 实施：两个工作台都能进；交付工作台没有账号与审计。
const implementerDelivery = keys(visiblePagesFor("delivery", false, true))
assert.deepEqual(
  implementerDelivery,
  ["deploy", "ledger", "channels", "points", "aggregate", "opcua", "energyUnits", "unitWiring", "resourceGroups", "rules", "allocators"],
  "实施角色的交付工作台页面不符",
)
assert.ok(!implementerDelivery.includes("users") && !implementerDelivery.includes("audit"), "实施角色不应看到账号与审计")

const implementerNav = navGroupsFor("delivery", false, true)
assert.deepEqual(implementerNav.map((group) => group.key), ["access", "assets", "strategy", "acceptance"], "交付一级导航应为四个任务")
assert.deepEqual(keys(implementerNav.find((group) => group.key === "access").pages), ["channels", "points", "aggregate", "opcua"], "接入页签不符")
assert.deepEqual(keys(implementerNav.find((group) => group.key === "assets").pages), ["energyUnits", "unitWiring", "resourceGroups"], "能源设备页签不符")
assert.deepEqual(keys(implementerNav.find((group) => group.key === "strategy").pages), ["rules", "allocators"], "策略页签不符")
assert.deepEqual(keys(implementerNav.find((group) => group.key === "acceptance").pages), ["deploy", "ledger"], "交付验收页签不符")

// 管理员：交付工作台多出系统任务。
const adminNav = navGroupsFor("delivery", true, true)
assert.deepEqual(adminNav.map((group) => group.key), ["access", "assets", "strategy", "acceptance", "system"], "管理员交付一级导航应多出系统")
assert.deepEqual(keys(adminNav.find((group) => group.key === "system").pages), ["users", "audit"], "系统页签不符")

// 隐藏页不得出现在任何一级导航或页签里。
const shownKeys = new Set(adminNav.flatMap((group) => keys(group.pages)))
for (const hidden of ["history", "dispatchTrace", "health"]) {
  assert.ok(!shownKeys.has(hidden), `隐藏页 ${hidden} 不应出现在导航`)
}

// 任何角色都不能从客户工作台看到配置类页面。
for (const def of visiblePagesFor("customer", true, true)) {
  assert.ok(!["channels", "points", "energyUnits", "rules", "deploy"].includes(def.key), `客户工作台混入了配置页 ${def.key}`)
}

console.log("navigation-check passed:", {
  pages: allKeys.length,
  customerGroups: navGroupsFor("customer", true, true).length,
  implementerGroups: implementerNav.length,
  adminGroups: adminNav.length,
})
