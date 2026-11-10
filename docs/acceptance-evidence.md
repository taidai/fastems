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

## 结论

首版单柜储能的 9 个软件场景，仿真层逻辑全部验证通过、可复现。
**已定案**：SCH-02 分配策略 = 按比例水填充。
**待办**：真机验收（M5）覆盖对表/实测跟踪/长稳等物理相关场景。

## 2026-11-10 剩余缺口收口（软件侧）

证据来自 `go test`、前端生产构建与真实浏览器/容器验证；不代表 M5 真机签收。

### 1. 树范围授权与生产演示策略

- 新 EMS 路由（能源设备、规则/模拟、设备、能力来源、追溯、电站 PCC/策略、资源集群、站点目标、OPC-UA）统一使用 `h.node`/`h.allows`；列表按可见范围过滤，创建校验可见父节点与所有引用。
- `emsapi` 代理的 channels/state/history/quality/重启/创建/删除同样按节点可见性过滤；管理员也不能越过范围。
- `TestTreeScopeHTTPIsolation`（真实 HTTP，端口 9810/9811）证明站点 A 用户对站点 B 的读取、写入、发布、组调度全部 403，且无点位/追溯副作用。
- 生产新实例默认不生成演示站点；仅 `EASYEMS_SEED_DEMO=1` 显式启用，验收脚本已显式打开。

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
- **逐相独立调节**：三相判定纯函数已具备，但未接入真实三相遥测与独立执行点位；不宣称可分相调节。
- **Profile/CSV 主入口合并与集中运行设置页**：现有入口可用，未做进一步合并。
- ARM64 目标工控机性能、品牌 A/B 更换、独立用户试用与公网部署仍需外部验收。
