# 存量数据迁移：点位级 → 能力级

日期：2026-11-08
状态：能力级调度前后端闭环已完成（见 `domain-model-refactor-plan.md`），本文件说明存量点位级数据的处置（Q12=A：清空重来，历史留档只读）。

## 原则

- 旧点位级模型（decisionTable 节点、`easyemsAllocator`/`easyemsAggregate` point）与新能力级模型（`site`/`plant`/`energyUnit`/`device` 节点 + `easyemsCapabilityRule`/`easyemsEnergyAllocator`）**语义不兼容**，不自动迁移。
- 历史数据（点位 `value`/`valueSet`、告警事件、审计日志）**留档只读**，不迁入新模型。
- 新模型从**空实例**开始：重新接入设备、按能力级重建规则与分配器。

## 迁移步骤（实施现场）

1. **备份**：停止应用，复制数据目录（含 JetStream、audit.jsonl、commands.jsonl），作为只读历史档案。
2. **新建数据目录**：全新数据目录启动（首次启动自动 Bootstrap 管理员）。
3. **接入**：按接入向导扫描 → 绑 Profile → 映射点位（此时设备落在新的 `device` 节点下）。
4. **建能源节点**：创建 `energyUnit` 节点，把柜内 Device 挂到其下。
5. **配能力级规则/分配器**：调度 → 能源节点，配置 `easyemsCapabilityRule`（条件引用能力字段、动作是功率请求）与 `easyemsEnergyAllocator`（成员是 EnergyUnit）。
6. **验收**：用模拟器/真机跑能力级调度闭环（规则命中 → 拆解 → 写设备 → 回读）。

## 清理

旧 decisionTable 节点与 `easyemsAllocator` point 不再被能力级调度读取；确认新模型验收后，可从旧数据目录删除（历史档案已备份）。

## 不做的事

- 不自动改写旧规则到能力级（语义差异大，自动改写易错）。
- 不保留旧 decisionTable 节点的运行（能力级调度已接管）。
