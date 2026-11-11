// 导航分组与角色可见性检查：不依赖浏览器，直接验证规则本身。
// 运行：cd frontend && node navigation-check.mjs
import assert from "node:assert/strict"
import { pageDefs, visiblePagesFor } from "./src/navigation.ts"

const keys = (list) => list.map((def) => def.key)

// 注册表完整性：页面不重复，且每个工作台都有自己的入口页。
const allKeys = keys(pageDefs)
assert.equal(new Set(allKeys).size, allKeys.length, "页面注册表存在重复 key")
assert.equal(keys(visiblePagesFor("customer", false, false))[0], "devices", "客户工作台应默认落在运行总览")
assert.equal(keys(visiblePagesFor("delivery", true, true))[0], "deploy", "交付工作台应默认落在交付工作台页")

// 查看者/操作员：只有客户工作台，且没有交付页面。
const customerForViewer = ["devices", "screen", "alarms", "dispatchTrace", "history", "health", "ledger"]
for (const [role, admin, canConfigure] of [
  ["viewer", false, false],
  ["operator", false, false],
]) {
  assert.deepEqual(keys(visiblePagesFor("customer", admin, canConfigure)), customerForViewer, `${role} 的客户工作台页面不符`)
  assert.deepEqual(visiblePagesFor("delivery", admin, canConfigure), [], `${role} 不应看到交付工作台`)
}

// 实施：两个工作台都能进；交付工作台没有账号与审计。
const implementerDelivery = keys(visiblePagesFor("delivery", false, true))
assert.deepEqual(
  implementerDelivery,
  ["deploy", "ledger", "channels", "points", "aggregate", "unitWiring", "opcua", "energyUnits", "resourceGroups", "allocators", "rules"],
  "实施角色的交付工作台页面不符",
)
assert.ok(!implementerDelivery.includes("users") && !implementerDelivery.includes("audit"), "实施角色不应看到账号与审计")
assert.deepEqual(keys(visiblePagesFor("customer", false, true)), customerForViewer, "实施角色的客户工作台页面不符")

// 管理员：交付工作台包含账号与审计。
const adminDelivery = keys(visiblePagesFor("delivery", true, true))
assert.deepEqual(adminDelivery, [...implementerDelivery, "users", "audit"], "管理员交付工作台应多出账号与审计")
assert.deepEqual(keys(visiblePagesFor("customer", true, true)), customerForViewer, "管理员的客户工作台页面不符")

// 任何角色都不能从客户工作台看到配置类页面。
for (const def of visiblePagesFor("customer", true, true)) {
  assert.ok(!["channels", "points", "energyUnits", "rules", "deploy"].includes(def.key), `客户工作台混入了配置页 ${def.key}`)
}

console.log("navigation-check passed:", {
  pages: allKeys.length,
  customer: customerForViewer.length,
  implementerDelivery: implementerDelivery.length,
  adminDelivery: adminDelivery.length,
})
