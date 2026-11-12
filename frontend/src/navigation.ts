export type Page = "devices" | "screen" | "dispatchTrace" | "ledger" | "points" | "channels" | "aggregate" | "deploy" | "allocators" | "energyUnits" | "unitWiring" | "resourceGroups" | "alarms" | "history" | "health" | "rules" | "users" | "audit" | "delivery" | "opcua"

export type Workbench = "customer" | "delivery"

export type PageDef = {
  key: Page
  label: string
  workbench: Workbench
  adminOnly?: boolean
  configOnly?: boolean
  // 交付页同时以只读形式出现在客户工作台（如设备台账：客户需要看设备健康，但不能改绑定）。
  customerReadOnly?: boolean
}

// 页面归属：workbench 决定"这是谁的任务"，adminOnly/configOnly 决定"哪个角色能看见"。
// 与《导航总览》一致：客户工作台只有运行类页面；接入、建模、策略、发布都在交付工作台。
// 隐藏只减少误操作；能不能读改仍由后端按节点范围判定。
export const pageDefs: PageDef[] = [
  { key: "devices", label: "运行总览", workbench: "customer" },
  { key: "screen", label: "运行画面", workbench: "customer" },
  { key: "alarms", label: "报警与事件", workbench: "customer" },
  { key: "dispatchTrace", label: "调度追溯", workbench: "customer" },
  { key: "history", label: "历史与追溯", workbench: "customer" },
  { key: "health", label: "通信与健康", workbench: "customer" },
  { key: "deploy", label: "交付工作台", workbench: "delivery", configOnly: true },
  { key: "ledger", label: "设备台账", workbench: "delivery", customerReadOnly: true },
  { key: "channels", label: "通道", workbench: "delivery" },
  { key: "points", label: "点位表", workbench: "delivery" },
  { key: "aggregate", label: "虚拟聚合", workbench: "delivery" },
  { key: "unitWiring", label: "能源设备接线", workbench: "delivery", configOnly: true },
  { key: "opcua", label: "OPC-UA", workbench: "delivery", configOnly: true },
  { key: "energyUnits", label: "能源设备与策略", workbench: "delivery", configOnly: true },
  { key: "resourceGroups", label: "资源集群", workbench: "delivery", configOnly: true },
  { key: "allocators", label: "功率分配器", workbench: "delivery", configOnly: true },
  { key: "rules", label: "决策表", workbench: "delivery", configOnly: true },
  { key: "users", label: "账号与权限", workbench: "delivery", adminOnly: true },
  { key: "audit", label: "审计日志", workbench: "delivery", adminOnly: true },
]

export const workbenchNames: Record<Workbench, string> = { customer: "客户", delivery: "交付" }
export const workbenchFullNames: Record<Workbench, string> = { customer: "客户工作台", delivery: "交付工作台" }

export function visiblePagesFor(workbench: Workbench, isAdmin: boolean, canConfigure: boolean): PageDef[] {
  // 交付工作台整体要求可配置角色；客户工作台对所有能登录的角色开放。
  if (workbench === "delivery" && !canConfigure) return []
  return pageDefs.filter((def) => {
    const inWorkbench = def.workbench === workbench || (workbench === "customer" && def.customerReadOnly)
    return inWorkbench && (!def.adminOnly || isAdmin) && (!def.configOnly || canConfigure)
  })
}
