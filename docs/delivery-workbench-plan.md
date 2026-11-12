# 交付工作台落地计划

日期：2026-11-11
状态：已实施（2026-11-11），实施差异见文末第 6 节

## 0. 已定决策

| # | 决策 | 结论 |
|---|---|---|
| Q1 | 本次终点 | 只读门槛 + 一键预检，不做发布动作 |
| Q2 | 载体 | 改造现有"部署与验收"页为交付工作台 |
| Q3 | 门槛语义 | 两层：运行时门槛 + A 行进度 |
| Q4 | 严重级 | 阻断＝通道离线、关键点位坏值/过期、已启用规则未发布、命令未知或失败；注意＝人工接管、停用、活动报警、历史为空、备份未演练；通过＝权限 |
| Q5 | 判定位置 | 新增后端聚合接口，预检写审计 |
| Q6 | A 行数据 | 仓库内单一 JSON，前端导入，文档保持叙述 |
| Q7 | 未实现 | 独立 `unimplemented` 状态，灰显，单独计数，不算通过 |
| Q8 | 默认落点 | 首次按角色，之后 localStorage 记住上次工作台 |
| Q9 | 刷新 | 运行时门槛自动轮询；预检手动触发并留痕；两者分开展示 |
| Q10 | 步骤壳 | 7 步：站点 → 通道 → 点位与映射 → 建模 → 策略与试算 → 联调 → 预检 |
| Q11 | 预检存储 | `data/preflight.jsonl` 追加 + 审计摘要 |
| Q12 | 关键点位 | 被启用规则/命令引用 + PCC 反馈 + 能力输入三类 |
| Q13 | 运行验收按钮 | 预检取代，旧接口保留兼容 |
| Q14 | 预检与 A 行 | 预检不含 A 行；A 行独立显示，标注数据日期 |
| Q15 | 主区形态 | 主区放步骤摘要与关键数字，步骤跳转到现有页面 |
| Q16 | 步骤状态 | 从现有数据推导，不手工勾选 |
| Q17 | 两层布局 | 页签：投运门槛 ｜ 验收进度 ｜ 接入工具 |
| Q18 | A 行 JSON | 只读、PR 维护、校验脚本 |
| Q19 | 预检权限 | 实施与管理员（ActionConfigure） |

## 1. 范围

**做**：交付工作台页面（7 步壳 + 门槛页签 + 预检按钮）、后端门槛与预检接口、A 行状态 JSON 与校验、默认工作台记忆。

**不做**：真正的发布动作（A19）、项目包（A17）、把通道/点位/建模/策略页面内嵌进工作台（留作后续演进）、A 行界面编辑。

## 2. 后端

### 2.1 接口

| 方法 | 路径 | 权限 | 行为 |
|---|---|---|---|
| GET | `/api/delivery/gates` | ActionConfigure | 实时计算门槛，不写任何存储；供页面 5–10 秒轮询 |
| POST | `/api/delivery/preflight` | ActionConfigure | 计算同一批门槛，生成快照 ID、写 `preflight.jsonl` 与审计 |
| GET | `/api/delivery/preflight` | ActionConfigure | 读取最近一次预检（文件尾行），没有时返回 200 与 null |

预检响应：

```json
{
  "id": "pf-20261111-201530",
  "generated_at": "2026-11-11T20:15:30+08:00",
  "operator": "zhang@example.com",
  "status": "fail",
  "summary": { "pass": 4, "warn": 3, "fail": 2 },
  "gates": [
    { "key": "channels", "title": "通道", "status": "fail",
      "detail": "2 条通道：离线 1，正常 1", "object": "3 号柜通道",
      "next_step": "检查地址与接线" }
  ]
}
```

`status` 只有 `pass | warn | fail`；`unimplemented` 只出现在 A 行 JSON，不进门槛。

### 2.2 门槛合并规则

| key | 数据来源 | 判定 | 级别 |
|---|---|---|---|
| channels | 运行时通道 + 台账 | 离线 > 0 | fail |
| | | 不稳定 > 0 | warn |
| points | 关键点位集合 | 关键点位过期或坏值 > 0 | fail |
| | | 非关键点位过期 > 0 | warn |
| rules | decisionTable 节点 | 有启用但未发布（`enabled≠0` 且 `applied==""`） | fail |
| | | 没有任何决策表 | warn |
| commands | 点位 `lastCommand` | 存在 unknown / failed | fail |
| manual | 点位 `easyemsManual` | 存在人工接管 | warn |
| disabled | 点位/通道 `disabled` | 存在停用对象 | warn |
| alarms | 活动报警 | 数量 > 0 | warn（附确认入口） |
| history | 历史点位 | 0 个点位 | warn |
| backup | 静态 | 未演练 | warn |
| permissions | 静态 | 服务端逐请求校验 | pass |

同一数据源只出现一次：现有 `/delivery/check` 与 `/ems/acceptance/run` 的重复项（通道、规则、备份）以本表为准各合并为一条。

### 2.3 关键点位推导

关键点位集合 = 三类引用的并集：

1. **启用的决策表**：解析 `applied` 文档的 `bindings.points` 与 `bindings.actions` 中引用的点位 ID；
2. **PCC 反馈**：`plant` 节点的 `pcc` 点 JSON 中的 `meter_point_id` 与三相点位；
3. **能力输入**：`energyUnit` 节点的 `easyemsCapabilityInputs` 中的 SOC、温度、BMS 允许充/放、充放限值点位。

去重后与运行时快照比对新鲜度（沿用 `max_age` 语义：能力输入用各自 `max_age_ms`，其余用站点默认 5 秒）。集合为空（新站尚未配置）时，所有点位按非关键处理，并在门槛 `detail` 中说明。

### 2.4 持久化与审计

- `data/preflight.jsonl` 每行一条完整快照；与 `audit.jsonl`、`commands.jsonl` 同为追加式，不做数据库。
- 审计记录：动作 `delivery.preflight`，字段含快照 ID、状态、汇总计数；不写门槛明细（明细在 JSONL）。
- `GET /api/delivery/preflight` 读文件尾行；文件不存在返回 404。

### 2.5 代码结构

- 新增 `internal/appapi/preflight.go`：门槛模型、合并、关键点位推导、两个 handler。
- 重构 `internal/appapi/delivery.go`：抽出 `deliveryItems(identity) ([]DeliveryItem, error)`，供原 handler 与门槛复用；对外行为不变。
- 复用 `internal/appapi/acceptance.go` 的 `buildAcceptanceReport`，门槛从中取通道/点位/规则/报警/历史/权限/备份项。
- 路由注册在 `handler.go` 的 `delivery/*` 分支。

### 2.6 测试

- 单测：关键点位推导（规则引用、PCC、能力输入三类各一例，含交集去重）。
- 单测：合并规则（通道离线→fail；仅人工接管→warn；无决策表→warn；全部正常→pass）。
- 集成：实施可 POST，viewer 403；预检后 `preflight.jsonl` 多一行且审计多一条；GET 返回最近快照；无历史时 404。
- 现有 `TestModbusCSVDiffAndDeliveryEvidence` 必须继续通过（重构 deliveryCheck 的回归）。

## 3. 前端

### 3.1 页面改造（DeployView → 交付工作台）

- 入口与路由不变：仍为 `deploy` 页，导航标签改为"交付工作台"，`pageInfo` 标题同步。
- 布局：左侧 240px 步骤栏，右侧内容区。

```
┌ 交付工作台 ─────────────────────────────────────────────┐
│ 站点       │ 当前步骤摘要：通道                                     │
│ 1 站点 ✓   │ 通道 2 条 · 在线 1 · 离线 1        [打开通道页 →]     │
│ 2 通道 ●   │                                                      │
│ 3 点位与映射│ ┌ 投运门槛 ┬ 验收进度 ┬ 接入工具 ┐                     │
│ 4 建模     │ │ 实时门槛（10 秒轮询）                              │ │
│ 5 策略     │ │ 通道      ✗ 1 条离线    检查地址与接线             │ │
│ 6 联调     │ │ 关键点位  ✓ 12/12 新鲜                             │ │
│ 7 预检     │ │ ……                                                │ │
│            │ │ [运行预检]  最近一次：11-11 20:15 张工 fail        │ │
└────────────┴──────────────────────────────────────────────────────┘
```

### 3.2 步骤与状态推导

| 步骤 | 跳转目标 | 完成条件 |
|---|---|---|
| 1 站点 | 无（顶栏切换） | 已登录且存在根节点 |
| 2 通道 | `channels` | 至少 1 条通道有成功轮询 |
| 3 点位与映射 | `points` | 至少 1 个点位有值 |
| 4 建模 | `energyUnits` | 至少 1 个 `energyUnit` 节点 |
| 5 策略与试算 | `rules` | 至少 1 条已启用决策表 |
| 6 联调 | `ledger` | 台账中至少 1 台设备已验证 |
| 7 预检 | 本页门槛页签 | 最近一次预检状态为 pass |

状态三值：`done / current / pending`；`current`＝第一个未完成步骤。

### 3.3 A 行进度

- 数据文件：`frontend/src/data/acceptance-status.json`（tsconfig 已开 `resolveJsonModule`，`include: src`）。
- 字段：`{ "id": "A01", "title": "...", "status": "implemented|partial|unimplemented", "summary": "...", "evidence": ["TestX"], "gap": "...", "updated_at": "2026-11-11" }`。
- 页面：表格列出 23 行，按状态分组计数；`unimplemented` 灰显单独计数，不计入通过率分母之外的"通过"。
- 校验脚本：`frontend/acceptance-status-check.mjs`，检查 A01–A23 齐全、状态合法、implemented/partial 必须有 evidence；挂 `npm run check:acceptance`。

### 3.4 默认工作台

- 首次登录：实施/管理员 → 交付；其他 → 客户。
- 之后读 `localStorage["easyems-workbench"]`；切换时写入。
- 若记住的值当前角色不可见（如被降权），回落到角色默认。

### 3.5 预检交互

- 门槛页签默认展示实时门槛（GET，10 秒轮询），每行显示级别、对象、下一步。
- "运行预检"按钮：POST，成功后显示快照时间与操作者；失败（500）显示错误，不产生假记录。
- 页面同时显示"实时状态"与"最近一次预检"；两者时间不同时，标注"快照早于实时状态"。
- 旧的"运行验收"按钮删除；"接入工具"页签保留现有扫描、点表校验与接入向导。

### 3.6 测试

- `navigation-check.mjs`：deploy 仍在交付工作台，标签变更不影响既有断言。
- 新增 `workbench-check.mjs`（逻辑）：步骤状态推导函数对给定快照的期望值；门槛级别映射与后端一致（同一份 fixture JSON）。
- 浏览器检查：管理员进入交付工作台看到 7 步与门槛，点击"运行预检"后出现快照时间；viewer 不可见该页（现有 configOnly 已覆盖）。

## 4. 实施顺序

1. 后端门槛模型与关键点位推导 + 单测（不改 UI）。
2. `GET /gates`、`POST /preflight`、JSONL 与审计 + 集成测试。
3. `acceptance-status.json` + 校验脚本 + `npm run check:acceptance`。
4. 前端工作台布局：步骤栏、步骤摘要、三个页签、预检按钮。
5. 默认工作台记忆。
6. 浏览器验证与截图；更新导航总览与相关文档。

## 5. 对验收基线的影响（如实）

- **A10**（现场联调与投运门槛）：本次把"通道/点位/规则/命令"变成具名门槛，但"方向未确认""PCC 主表过期"这类具名阻塞项仍不存在，A10 仍是**部分**。
- **A14**（交接）：预检快照可作为交接证据的一部分，但交接清单与 S/H 标注未实现，A14 仍是**部分**。
- 其余 A 行状态不变；本计划不新增验收行。

## 6. 实施结果（2026-11-11）

**后端**
- `internal/appapi/preflight.go`：10 项门槛（通道、关键点位、规则、命令、人工接管、停用、报警、历史、备份、权限）、关键点位推导（启用决策表引用 + PCC 反馈 + 能力输入）、`preflight.jsonl` 追加与最近快照读取。
- `handler.go` 注册 `GET /api/delivery/gates`、`POST /api/delivery/preflight`、`GET /api/delivery/preflight`，均要求 `ActionConfigure`。
- `internal/appapi/preflight_test.go`：关键点位推导、持久化、HTTP 语义（含查看者 403）三组测试通过；`TestModbusCSVDiffAndDeliveryEvidence` 回归通过。

**前端**
- `DeliveryWorkbench.tsx`：7 步导航、步骤摘要、三个页签、10 秒轮询、预检按钮与最近快照。
- `data/acceptance-status.json`：A01–A23 状态与证据，`npm run check:acceptance` 校验（5 实现 / 12 部分 / 6 未实现）。
- 默认工作台：首次按角色进入，之后 localStorage 记忆；角色降权回落到客户工作台。
- `deploy` 页改为交付工作台并排在交付工作台首位，实施/管理员登录后落在这一页。

**与计划的差异**
- 没有重构 `delivery.go` 的 `deliveryItems`：预检自成一套门槛计算，避免改动已被测试覆盖的交付检查路径；两处对"命令未知/失败"的判定逻辑目前重复，后续可合并。
- `DeployView` 增加 `embedded` 参数隐藏旧验收卡片，而不是删除代码，保留回退余地。

**验证**
- `go test`（预检 + 交付证据）通过；`npm run build` 通过；导航逻辑检查通过；浏览器端到端检查通过（默认交付工作台、运行预检、打开设备台账、切客户工作台变只读）。
- 证据截图：`frontend/evidence/navigation-delivery.png`、`workbench-acceptance.png`、`workbench-tools.png`。

## 7. 第二轮补齐（2026-11-11）：复用、需量、计划、版本

按产品描述继续补齐其声称但此前缺失的能力：

**A01 项目复用**
- `internal/appapi/reuse.go`：`GET /api/projects/reuse/preview?source=` 给出复用/变化/待确认清单；`POST /api/projects/reuse` 克隆整棵站点树。
- 凭据、运行时值、旧连接参数不复制；跨节点引用（PCC、能力输入、功率设定点、规则绑定）按旧→新 ID 重写，无法映射的引用被移除并列入待确认。
- 交付工作台"站点"步骤提供源项目选择、预览与创建面板。
- 测试：`TestReuseProject`、`TestRewriteReferences`。

**A08 15 分钟滑动需量**
- `decision.SlidingDemand`：`(t-15min, t]`、15 个一分钟 bin、bin 内先平均再对 bin 平均；缺 bin 不可用并列出缺样区间。
- `history.QueryByPoint` 支持跨设备按点位查历史；`demo` 之外的快照仍从运行时历史读取。
- 削峰改为只使用窗口需量；缺 bin 时不做预测性削峰，防逆流与瞬时限值不受影响。
- `GET /api/demand?site=` 返回逐电站窗口与缺样；工作台"策略与试算"展示。
- 测试：`TestSlidingDemand`、`TestPlantDemandWindow`。

**A09 96 点计划**
- 站点节点保存 `easyemsPlan96`：96 个 15 分钟时段，null 表示无计划。
- `GET/PUT /api/projects/plan96`；`SiteUnitGoals` 当前时段有计划时按计划分配，否则回落到站级目标；空缺时段不填零。
- 工作台"策略与试算"提供 24×4 网格编辑与保存。
- 测试：`TestPlan96TargetAt`；原有硬优先级测试继续通过。

**A13 / A19 版本发布与回退**
- `POST /api/projects/publish` 冻结当前工程配置为版本（原子写 `data/projects/<site>/<id>.json` + 审计）；`GET /api/projects/versions` 列表；`POST /api/projects/rollback` 重放版本配置。
- 回退前自动冻结当前配置；历史（审计、报警、命令）不随回退变化，也不回放旧命令。
- 版本文件不含凭据与运行时值；回退写入使用当前时间戳，避免被存储当作过期写入丢弃。
- 测试：`TestProjectVersionPublishAndRollback`。
- A19 仍为部分：发布前预检阻断与迁移尚未实现。

**状态变化**：A01、A08、A09 升为已实现；A19 由未实现升为部分；当前为 8 已实现 / 11 部分 / 4 未实现（A17、A18、A21、A23）。

## 8. 第三轮补齐（2026-11-11）：换厂商批量重映射

**A02 / A15**
- `internal/appapi/profile_apply.go`：`POST /api/modbus/{channel}/units/apply`，一次把 Profile 应用到多个站号；支持 `dry_run` 预览。
- 按逻辑点名称自动匹配物理点位，逐点给出 `exact / scaled / missing`；缩放不一致会标注 Profile 与点位的差异。
- 命令与参数引用的点位缺失、只读或未映射时，对应控制进入 `blocked` 列表，不会被当成可用。
- 换厂商保留旧型号上的业务节点绑定（画面与策略引用不变），但 `verified` 复位，必须重新验证。
- 交付工作台"点位与映射"步骤提供通道、Profile、站号范围、预览与应用。
- 测试：`TestApplyProfileVerdicts`（12 精确 / 2 缩放 / 2 缺失，两个命令因点位只读被阻断）、`TestApplyProfilesPersistsModels`（批量应用落盘并保留业务绑定）。

**状态变化**：A02、A15 升为已实现；当前 10 已实现 / 9 部分 / 4 未实现（A17、A18、A21、A23）。
