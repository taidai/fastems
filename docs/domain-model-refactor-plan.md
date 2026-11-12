# FastEMS 领域模型重构 · 实施计划

日期：2026-11-08
基线：`docs/domain-model-refactor-decision.md`（14 决策定案）

## 技术事实（已查清）

- Node type 是字符串常量（`third_party/simpleiot/data/schema.go`）；**`device` 节点已存在**（Q9=A 直接复用），`site`/`plant`/`energyUnit` 需加 3 个常量。
- 纯数据节点（site/plant/energyUnit/device）不需要 client，只挂 point、由 appapi 读写。
- 破坏性重构**不动 Simple IoT 上游**，全部改动在 FastEMS 层。

## 四阶段

### 阶段 1：领域模型核心
新增 `site`/`plant`/`energyUnit` 常量；定义四层节点字段、Capability 模型、asset_code；数据模型 API。
**验证**：`go test ./...` 绿 + 能创建/读写四层节点。

### 阶段 2：调度语义切换
规则/分配器/聚合从「点位级」改为「能力级」（引用 EnergyUnit.Capability）；decision 引擎能力级调度。
**验证**：决策表/分配器单测绿，能力级规则模拟命中。

### 阶段 3：站级/电站级调度（Q10=B）
Site Scheduler（总目标/防逆流/需量/跨站）+ Plant Scheduler（功率分配/柜间协调/电站约束）。
**验证**：站级/电站级分配单测绿。

### 阶段 4：模拟器重建 + 验证
重建 4 柜模拟器（新模型），M4 证据重跑 + 新调度验证。
**验证**：`browser-smoke`/`layout-check`/`theme-check` 全绿，M4 能力在新模型复现。

## 复用不动（Q6=A）

`styles.css`/主题、`HistoryView`、`AlarmsView`+面板、`AuditView`、登录/权限层、`DeployView` 向导框架、`modbusrt` 协议层、`framelog` 报文层。

## 重写面

后端：`unitmodels`/`ledger`/`modbus`/`rules`/`allocators`/`aggregate`/`resources`/`evidence`/`delivery_evidence`/`handler` + `emsapi` + `bridge`/`channel`/`device`/`command`/`alloc`/`runtime` + `decision`（部分）。
前端：`deviceTypes`/`DeviceLedgerView`/`DeviceDetail`/`CommunicationView`/`PointsView`/`DecisionEditor`/`AllocatorsView`/`App`。
