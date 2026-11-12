# 验收证据映射（A 行）

日期：2026-11-11
基线：`docs/acceptance-map.html` 的 A01–A23。**A 行编号是本仓库唯一的验收编号**；旧 Txx / OBJ-xx 编号只作历史对照（见文末）。
性质：自动化证据只能证明软件行为；`H` 类仍需真机，模拟器通过不等于现场可用。

## 状态汇总

| 结论 | 行 |
|---|---|
| 已实现，可自动取证 | A01、A02、A03、A04、A05、A08、A09、A11、A12、A15 |
| 部分实现（有自动化，但验收要求的一半没有代码） | A06、A07、A10、A13、A14、A16、A19、A20、A22 |
| 未实现（无代码、无测试，不得签收） | A17、A18、A21、A23 |

当前仍明确未实现或只兑现一半的部分：

- **A17 项目包导出/导入**：版本快照只落在本机 `data/projects/`，没有跨机 manifest 与引用重写。
- **A18 应用资产生成**：页面仍是手写 React，配置不能生成页面，换站需要发版。
- **A19 发布前的预检阻断与迁移**：版本发布是原子快照，但尚未用预检失败来阻断发布。
- **A21 / A23**：MQTT 设备凭据、ACL、下行与 STS 三态仍未建模。


| A | 结论 | 自动化证据（仓库路径 · 测试） | 已覆盖 | 缺口（需实现，或需人工/现场） |
|---|---|---|---|---|
| A01 | 已实现 | `internal/appapi/reuse_test.go` · TestReuseProject、TestRewriteReferences<br/>`internal/appapi/multi_tenant_integration_test.go` · TestMultiTenantIsolation、TestTreeScopeHTTPIsolation | 从已交付项目克隆整棵站点树；给出复用/变化/待确认清单；凭据、运行时值与旧节点引用不复制，无法映射的引用移除并列入待确认 | 方向确认与现场复验属于 A10；“源项目已签收”门槛尚未建模 |
| A02 | 已实现 | `internal/appapi/profile_apply_test.go` · TestApplyProfilesPersistsModels、TestApplyProfileVerdicts<br/>`internal/ems/profile/profile_test.go` · TestCatalogProfilesCarryPointsAndVerification、TestProfileCSVMatchesPointTableFormat | 批量应用到多个站号；按逻辑点名称自动映射；业务节点绑定保持；缺回读/只读/未映射的控制阻断 | 现场批量换柜与画面绑定的真机复验仍需 M5 |
| A03 | 已实现 | `internal/decision/scenario_test.go` · TestRunScenariosVerdicts、TestRunScenariosAtSimulatedTime<br/>`internal/decision/client_integration_test.go` · TestDecisionTableOnSimpleIoT<br/>`internal/appapi/energy_run_integration_test.go` · TestRunCapabilityDispatch、TestRunCapabilityDispatchRejectsDraft | 同一输入试算、场景判定、草稿不生效、仲裁给出原因 | 金标准表尚未建立（当前只断言“一致”，未断言“正确”）。需按 A03 冻结 fixture 与期望值 |
| A04 | 已实现 | `internal/appapi/multi_plant_integration_test.go` · TestMultiPlantIndependentPCC<br/>`internal/appapi/pcc_run_integration_test.go` · TestPlantAntiReverseClosesLoop、TestPlantAntiReverseStaleDataBlocked<br/>`internal/appapi/resource_group_integration_test.go` · TestGroupDispatchRespectsSharedLimit<br/>`internal/decision/resource_group_test.go` · TestAcceptanceT27SharedConstraint<br/>`internal/decision/site_scheduler_test.go` · TestAllocateSite | 逐 PCC 独立闭环、全站净额为零仍判越限、组上限截断与缺口、组内不重复下发 | 缺“成员分属两个 Plant 的同一个组”的跨 Plant 用例；需补一条集成测试 |
| A05 | 已实现 | `internal/appapi/capability_refresh_test.go` · TestRefreshCapabilities<br/>`internal/decision/energy_allocator_test.go` · TestAllocateEnergyDirectionalCapabilityAndStatus<br/>`internal/decision/dynamic_capability_test.go` · TestDynamicCharge、TestDynamicDischarge<br/>`internal/decision/distribute_test.go` · TestDistributeDerateZeroesOneDirection<br/>`internal/ems/state/state_test.go` · TestValueGoesStaleByAge<br/>`internal/ems/chain_test.go` · TestSafetyBlocksDispatchWhenCapabilityInputIsStale | 分方向能力、单方向过期、SOC 过期双向关闭、过期值不授权控制 | 六个固定数值用例（96%、6000 ms、55℃）未做成 fixture，需按 A05 逐例固化 |
| A06 | 部分 | `internal/ems/command/command_test.go` · TestSubmitWritesAndReadbackConfirms、TestPointWriteRequiresFreshReadback、TestTimeoutBecomesUnknown、TestPointWriteTimeoutFallsBackOnce、TestPointWriteDuplicateConflictAndIdempotency、TestFallbackUsesLastConfirmedAndSupersede<br/>`internal/ems/device/device_test.go` · TestCheckConfirmsFromReadback<br/>`internal/ems/runtime/runtime_test.go` · TestEngineConfirmsCommandWithinOneCycle | 提交后新样本才确认、旧样本不能补认、超时结果未知、安全回退一次、幂等与冲突 | **功率跟踪偏差（容差、持续时间、与设定确认分开展示）没有实现**；需先实现再验收 |
| A07 | 部分 | `internal/decision/cabinet_state_test.go` · TestCabinetTransition、TestNeedsManualRelease、TestStopWaitsForConfirmation、TestAllowsAutoDispatch<br/>`internal/appapi/cabinet_state_integration_test.go` · TestFaultInterlocksDispatch<br/>`internal/ems/alarm/alarm_test.go` · TestMaintenanceSuppressesRaiseUntilReleased<br/>`internal/ems/runtime/runtime_test.go` · TestEngineRaisesAndClearsOfflineAlarm | 故障互锁、检修不自动写、故障解除需人工、离线告警、停机等确认 | **按场景的故障动作表（限功率/停充/停放/联动）没有实现**，当前只有“阻断或保持”；失联期间写入 0 缺显式断言 |
| A08 | 已实现 | `internal/decision/demand_window.go` · TestSlidingDemand<br/>`internal/appapi/demand.go` · TestPlantDemandWindow | 15 个一分钟 bin、`(t-15min,t]` 窗口、缺 bin 不可用并列出缺样区间；削峰只使用窗口需量，缺 bin 不做预测性削峰 | 现场秒级采样与长稳表现仍需 M5 |
| A09 | 已实现 | `internal/appapi/demand.go` · TestPlan96TargetAt<br/>`internal/decision/arbitration_test.go` · TestArbitrate、TestOrderByPriority<br/>`internal/appapi/plant_strategy_integration_test.go` · TestPlantPeakValleyStrategy | 96 点计划按 15 分钟时段生效，空缺时段无目标而非 0；硬优先级与胜负来源不变 | 计划编辑的现场联调与金标准表仍属人工 |
| A10 | 部分 | `internal/appapi/delivery_evidence_test.go` · TestModbusCSVDiffAndDeliveryEvidence（`/delivery/check`、`/delivery/evidence`）<br/>`internal/appapi/preflight_test.go` · TestDeliveryPreflight（投运门槛、预检快照、权限）<br/>`internal/ems/alarmwatch/alarmwatch_test.go` · TestBusHeartbeat、TestNewBusGetsGrace<br/>`internal/ems/chain_test.go` · TestSafetyBlocksDispatchWhenCapabilityInputIsStale | 交付检查项（规则未发布、人工接管、停用、命令失败、Modbus 心跳）、证据 zip；投运门槛（通道/关键点位/规则/命令等 10 项）与预检快照 | “方向未确认”“PCC-2 主表过期”这类具名阻塞项与复验步骤没有实现 |
| A11 | 已实现 | `internal/appapi/multi_tenant_integration_test.go` · TestTreeScopeHTTPIsolation、TestMultiTenantIsolation<br/>`internal/appapi/permission_test.go` · TestCanAction*<br/>`internal/emsapi/emsapi_test.go` · TestTreeScopeFiltersProxyReads、TestTreeScopeGuardsChannelMutations、TestChannelConfigurationRequiresAdmin<br/>`internal/decision/ownership_test.go` · TestOutputTargetsEmptyMeansNoOwnership | 跨站 403、本站 200、viewer 被拒、白名单覆盖角色、审计记录 | 无（MQTT 设备凭据与 ACL 已移出本行，见 A21） |
| A12 | 已实现 | `internal/ems/modbusrt/modbusrt_test.go` · TestRealTCPFrames、TestRealRtuFramesReadAndWrite、TestParseSerialAddressRejectsInvalid<br/>`internal/ems/channel/channel_test.go` · TestDecodesSignedAndFloatFormats、TestHealthGoesOfflineWhenSuccessIsStale、TestPollsAllDevicesOnOneChannel<br/>`internal/ems/opcua/runner_test.go` · TestParseNodeIDFormats、TestRunnerPollsAndWritesMappedPoints、TestRunnerReportsHealthAndReconnects<br/>`internal/appapi/opcua_integration_test.go` · TestOpcuaRunnerPublishesValues<br/>`internal/ems/state/state_test.go` · TestOfflineDeviceMarksPointsStale | 缩放与有符号/浮点解码、真实 TCP/RTU 帧、NodeID 解析、断线重连、离线不伪造在线值 | 真机对表仍需 M5；证书/用户认证未实现，不宣称支持 |
| A13 | 部分 | `internal/appapi/project_version_test.go` · TestProjectVersionPublishAndRollback<br/>`internal/decision/rulehistory_test.go` · TestReplayVersionAtTime、TestDiffDocumentsUnchanged<br/>`internal/audit/audit_test.go` · TestAppendQueryAndCSV | 版本快照、回退重放工程配置、回退前自动快照；审计与历史不回滚、不回放命令 | 三方差异的人工确认角色（installer 403、交付确认）未实现；升级迁移尚无独立用例 |
| A14 | 部分 | `internal/appapi/handler_integration_test.go` · TestProductAPIVerticalSlice<br/>`internal/appapi/energy_device_create_integration_test.go` · TestUserBuiltUnitControlsDevice<br/>`internal/appapi/energy_control_integration_test.go` · TestEnergyControlEvents<br/>`internal/ems/alarm/alarm_test.go` · TestAcknowledgeAndCloseRules<br/>`frontend/browser-smoke.mjs`、`frontend/m5-site-acceptance.mjs`（容器/浏览器） | 建单元到控设备、启停与人工、报警确认、页面冒烟 | 交接清单、S/H 证据标签、角色×动作矩阵、客户账号无配置入口，均无实现；`/ems/acceptance/run` 无集成测试 |
| A15 | 已实现 | `internal/appapi/profile_apply_test.go` · TestApplyProfileVerdicts、TestApplyProfilesPersistsModels | 逐点判定 exact/scaled/missing；缩放不一致标注；缺失或只读点位阻断相关控制；业务绑定保持 | 簇数或管理边界变化按新单元重建，不属于换点流程 |
| A16 | 部分 | `internal/decision/energy_allocator_test.go` · TestAllocateEnergyDirectionalCapabilityAndStatus<br/>`internal/decision/distribute_test.go` · TestDistributeProportionalSplit、TestDistributePriorityFillsHigherFirst、TestDistributeWeightCappedAtCapacity<br/>`internal/decision/allocator_integration_test.go` · TestAllocatorRejectsSharedOwnership<br/>`internal/ems/alloc/alloc_test.go` · TestProportionalSplit、TestSaturatesAtCapacityAndReportsShortfall | 功率按 kW 与实时能力分配、比例水填充、越权归属拒绝、缺口上报 | **多簇管理器的内部簇没有建模**（全仓库无 cluster 概念），内部簇遥测与“恰好 1 次调度”无法验证；522/261 fixture 未建；能量计划未实现 |
| A17 | 未实现 | 无 | — | 项目包导出/导入、manifest 哈希、引用重写、离线安装全部没有代码 |
| A18 | 未实现 | 无 | — | 画面/报警/历史/角色动作是手写 React，无配置生成路径 |
| A19 | 部分 | `internal/appapi/project_version_test.go` · TestProjectVersionPublishAndRollback | 一键冻结当前配置为版本、原子写文件、写审计；失败不产生半个版本 | 发布前预检阻断、迁移与失败注入未实现；无跨机部署语义 |
| A20 | 部分 | `internal/appapi/commandlog_test.go` · TestCommandHistorySurvivesRestart<br/>`internal/ems/rule/engine_test.go` · TestPrimeSuppressesReplayAfterRestart<br/>`internal/ems/command/command_test.go` · TestSubmitIsIdempotent、TestPointWriteDuplicateConflictAndIdempotency<br/>`internal/ems/history/history_test.go` · TestCapDropsOldest、TestSetLimitTrimsExistingSamples | 重启后命令历史、重启不重放、幂等键恰好一次、内存上限 | 关系库与 Outbox 未实现；断库恢复场景不适用；当前只按 JSONL 口径签收 |
| A21 | 未实现 | 无 | — | MQTT 仅上行订阅（`third_party/simpleiot/client/mqtt*.go`）；无设备级凭据、无 Topic ACL、无下行命令、无 retained 判据 |
| A22 | 部分 | `frontend/restore-check.mjs`（离线整目录恢复，校验登录/节点/输出）<br/>`internal/audit/audit_test.go` · TestAppendQueryAndCSV | 停机复制数据目录后恢复可登录、节点与输出存在 | 脚本针对上游演示节点，未校验 EMS 的 4 单元/100 历史点/214 审计；需按 A22 在真站数据上补一次 |

## 复现命令

```bash
go test ./internal/appapi/ ./internal/emsapi/ -run 'TreeScope|MultiTenant|CanAction' -v
go test ./internal/decision/ -run 'Arbitrate|OrderByPriority|AcceptanceT27|AllocateSite' -v
go test ./internal/ems/command/ ./internal/ems/device/ ./internal/ems/runtime/ -v
go test ./internal/ems/modbusrt/ ./internal/ems/channel/ ./internal/ems/opcua/ -v
go test ./internal/appapi/ -run 'MultiPlantIndependentPCC|GroupDispatchRespectsSharedLimit|DeliveryEvidence' -v
```

## 旧编号对照

| 旧编号 | A 行 | 说明 |
|---|---|---|
| T03 / T04 / T05 / T27 | A04 | 分配、降额、能力不足、组共享上限 |
| T06 / T19 | A04 | 并网点闭环、逐 PCC 独立 |
| T07 | A05 | 过期数据不授权 |
| T10 | A06 | 回执丢失/重复/迟到 |
| T11 | A07 | 互锁与复归 |
| T13 | A03 | 决策表试算与草稿 |
| T16 | A11 | 越权与默认拒绝 |
| T18 | A14 | 追溯串起能力/分配/设定 |
| T23 | A09 | 光储充联合调节（当前为站级目标 + 分时时段） |
| T24 | A04 / A12 | 逐相判定（仅对称调节） |
| T26 | A08 | 仅瞬时削峰，非 15 分钟滑动窗口 |
| OBJ-02 | A09 | 多目标仲裁 |

---

# 历史：2026-11-09 场景证据（T 编号，保留）

范围：《通用型 EMS-Core 评审清单》验收场景中**不依赖真机**的 9 个软件场景。
性质：仿真层证据，验证纯软件逻辑正确性。**不替代真机验收（M5）**——对表、实测跟踪、长稳负载等仍需现场。
复现：`go test ./internal/decision/ ./internal/appapi/ ./internal/ems/command/ -run <对应测试> -v`

## 场景 → 证据（旧表）

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
| T19 多站逐 PCC 独立闭环 | A 反送-30、B 正常+30 | A 独立削减、B 独立达标不动作 | A 削减-30、B 不动作 ✅ | `TestMultiPlantIndependentPCC` |
| T23 光储充联合调节 | 光伏降至35，储能增放/充电减载 | 净购电维持 60，缺口如实上报 | 两步均回 60 ✅ | `TestAcceptanceT23MixDispatch` |
| T24 三相合计与逐相反送 | 三相 −6/+3/+3，反送限值 0 | 合相为 0、A 相反送 6，不支持分相如说明 | A 相反送 6 ✅ | `TestAcceptanceT24PerPhase` |
| T26（部分）需量窗口 | 瞬时功率上限 | 瞬时需量削减 | PeakShaving ✅ | `TestPeakShaving` |
| OBJ-02 多目标仲裁 | 单柜放电+60 vs 防逆流充电-50 | 仲裁选防逆流，写充电负值 | 写充电-50 ✅ | `TestPlantAntiReverseWritesDevice`（RunUnitArbitratedDispatch） |

## 与清单算例的策略差异（已定案）

**SCH-02 分配策略 = 按比例水填充（2026-11-09 定案）**：各柜按可用能力比例分担目标，
利于电池均衡与寿命。清单 T03/T04 算例假设的"按上限优先填满"是另一种策略，
本实现不采用；两者都满足"不越界 + 合计达标"的通用通过依据，仅分配比例不同
（本实现 A=27.3、B=72.7）。T05 因能力用尽，两种策略结果一致（A=30 B=40 缺口30）。
首版分配策略就此固定为按比例，不再列为待裁决。

## 结论（旧）

首版单柜储能的 9 个软件场景，仿真层逻辑全部验证通过、可复现。
**已定案**：SCH-02 分配策略 = 按比例水填充。
**待办**：真机验收（M5）覆盖对表/实测跟踪/长稳等物理相关场景。

# 历史：2026-11-10 剩余缺口收口（软件侧）

证据来自 `go test`、前端生产构建与真实浏览器/容器验证；不代表 M5 真机签收。

### 1. 树范围授权与生产演示策略

- 新 EMS 路由（能源设备、规则/模拟、设备、能力来源、追溯、电站 PCC/策略、资源集群、站点目标、OPC-UA）统一使用 `h.node`/`h.allows`；列表按可见范围过滤，创建校验可见父节点与所有引用。
- `emsapi` 代理的 channels/state/history/quality/重启/创建/删除同样按节点可见性过滤；管理员也不能越过范围。
- `TestTreeScopeHTTPIsolation`（真实 HTTP，端口 9810/9811）证明站点 A 用户对站点 B 的读取、写入、发布、组调度全部 403，且无点位/追溯副作用。
- 生产新实例默认不生成演示站点；仅 `FASTIOT_SEED_DEMO=1` 显式启用，验收脚本已显式打开。

### 2. 动态能力：分方向、BMS 限制与时效

- `CapabilityInputs` 增加 BMS 允许充/放点位、`max_age_ms`（默认 5000）、爬坡速率与死区（默认 10 kW/s、5 kW）。
- 能力保存 `charge_max_kw`/`discharge_max_kw`；`AllocateEnergy` 使用分方向能力与在线状态，旧数据回退到双向合并值。
- SOC/温度/BMS 样本必须带时间且未过期；SOC 过期双向关闭，单一 BMS 方向过期只关闭该方向，并在 `limit_reason` 说明。
- 执行边界再次校验能力与人工/互锁状态，防止柜内拆解放大到 PCS 额定值。
- `TestRefreshCapabilities` 覆盖分方向、单方向过期与 SOC 过期的双向关闭；`TestAllocateEnergyDirectionalCapabilityAndStatus` 覆盖充放上限与离线剔除。

### 3. 唯一仲裁与命令生命周期

- `EnergyScheduler` 每柜只调用一次 `RunUnitArbitratedDispatch`；站级策略（防逆流/需量/削峰填谷）只计算目标，不再直接写设备。
- 没有已发布单柜规则的柜体也能被站级策略控制；仲裁结果记录来源、优先级与执行阶段。
- 命令管理器支持单点写入、提交后新样本才可确认、参数指纹幂等、一次安全回退（上一个已确认值或 0）、取代语义与 `submitted`（未映射回读时不冒充确认）。内存订单有界，自动功率明细不写入命令历史以免淹没人工记录。
- 运行时支持注册非 Modbus dispatcher 并接收真实观测进入共用状态/历史。
- 测试：`command` 包新增旧样本不确认、参数冲突零写入、回退一次、取代不回退；`TestSchedulerDispatchesEveryUnit` 验证每柜一次且跳过停机单元。

### 4. 控制器人工操作

- 新增 `POST /energy-units/{id}/events`（start/stop/fault_clear/enter_maintenance/exit_maintenance）与 `PUT /energy-units/{id}/mode`。
- 启停必须有设备命令与回读确认；停机新增 `stopping` 中间态；没有命令运行时时明确返回 503，不假装成功。
- 人工模式立即停止自动目标，需显式恢复。
- 运行页每柜提供启动/停机/检修/故障确认/人工接管，所有动作二次确认并说明后果。
- `TestEnergyControlEvents` 覆盖人工模式、检修互锁、无运行时拒绝与故障解除；`TestStopWaitsForConfirmation` 覆盖停机确认。

### 5. 站级计划、预测与光储充

- Site 总目标按分方向能力分配到 Plant，再分配到 EnergyUnit，作为计划目标进入统一仲裁；1 秒内缓存，避免每柜重复计算。
- 光伏预测按配置的慢周期读取历史，写 `pvForecastKW` 供能力规则使用；无历史不写。
- `MixDispatch` 修正方向：超购电时不再限发光伏；外送偏多时先多充、再限发，并如实报告剩余缺口。

### 5.1 逐相判定

- PCC 绑定可配置 A/B/C 三相点位；三相都新鲜时才返回逐相判定，否则说明原因。
- 判定复用 `EvaluatePhase`：合相为零仍可识别单相反送，并明确执行器仅支持三相对称调节。
- 未配置独立分相执行点位，不宣称可分相调节。

### 6. 运维能力

- 审计保留可配置（单文件大小、备份个数、按天清理），查询与 CSV 包含仍保留的轮换文件，轮换后不静默丢历史。
- 通信质量新增最近 5 分钟/1 小时窗口与超时/断线/CRC/异常码/写失败分类，页面标注数据来自内存、重启归零。

### 7. OPC-UA 接入

- 完整 NodeID 解析（ns=/i=/g=/b=），读取保留源时间与状态；匿名轮询、断线重连、健康与最近错误。
- 产品节点 `opcua`/`opcuaPoint`，接入建模页可创建通道与测点；运行器把真实读数写回点位 value，并接入共享命令管理器与历史/规则时效检查。
- 测试：`TestParseNodeIDFormats`、`TestRunnerPollsAndWritesMappedPoints`、`TestRunnerReportsHealthAndReconnects`、`TestOpcuaRunnerPublishesValues`。未连接真实厂商服务器；证书/用户认证未实现，不宣称支持。

### 仍未完成（外部或明确未宣称）

- **M5 真机验收**：恩玖 PCS、BMS、电表、华为逆变器与四柜 800 kW 场景仍待现场签收。
- **MQTT 设备接入**：上游 MQTT 认证仅支持共享 token/用户 JWT，缺少按设备 topic ACL；在实现并验证凭据与 ACL 前不开放产品 MQTT 接入，避免以共享管理员 token 充当设备方案。下行命令与外部 broker 亦未宣称。
- **逐相独立调节**：已按配置的三相遥测给出判定与“仅对称调节”提示；独立分相执行点位仍未实现，不宣称可分相调节。
- **Profile/CSV 主入口合并与集中运行设置页**：现有入口可用，未做进一步合并。
- ARM64 目标工控机性能、品牌 A/B 更换、独立用户试用与公网部署仍需外部验收。
