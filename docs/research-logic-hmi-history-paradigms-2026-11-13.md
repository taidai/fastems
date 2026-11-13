# FastIOT：逻辑、组态、报警与历史的成熟范式

研究日期：2026-11-13。目标是让人能制作、操作并追查设备应用，不是堆管理后台。需求来自 [core-capabilities 第 4–7 节](specs/2026-11-13-core-capabilities.md)，仅作需求依据；下列事实来自 11 份官方规范／产品文档。“建议／验收”是 FastIOT 的设计选择，非标准要求或现有实现声明。

## 1. 决策表：判断条件，不等于执行流程

**真实任务：** 工程人员填写液位、运行条件及输出；操作员理解为什么命中、最终设备是否响应。

**成熟模式与差异：** DMN 将输入测试组合成规则匹配，再按命中策略产生输出；它不是设备动作执行成功的证明。[1] DMN 1.3 默认是 **Unique**（规则不得重叠），并非 FIRST；Any 允许重叠但要求输出相同，Priority 按输出优先级选择，First 才按行序取首条。多命中还有 Rule order、Output order、Collect；Collect 可求和、最小、最大或计数。规范特别警告 FIRST 的逻辑依赖行序，人工验证更困难。[1]

**最小建议／避免：** 保留当前 FIRST 产品选择，但明确标注，显示被遮蔽的匹配行、未命中结果。持续、冷却作为显式平台时序条件，不冒称 DMN 原生策略。决策输出交给统一命令入口；不做全套 FEEL、流程画布或任意脚本，不宣称完整 DMN 兼容。

**可观察验收：** 两行同时满足时，只采用第一行；调换行序，模拟结果随之改变并解释原因。模拟零真实写入，运行中“命中”与“设备反馈成功”分开记录。

## 2. 状态机：表达过程记忆，不是连线编程

**真实任务：** 配置停止→等待启动反馈→运行，以及拒动、超时／未知分支。

**成熟模式与差异：** SCXML 1.0 用当前状态、事件和守卫决定转移；执行有退出、转移动作、进入动作的顺序，并在处理下一外部事件前完成当前内部处理。其转移冲突有层级及文档顺序规则，也支持嵌套、并行状态。[2] 因而状态图应有这些明确语义，不能只把任意计算节点接起来便称状态机。

**最小建议／避免：** 先做平面状态、初始状态、事件、守卫、动作与超时；约定同源竞争转移的显式优先级或报歧义，不能照数据库遍历顺序执行。所有动作复用平台控制入口；先不做并行嵌套、任意代码和审批流。借鉴语义不等于实现 SCXML。

**可观察验收：** 点击启动仅进入等待；运行反馈才允许进入运行。延迟反馈、坏质量、超时分别可见；重新连接不重发旧命令，也不凭保存的状态宣称设备仍在运行。

## 3. HMI：用户可操作应用，AI 只是制作方式

**真实任务：** 在同一设备画面看实时值、质量、报警、趋势，选择允许的启停或设定操作并看到实际结果。

**成熟模式与差异：** Perspective 8.1 用视图、容器、组件及数据绑定构建监视与控制界面。[3] Tag binding 默认只读，双向绑定才产生写请求，且有写回确认等待与质量覆盖显示；这不等于证明机械过程已经完成。[4] 事件动作可设权限，按钮禁用表达式只是视觉反馈，不改变 Gateway 安全配置；Tag 还具有读写权限、只读标志和有效 CanWrite 判断。[5][6]

**最小建议／避免：** AI 生成网页／Three.js 交互属于本项目采用的**新兴制作方法**，不是上述文档已经证明的行业通行做法；本轮不对其成熟性作普遍结论。生成物只绑定模型数据与受控命令，不承担自主实时控制。2D／3D 共用可信确认入口、服务端鉴权、参数与控制权校验；设备保护／互锁不得由按钮权限替代。默认任务画面，不强迫进入命令管理后台；不让生成页拿设备凭据、直接协议写口或另写控制逻辑。

**可观察验收：** 操作员从生成页启动指定设备，核对目标、参数和真实反馈；无权请求即使绕过按钮仍被拒绝。预览零真实写入；刷新、重连不发命令；断开 AI 与外网，已部署本地页面仍可授权操作。

## 4. 报警／事件：接手与恢复是两个维度

**真实任务：** 定位“启动失败”，确认接手，查看异常是否仍在，再追查整个发生过程。

**成熟模式与差异：** OPC UA Part 9 分别定义 AckedState、可选 ConfirmedState，以及表示异常存在的 ActiveState。[7][8] Ignition 同样区分活动／清除与确认／未确认四种组合；确认不表示触发条件结束。当前状态表与记录状态变化的 Alarm Journal 分工不同，清除后再次发生产生新的报警事件。[9] OPC UA 还区别操作员临时搁置、抑制与维护停用；这些不能混成“设备正常”。[8]

**最小建议／避免：** 用同一报警过程展示发生、接手、恢复及相关命令；普通状态事件不强制走人工确认。持续异常不按轮询重复新建；新一轮异常另立身份。若提供搁置，记录对象、期限和原因，不借隐藏报警取消互锁。先不做云消息中心、复杂通知流程或强制二次确认。

**可观察验收：** 故障未消失时确认，仍显示“活动／已确认”；设备反馈恢复后更新恢复时间，保留处理人及说明。再次故障有新的过程记录，能从报警跳到设备与当时命令。

## 5. 历史／趋势：查询结果不一定是实测样本

**真实任务：** 对比故障前后的液位、命令与反馈，识别缺样，导出同一范围供复核。

**成熟模式与差异：** OPC UA Historical Access 的 raw read 返回已存储值、质量与时间戳；存储可能已压缩，raw 并不保证每个采样都保留。修改历史另有读取方式，边界值可供客户端插值。[10] Ignition queryTagHistory 支持按变化、自然或固定间隔查询，以及聚合、插值、边界值和坏质量过滤；平均还区分时间加权与算术平均。因此曲线上的值不能无条件标为原始测量。[11]

**最小建议／避免：** 默认让用户选设备、点位、时间段，明确原始记录／聚合／插值及单位、质量、时间来源；缺样画缺口，不补零。导出沿用查询范围与授权；报警、规则命中、状态变化关联当时版本和参数。不先做任意 BI 或另建日志后台。

**可观察验收：** 注入一段断线后查询并导出，缺口、质量和范围一致；切换聚合显示口径，插值明确标注而非冒充观测；无数据、查询失败与无权限分别呈现。

## 证据范围

本轮没有读取 ISA 付费标准，也不作 ISA、DMN、SCXML 或 OPC UA 合规承诺。DMN 1.5 PDF 检索受大小限制，改为实读官方 **1.3（2021-03）**相关章节，不将旧版证据包装成最新版。Three.js 教程地址检索失败，未据此宣称其具有工业控制／授权能力；AI 方法为产品建议而非规范事实。不新增独立检查、升级或恢复模块。

## Sources

[1] OMG，[Decision Model and Notation 1.3](https://www.omg.org/spec/DMN/1.3/PDF)，§8.2.10、§10.3.2.10，印刷页 72–74、114–116；官方 PDF 实读。

[2] W3C，[SCXML 1.0 Recommendation](https://www.w3.org/TR/scxml/)，2015-09-01；§3.3–3.9、§3.13、§4.9–4.10。

[3] Inductive Automation，[Perspective](https://www.docs.inductiveautomation.com/docs/8.1/ignition-modules/perspective)，Ignition 8.1；Views and Containers、Components、Bindings。

[4] Inductive Automation，[Tag Bindings in Perspective](https://www.docs.inductiveautomation.com/docs/8.1/ignition-modules/perspective/working-with-perspective-components/bindings-in-perspective/tag-bindings-in-perspective)，8.1；Options、Indirect Tag Binding – Bidirectional。

[5] Inductive Automation，[Security in Perspective](https://www.docs.inductiveautomation.com/docs/8.1/ignition-modules/perspective/security-in-perspective)，8.1；Event Actions on Perspective Components、组件授权绑定说明。

[6] Inductive Automation，[Tag Properties](https://www.docs.inductiveautomation.com/docs/8.1/platform/tags/tag-properties)，8.1；readPermissions、writePermissions、readOnly、CanWrite；后者自 8.1.8 引入。

[7] OPC Foundation，[Part 9 — AcknowledgeableConditionType](https://reference.opcfoundation.org/Core/Part9/v105/docs/5.7.2)，页面版本 1.05.06，§5.7.2、表 32。

[8] OPC Foundation，[Part 9 — AlarmConditionType](https://reference.opcfoundation.org/Core/Part9/v105/docs/5.8.2)，页面版本 1.05.06，§5.8.2。

[9] Inductive Automation，[Alarming](https://www.docs.inductiveautomation.com/docs/8.1/platform/alarming)，8.1；Alarm Events / Conditions / States、Viewing Alarm History。

[10] OPC Foundation，[Part 11 — ReadRawModifiedDetails](https://reference.opcfoundation.org/Core/Part11/v105/docs/6.4.3)，页面版本 1.05.04；Read raw functionality、Read modified functionality。链接编号与当前正文编排可能不同，以章节标题为准。

[11] Inductive Automation，[system.tag.queryTagHistory](https://www.docs.inductiveautomation.com/docs/8.1/appendix/scripting-functions/system-tag/system-tag-queryTagHistory)，8.1；Description、Syntax / Parameters。
