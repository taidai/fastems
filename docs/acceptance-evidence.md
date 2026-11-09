# 仿真验收证据（A 类软件场景）

日期：2026-11-09
范围：《通用型 EMS-Core 评审清单》验收场景中**不依赖真机**的 9 个软件场景。
性质：仿真层证据，验证纯软件逻辑正确性。**不替代真机验收（M5）**——对表、实测跟踪、长稳负载等仍需现场。
复现：`go test ./internal/decision/ ./internal/appapi/ ./internal/ems/command/ -run <对应测试> -v`

## 场景 → 证据映射

| 场景 | 算例输入 | 通过依据 | 实测结果 | 测试 |
|---|---|---|---|---|
| T03 双柜正常分配 | A/B 上限 60/80，目标+100 | 合计100，不越上限 | A=42.9 B=57.1 合计100 ✅ | `TestAcceptanceT03NormalAllocation` |
| T04 单柜降额重分配 | A 降额 60→30，目标+100 | A 不超新上限，合计达标 | A=27.3 B=72.7 合计100 ✅ | `TestAcceptanceT04DerateReallocation` |
| T05 总能力不足 | A/B 上限 30/40，目标+100 | 可行70，缺口30上报 | A=30 B=40 合计70，缺口30 ✅ | `TestAcceptanceT05InsufficientCapacity` |
| T06 并网点闭环 | 主表反送 -30kW | 削减30并正确分配 | 削减分配≈-30 ✅ | `TestPlantAntiReverseClosesLoop` |
| T07（部分）数据过期 | 主表超期 | 过期数据不授权调节 | 过期阻断 ✅ | `TestPlantAntiReverseStaleDataBlocked` |
| T10 回执丢失/重复/迟到 | 重复提交/超时 | 幂等、超时结果未知 | 见 command 包测试 ✅ | `internal/ems/command` |
| T11 保护联锁与复归 | 故障注入/人工解除 | 故障禁调度，解除回待机不自动恢复 | 互锁生效 ✅ | `TestCabinetTransition` `TestFaultInterlocksDispatch` |
| T13 决策表试算/回退 | 草稿规则 | 草稿不调度，发布才生效 | 草稿拒绝 ✅ | `TestRunCapabilityDispatchRejectsDraft` |
| T16 越权/模拟写入 | 各角色×动作 | 默认拒绝，白名单覆盖 | 权限矩阵通过 ✅ | `internal/appapi/permission_test.go` |
| T18 值班员复盘 | 一次调度 | dispatchID 串起能力/分配/设定可复盘 | 追溯完整 ✅ | `TestDispatchRecordTraceable` |
| T27 资源分组共享约束 | 两柜各60目标100，共享上限70 | 合计≤70、缺口30、不重复下发 | 合计70缺口30 ✅ | `TestAcceptanceT27SharedConstraint` `TestGroupDispatchRespectsSharedLimit` |

## 与清单算例的策略差异（已定案）

**SCH-02 分配策略 = 按比例水填充（2026-11-09 定案）**：各柜按可用能力比例分担目标，
利于电池均衡与寿命。清单 T03/T04 算例假设的"按上限优先填满"是另一种策略，
本实现不采用；两者都满足"不越界 + 合计达标"的通用通过依据，仅分配比例不同
（本实现 A=27.3、B=72.7）。T05 因能力用尽，两种策略结果一致（A=30 B=40 缺口30）。
首版分配策略就此固定为按比例，不再列为待裁决。

## 结论

首版单柜储能的 9 个软件场景，仿真层逻辑全部验证通过、可复现。
**已定案**：SCH-02 分配策略 = 按比例水填充。
**待办**：真机验收（M5）覆盖对表/实测跟踪/长稳等物理相关场景。
