# OpenRemote：对 FastIOT 的可借鉴边界

## 五行结论
1. OpenRemote 是资产中心的 IoT 平台，不是轻量单服务组件；官方部署是多容器栈。[S1][S2]
2. Asset 属性经 Agent Link 接入协议，支持业务组织与通信分离，但不证明换设备后业务语义自动等价。[S3][S4][S5]
3. UI 可加属性；可复用自定义资产类型仍需 Java，不能称为完整无代码物模型。[S3][S8]
4. When-Then、Flow、Groovy 是三种规则形式；Flow 不能据此称为设备状态机。[S6]
5. 值写入不证明设备到位；核心为 AGPL-3.0-or-later，依赖另有条款，不能概括为整个部署统一许可。[S5][S9]

## 核验范围

核验日：2026-11-14。九项一手页面／源码，十三次获取尝试（含失败）；旧研究仅作线索。滚动文档显示 **1.34.0**，未标页面修订日期。[S1]

GitHub 使用 `openremote/openremote`，API 返回 `openremote/core` 链接；源码固定于 `c7652131fe2b36f7b4341a0d8a20a94fc8107667`（2026-11-13 15:55:22 UTC），不是发布版本声明。[R] `master` 不存在，改查 `main`；许可证摘录一次失败后重取。未运行产品、安装依赖或连接设备；本地规格不代表实现验收。[M][C]

## 能力与证据边界

### 1. 定位和真实部署

**内建／官方方案：** Java Manager 管理资产状态；Docker Compose 组合 Manager、PostgreSQL、Keycloak、HAProxy，可选地图容器，也可按依赖顺序逐个启动 Docker 容器。数据库使用 PostGIS、TimescaleDB；不是“一个 Go 业务服务”。[S1][S2]

**宣传／未核验：** “唯一 100% 开源 IoT 平台”是官网表述，不是本报告确认的排他事实。所读页面未给完整宿主 OS 支持矩阵、离线安装包保证或性能实测，不从 Docker 推导这些结论。[S1][S2]

### 2. 模型与接入是否真正分开

**内建：** Asset 表示物理或概念对象，按父子层级组织；类型描述符约束命名属性，值描述符定义数据形状，metadata 控制连接、历史等行为。通用 `Thing` 不限制属性；Manager UI 能添加自定义属性，但不等于定义可复用新类型。[S3][S8]

Agent 本身也是 Asset，与协议实例一对一；其属性提供连接配置。业务属性通过 `agentLink` 指向 Agent，并使用协议所需链接类型；入站过滤／转换与出站转换分别配置。HTTP、MQTT、TCP 等已有实现；自定义类型需 Java，自定义协议需实现对应接口，属于代码扩展。[S3][S4][S5]

**判断：有条件支持分层。** 属性链接独立于资产层级，但不证明可选 Unit、横向 Group、成员角色、换机历史归属及能力等价检查；任意品牌无缝替换尤其没有证据。这些仍由 FastIOT 模型承担。[S3][S5][M]

### 3. 规则不是同一种逻辑模型

**内建：** When-Then 提供事件条件和动作；Flow 用于属性转换、连接、派生值；Groovy 提供脚本逻辑。规则可手动启停或调度，Global 与 Realm 有不同访问范围。[S6]

When-Then 属性条件需启用 Rule state，支持 AND／OR、持续时长、时钟／太阳事件、属性写入、通知、Webhook、创建报警。[S7] **未核验：** 所读页面未明确重触发去重、顺序确定性和完整审计保证，也未定义 Flow 的状态、守卫及转移语义；不能把它替代 FastIOT 决策表或状态机。[S6][S7][C]

### 4. 页面、控制与实际状态

**内建：** Insights 有折线／柱状图、KPI、地图、表格及属性控制等组件；仪表盘可独立分享为应用，公开访问仍受资产／属性公开权限约束。移动控制台可访问仪表盘；自定义组件属于扩展。[S8]

协议契约区分用户写属性产生设备动作，与协议通过 `AttributeEvent` 更新观测值。[S5] **未核验：** UI 提交、接口应答或单次属性变化，都不足以证明设备达到目标；所读资料没有统一的命令关联、设定确认和物理到位保证。FastIOT 仍需独立目标、反馈与超时未知语义，不能让 Three.js 动画跟随按钮冒充实态。[S5][S8][C]

### 5. 历史、报警和事件

**内建：** 属性可配置持久化，资产页有历史区域；后台存在 `asset_datapoint` 历史存储与压缩清理机制，不只是展示样图。报警可由规则创建，带严重度、处理人、关联资产；UI 状态包括 Open、Acknowledged、In progress、Resolved、Closed。[S2][S7][S8]

**文档冲突：** Manager 页称默认保留一个月，运维页称默认不清理，由 `OR_DATA_POINTS_MAX_AGE_WEEKS` 全局控制并按七天 chunk 删除。本次不裁定实际默认值，也不承诺逐属性独立保留。[S2][S8]

协议属性事件及 WebSocket 事件订阅有文档；事件订阅不等于持久事件日志。缺样／质量保真、报警去重、确认与物理恢复隔离、导出及交付可靠性未在本次证据中闭环，不据此认定缺失或已满足。[S5][S7][S8]

### 6. 当前许可及适用范围

固定提交的 `LICENSE.txt` 开头明确：本程序适用 **GNU AGPL 第 3 版或后续版本**，无担保；同时声明软件汇集独立作品并列其许可证。因此核心许可不应外推到所有容器、依赖或自定义内容。[S9]

同文件收录 Timescale 条款，区分开源部分与 TSL 部分，并包含数据库服务限制、增值产品条件及向客户提供条款等要求；是否适用于具体交付须查实际组件版本与使用方式。本文不认定 FastIOT 付费 EMS 可以闭源或必须开源，也未核验另行商业授权，不替代法律审查。[S9]

## FastIOT：最多五项最小建议

以下是建议，不修改规格；“已覆盖”不等于已实现。[M][C]

| 判断 | 用户价值／对应规格 | 最小采用与反例验收 | 依据 |
| --- | --- | --- | --- |
| 已覆盖 | 换地址不重画页面；物模型 §5、T28/T40 | 保留稳定业务引用，Profile 单独绑定；改地址后规则／画面引用不变，量纲不等价则拒绝绑定 | [S3][S4] |
| 已覆盖 | 普通传感器不承担能源复杂度；模型 §2–4 | 直接 Device 可用，按需 Unit；无 Unit 温度计可建模，展示 Group 不获得广播控制 | [S3][M] |
| 已覆盖 | AI 画面不误报成功；T30/T31 | 页面只调用已有命令接口并显示独立反馈；启动已应答但无运行反馈时显示未知，不转成运行 | [S5][S8] |
| 已覆盖 | 交付人员填参而非编程；T29/T30 | 坚持现有决策表／状态机及模板；需要任意 Groovy 的例子明确不支持，不偷偷塞后台脚本 | [S6][S7] |
| 新建议：细化验收 | 用户可预测历史保存；T32 | 为现有保留设置补边界测试，不加新流程；设置到期后核对查询范围、清理粒度与说明一致，不能只看 UI 默认文字 | [S2][S8] |

## 采用／不照搬

**采用：** 语义与接入绑定分开、描述驱动展示、历史与报警以数据证据验收。[S3][S4][S5][S8]

**不照搬：** Java／Keycloak 栈、Realm 导航、Groovy 或 Flow 画布。[S2][S6] 研究建议保持已批准的开源核心＋付费 EMS、单 Go 业务服务／离线 Docker、可选 Unit 与 Site/Area/Group/Device/Point、DeepSeek 网页／Three.js；不新增巡检、升级恢复或“加入 EMS”流程。

## 一手来源与本地对照

- [S1] [Introduction](https://docs.openremote.io/docs/introduction/)：定位、公开声明。
- [S2] [System administration](https://docs.openremote.io/docs/developer-guide/system-administration/)：容器、数据库、保留策略。
- [S3] [Assets, Agents and Attributes](https://docs.openremote.io/docs/user-guide/assets-agents-and-attributes/)：模型及 Java 扩展。
- [S4] [Agents & protocols overview](https://docs.openremote.io/docs/user-guide/agents-protocols/overview/)：链接与转换。
- [S5] [Protocol.java，固定提交](https://github.com/openremote/openremote/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/model/src/main/java/org/openremote/model/asset/agent/Protocol.java)：尤其 38–96、245–270 行。
- [S6] [Create rules](https://docs.openremote.io/docs/user-guide/rules-and-forecasting/create-rules/)：三类规则。
- [S7] [When-Then rules](https://docs.openremote.io/docs/user-guide/rules-and-forecasting/when-then-rules/)：条件、动作、报警。
- [S8] [Manager UI](https://docs.openremote.io/docs/user-guide/manager-ui/)：属性编辑、Insights、历史、报警。
- [S9] [LICENSE.txt，固定提交](https://github.com/openremote/openremote/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/LICENSE.txt)：开头适用声明与所附依赖条款。
- [R] [提交元数据](https://api.github.com/repos/openremote/openremote/commits/c7652131fe2b36f7b4341a0d8a20a94fc8107667)。
- [M] [FastIOT 通用物模型](specs/2026-11-13-generic-thing-model.md)。
- [C] [FastIOT 平台能力细则](specs/2026-11-13-core-capabilities.md)。

[S1]: https://docs.openremote.io/docs/introduction/
[S2]: https://docs.openremote.io/docs/developer-guide/system-administration/
[S3]: https://docs.openremote.io/docs/user-guide/assets-agents-and-attributes/
[S4]: https://docs.openremote.io/docs/user-guide/agents-protocols/overview/
[S5]: https://github.com/openremote/openremote/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/model/src/main/java/org/openremote/model/asset/agent/Protocol.java
[S6]: https://docs.openremote.io/docs/user-guide/rules-and-forecasting/create-rules/
[S7]: https://docs.openremote.io/docs/user-guide/rules-and-forecasting/when-then-rules/
[S8]: https://docs.openremote.io/docs/user-guide/manager-ui/
[S9]: https://github.com/openremote/openremote/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/LICENSE.txt
[R]: https://api.github.com/repos/openremote/openremote/commits/c7652131fe2b36f7b4341a0d8a20a94fc8107667
[M]: specs/2026-11-13-generic-thing-model.md
[C]: specs/2026-11-13-core-capabilities.md
