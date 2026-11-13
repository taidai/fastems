# 设备、模型与 EMS：一手资料比较

研究日期：2026-11-13。项目上下文为 [能力细则](specs/2026-11-13-core-capabilities.md) 与 [术语表](../GLOSSARY.md)，它们是需求，不是行业共识证据。下文区分**规范、产品实践、建议**；选取的成熟实践不代表市场占有率调查。

## 一、连接、点位与工程复用

**典型任务：** 导入厂家点表，调通两台设备，核对原值与工程值，再复用到第三台而不重建画面。

**规范／实践：** OPC UA 将对象表达为变量、方法及关联，而非地址集合；类型与实例通过 `HasTypeDefinition` 联系。[1][2] Data Access 的 `AnalogItemType` 要求 `EURange`，`EngineeringUnits` 则是可选；工程范围不等于仪表极限，超工程范围的写入也不保证统一拒绝。[3] 因而“有单位／范围字段”不等于已经具备安全写入语义。

Ignition 8.1 用 UDT 定义复用标签结构，实例承担实际采集；参数替换 OPC 路径，局部属性可覆盖，定义修改会传播至实例。[4][5] 这是产品复用模式，并非 OPC UA 要求。其缺失参数可能保留为字面文本，不能照搬成 FastIOT 的成功绑定；8.1.8、8.1.13、8.1.17 的相关参数行为也有差别。[5]

**最小采纳建议：** 分开连接、设备实例、点位身份和地址映射；Profile 负责地址、编码、缩放、单位及枚举映射。模板保存公共定义，实例填写站号等差异并显示覆盖项；业务引用稳定成员 ID。CSV 导入预览错误和变化。按本项目要求固定生效版本，不能照搬 UDT 的自动传播。

**差异与限制：** “符合规范”和“满足项目要求”应分开判断：规范允许单位缺省，不妨碍 FastIOT 对参与运算的点位要求单位明确。建议采集值、最后有效值、质量和时间来源分别表达；协议没有设备时间戳时，显示平台采集时间，不补造设备时间。这是项目契约，不是上述规范字段的机械复制。

**暂不建设：** 全协议接入、任意表达式语言、多层模板自动合并。

**验收例：** 两台压力设备共用模板、站号不同；原值 123 经 0.1 缩放显示 12.3 bar。复制后地址独立，旧画面引用不变；错误类型、单位或未解析参数在编辑处报错，离线值不变成零。

## 二、Device、模型、模板与功能对象

**典型任务：** 同型设备批量建模；更换厂家后保留业务引用；把 PCS、BMS 等协作设备组织成一个可调度对象。

**规范／实践：** OPC UA 的类型可声明组件及实例化规则，支持复合结构，但未规定所有业务对象必须命名为 Unit。[2] OpenEMS 以 Nature 接口和 Channel 描述设备契约，例如 `SymmetricEss`、`ManagedSymmetricEss`，将硬件差异与控制逻辑分开；这是该项目架构，不是工业标准。[6] ISA88 公共概述讨论批处理物理与功能模型，但该页没有给出 Unit 定义，不能据此把 Unit 推广为任意行业对象；本研究未读取付费全文。[10]

**独立比较与建议：**

| 选择 | 收益／代价 | 当前判断 |
| --- | --- | --- |
| 保留 EnergyUnit，底座使用 Device＋模型／模板 | 能源调度与通用设备边界清楚；跨行业组合对象以后另证需求 | 最小可行，优先 |
| 改为通用 Unit，再附能力 | 可承载跨设备功能对象；须解释身份、成员、控制所有权及生命周期 | 仅改名没有收益，暂不采纳 |
| 所有对象都用 Device | 对简单单机直接；复合资源会混淆物理资产与调度对象 | 不适合替代现有能源组合关系 |

建议明确：Device 是有独立身份的实际／模拟设备实例；模型定义业务成员语义；模板是模型与默认配置的复用载体；Profile 是厂商映射。EnergyUnit 可以绑定多个 Device，也不按柜体数量推定可独立控制数量。泛化能力描述不等于现在就建立通用 Unit 实体或能力市场。

**换品牌必须分三层：**

1. **协议映射：** 地址、字节序、缩放和枚举可转换，只证明值能正确传输。
2. **能力等价：** 核对观测／设定、AC／DC、方向、范围、必需成员、模式和确认方式。SunSpec 124 的 `InWRte/OutWRte` 是相对最大充放电率的百分比，802 的额定充放电率却是 DC W；名称近似并不等价。[8][9]
3. **物理重新验证：** 核对实际方向、响应、互锁和反馈；这是本项目建议，映射和仿真不能证明现场硬件等价。

建议把“支持调功”拆成可验证契约：控制目标是什么、允许范围来自哪里、需要何种模式、接受请求后以何种反馈确认。观测能力与控制能力分别声明；设备额定存在某功能，不等于此刻可用，更不等于操作员已有权限。两个设备组合也不能仅靠成员清单自动获得新的控制能力。

**暂不建设：** 自动推断任意品牌能力、无损自动替换承诺。

**验收例：** A 泵运行位换成 B 泵枚举后，`running` 引用不变；B 无启动反馈时明确降级结果证明范围，不能仍报“已启动”。能源设备仅有遥测时不得继承功率控制按钮。

## 三、EMS 模型与可配置策略

**典型任务：** 绑定储能、PCC 电表和可调资源，填写计划、限值与 SOC 范围，理解目标为何受限。

**规范／实践：** SunSpec 124 是 Basic Storage Controls，802 是 Battery Base Model；802 分别表达额定充放电率、SOC、状态和操作，并包含必需、可选及条件性字段。[8][9] 它们提供设备数据／控制语义，不是 FastIOT 的 Site—Plant—ResourceGroup 拓扑规范；模型编号也不是版本号。

OpenEMS 采用 Controller＋Scheduler 组合控制，调度顺序及功率约束影响后续控制器，而非任意写入者“最后覆盖”。[6] 其对称削峰配置明确绑定 `ess_id`、`meter_id`、`peakShavingPower`、`rechargePower`。[7] 这是可配置算法实例的证据，但该配置只说明购电功率阈值，不能据此声称实现计费周期需量、96 点计划或任意光储充优化。

**最小采纳建议：** EMS 付费内容包提供能源模型、能力约束和已有算法模板，共用开源底座的点位、命令与界面绑定。每个模板声明必需输入、作用对象、反馈表计、单位方向、可调参数及失败条件；额定能力与实时允许能力分开。先配置已批准的计划、需量、防逆流及分配内容；需量必须另明确窗口、时间口径与缺样规则，不能用瞬时削峰冒充。

**差异与限制：** 建议只把算法已经支持且边界明确的参数开放给用户；改变反馈点、资源归属或计算口径属于工程修改，不应伪装成普通运行调参。监视型光伏可以参与能流展示，却不能默认参与限发；充电功率上限与储能双向功率目标也不应共用一个含义不明的“功率设置”。

**暂不建设：** 通用优化语言、任意算法自动生成、离网／黑启动；不照搬 OpenEMS 的 Java／OSGi 运行架构，也不新增独立交付检查、升级恢复或“加入 EMS”流程。维持单 Go／离线 Docker 的开源底座与付费内容边界。

**验收例：** 计划请求放电 50 kW，当前允许仅 20 kW，显示请求、受限目标、实测和来源；PCC 过期则不授权依赖该反馈的新动作。换成只支持百分比限幅的 Profile 时，若不能证明与功率目标等价，拒绝该绑定，而非伪装兼容。

## 一手来源与版本边界

以下 10 项均为官方规范参考、产品文档或官方仓库；未以第三方文章补证。检索约 16 次 HTTP/API 请求（含目录、版本定位与失败），未下载或执行外部代码。SunSpec 官网返回 403、OpenEMS 旧控制器页面／旧路径返回 404，相关论据改用成功读取的官方源码。

1. **OPC Foundation，Part 3 §4.3 Object Model**：[公开参考](https://reference.opcfoundation.org/Core/Part3/v105/docs/4.3)，检索页面显示 v1.05.06。
2. **OPC Foundation，Part 3 §4.6 TypeDefinitionNodes**：[类型、复合声明、实例关系](https://reference.opcfoundation.org/Core/Part3/v105/docs/4.6)，v1.05.06。
3. **OPC Foundation，Part 8 §5.3.2 AnalogItem Types**：[单位与范围语义](https://reference.opcfoundation.org/Core/Part8/v105/docs/5.3.2)，页面显示 v1.05.07；不能把 `v105` 路径当成冻结补丁版本。
4. **Inductive Automation，Ignition 8.1 UDTs**：[定义、实例、覆盖与修改传播](https://www.docs.inductiveautomation.com/docs/8.1/platform/tags/user-defined-types-udts)，不是 8.3 文档。
5. **Inductive Automation，Ignition 8.1 UDT Parameters**：[地址参数及版本差异](https://www.docs.inductiveautomation.com/docs/8.1/platform/tags/user-defined-types-udts/udt-parameters)。
6. **OpenEMS，Edge Architecture §§3–9**：[Controller、Scheduler、Nature、Channel](https://openems.github.io/openems.io/openems/latest/edge/architecture.html)，`latest` 为滚动文档，非固定发行版。
7. **OpenEMS，`controller.symmetric.peakshaving/Config.java`**：[配置源文件](https://github.com/OpenEMS/openems/blob/develop/io.openems.edge.controller.symmetric.peakshaving/src/io/openems/edge/controller/symmetric/peakshaving/Config.java)，读取时 Git blob `9892106de5de44071bc44734f14958206f584510`；不代表已核验算法实现。
8. **SunSpec Alliance，`json/model_124.json`**：[模型 124](https://github.com/sunspec/models/blob/90b4a331dcca1d6eac69c1bead952fddcc5852e0/json/model_124.json)。
9. **SunSpec Alliance，`json/model_802.json`**：[模型 802](https://github.com/sunspec/models/blob/90b4a331dcca1d6eac69c1bead952fddcc5852e0/json/model_802.json)。两文件读取自默认分支，随后记录 HEAD `90b4a331dcca1d6eac69c1bead952fddcc5852e0`；文件无顶层版本字段，不能自行标为某次规范发行或宣称认证兼容。
10. **ISA，ISA88 Committee 公共概述**：[批处理标准工作范围](https://www.isa.org/standards-and-publications/isa-standards/isa-standards-committees/isa88)，仅引用公开说明，未获取标准全文。
