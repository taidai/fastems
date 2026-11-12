export type Page = "devices" | "screen" | "dispatchTrace" | "ledger" | "points" | "channels" | "aggregate" | "deploy" | "allocators" | "energyUnits" | "unitWiring" | "resourceGroups" | "alarms" | "history" | "health" | "rules" | "users" | "audit" | "delivery" | "opcua"

export type Workbench = "customer" | "delivery"

// 导航按交付任务分组，不按后端模块分组；见 docs/jobs.md。
export type Group = "run" | "access" | "assets" | "strategy" | "acceptance" | "system"

export type PageDef = {
  key: Page
  label: string
  group: Group
  adminOnly?: boolean
  configOnly?: boolean
  // 可路由但不进一级导航：由总览、画面、报警内的链接进入（“查证”不是独立任务）。
  hidden?: boolean
}

export type GroupDef = { key: Group; label: string; workbench: Workbench }

export const groupDefs: GroupDef[] = [
  { key: "run", label: "运行", workbench: "customer" },
  { key: "access", label: "接入", workbench: "delivery" },
  { key: "assets", label: "能源设备", workbench: "delivery" },
  { key: "strategy", label: "策略", workbench: "delivery" },
  { key: "acceptance", label: "交付验收", workbench: "delivery" },
  { key: "system", label: "系统", workbench: "delivery" },
]

// 页面顺序即默认落点：客户落到总览，交付落到验收清单（项目状态与下一步）。
export const pageDefs: PageDef[] = [
  { key: "devices", label: "总览", group: "run" },
  { key: "screen", label: "画面", group: "run" },
  { key: "alarms", label: "报警", group: "run" },
  { key: "history", label: "历史", group: "run", hidden: true },
  { key: "dispatchTrace", label: "调度追溯", group: "run", hidden: true },
  { key: "health", label: "通信健康", group: "run", hidden: true },
  { key: "deploy", label: "验收清单", group: "acceptance", configOnly: true },
  { key: "ledger", label: "设备验证", group: "acceptance", configOnly: true },
  { key: "channels", label: "通道", group: "access", configOnly: true },
  { key: "points", label: "点位表", group: "access", configOnly: true },
  { key: "aggregate", label: "聚合计算", group: "access", configOnly: true },
  { key: "opcua", label: "OPC-UA", group: "access", configOnly: true },
  { key: "energyUnits", label: "储能柜", group: "assets", configOnly: true },
  { key: "unitWiring", label: "接线", group: "assets", configOnly: true },
  { key: "resourceGroups", label: "资源集群", group: "assets", configOnly: true },
  { key: "rules", label: "决策表", group: "strategy", configOnly: true },
  { key: "allocators", label: "功率分配器", group: "strategy", configOnly: true },
  { key: "users", label: "账号与权限", group: "system", adminOnly: true },
  { key: "audit", label: "审计日志", group: "system", adminOnly: true },
]

export const workbenchNames: Record<Workbench, string> = { customer: "客户", delivery: "交付" }
export const workbenchFullNames: Record<Workbench, string> = { customer: "客户工作台", delivery: "交付工作台" }

// 可路由页面（含隐藏页）：用于路由守卫和权限判断。
export function visiblePagesFor(workbench: Workbench, isAdmin: boolean, canConfigure: boolean): PageDef[] {
  if (workbench === "delivery" && !canConfigure) return []
  return pageDefs.filter((def) => {
    const group = groupDefs.find((item) => item.key === def.group)
    return group?.workbench === workbench && (!def.adminOnly || isAdmin) && (!def.configOnly || canConfigure)
  })
}

// 一级导航与页签：隐藏页不出现，空分组不出现。
export function navGroupsFor(workbench: Workbench, isAdmin: boolean, canConfigure: boolean): Array<GroupDef & { pages: PageDef[] }> {
  const visible = visiblePagesFor(workbench, isAdmin, canConfigure)
  return groupDefs
    .filter((group) => group.workbench === workbench)
    .map((group) => ({ ...group, pages: visible.filter((def) => def.group === group.key && !def.hidden) }))
    .filter((group) => group.pages.length > 0)
}
