# OpenRemote：对 FastIOT 的可借鉴边界

## 五行结论
1. OpenRemote 是资产中心的 IoT 平台，不是轻量单服务组件；官方部署是多容器栈。[S1][S2]
2. Asset 属性经 Agent Link 接入协议，支持业务组织与通信分离，但不证明换设备后业务语义自动等价。[S3][S4][S5]
3. UI 可给资产实例加属性；官方当前文档明确类型模型仅支持 Java 配置，不能把实例配置等同于无代码发布可复用类型；此结论不外推为没有复制、导入等复用途径。[S3][S8][D1]
4. When-Then、Flow、Groovy 是三种规则形式；Flow 不能据此称为设备状态机。[S6]
5. 值写入不证明设备到位；核心为 AGPL-3.0-or-later，依赖另有条款，不能概括为整个部署统一许可。[S5][S9]

## 核验范围

核验日：2026-11-14。首轮为九项一手页面／源码、十三次获取尝试（含失败）；旧研究仅作线索。首轮滚动文档显示 **1.34.0**，未标页面修订日期。[S1]

同日定向复核复用首轮研究，仅重读模型、协议配置、Manager UI 三份官方文档与协议契约，并新增核对 `AgentLink`、`AbstractProtocol` 两份源码。文档站直连失败（连接／TLS），改读官方 `openremote/documentation` 仓库，固定于 `c57c4c260daf8fa5ebc32e7c851a9accd9a6a155`；它是文档源码快照，不宣称与线上已部署页面完全同步。[D1][D2][D3][S5][S10][S11]

GitHub 使用 `openremote/openremote`，API 返回 `openremote/core` 链接；源码固定于 `c7652131fe2b36f7b4341a0d8a20a94fc8107667`（2026-11-13 15:55:22 UTC），不是发布版本声明。[R] `master` 不存在，改查 `main`；许可证摘录一次失败后重取。未运行产品、安装依赖或连接设备；本地规格不代表实现验收。[M][C]

## 用户实际怎么用

以下为官方文档描述的操作路径，非本次实机验证；创建和编辑需要相应权限，公开演示站不提供完整管理权限。[D3]

1. 在 **Assets → +** 选已有类型或通用 `Thing`，创建资产并选父级；在 **Edit asset → Add attribute** 添加已知值类型的实例属性，配置单位、约束、历史和显示。[D1][D3]
2. 同样通过 **+** 创建 HTTP、MQTT 等 Agent，填写连接参数；在业务属性上加 **Agent link**，指定 Agent、协议参数及消息提取／值转换。部分协议可发现／导入资产，但不是所有协议都支持。[D2][D3]
3. 在 Assets 查看读数，对可写属性发送值；按需用 Rules 自动写属性／通知，用 Insights 选属性组成展示或控制面板。控制值提交与真实设备反馈须分辨，不能仅凭页面值变化验收到位。[D3][S5][S10][S11]

## 能力与证据边界

### 1. 定位和真实部署

**内建／官方方案：** Java Manager 管理资产状态；Docker Compose 组合 Manager、PostgreSQL、Keycloak、HAProxy，可选地图容器，也可按依赖顺序逐个启动 Docker 容器。数据库使用 PostGIS、TimescaleDB；不是“一个 Go 业务服务”。[S1][S2]

**宣传／未核验：** “唯一 100% 开源 IoT 平台”是官网表述，不是本报告确认的排他事实。所读页面未给完整宿主 OS 支持矩阵、离线安装包保证或性能实测，不从 Docker 推导这些结论。[S1][S2]

### 2. 模型与接入是否真正分开

**内建：** Asset 表示物理或概念对象，按父子层级组织；类型描述符约束命名属性，值描述符定义数据形状，metadata 控制连接、历史等行为。通用 `Thing` 不限制属性；Manager UI 能添加已知值类型的自定义属性，并配置单位、约束、格式等，但这属于资产实例配置，不等于发布可复用新类型。官方 `Asset Type Model` 明确写明当前仅支持 Java 配置类型模型，Manager UI 类型配置属于长期目标；这不是从“没找到页面”推断功能不存在，也不否认实例复制／导入等其他复用方式。[S3][S8][D1][D3]

Agent 本身也是 Asset，与协议实例一对一；其属性提供连接配置。业务属性通过 `agentLink` 指向 Agent，并使用协议所需链接类型；入站过滤／转换与出站转换分别配置。HTTP、MQTT、TCP 等已有实现；自定义类型需 Java，自定义协议需实现对应接口，属于代码扩展。[S3][S4][S5]

**判断：有条件支持分层。** 属性链接独立于资产层级。官方已有父子资产分组和群控方案：把资产放到父资产下，在父子上建同名同类型属性、启用 Rule state，并给每个父资产配置 `ChildAssetControl.groovy` 规则；不能说 OpenRemote 没有分组／群控。该方案不等于 FastIOT 的横向 Group、成员角色或可选 Unit，也不证明换机历史归属与能力等价检查；任意品牌无缝替换尤其没有证据。这些仍需按 FastIOT 需求单独验证。[S3][S5][D3][M]

### 3. 规则不是同一种逻辑模型

**内建：** When-Then 提供事件条件和动作；Flow 用于属性转换、连接、派生值；Groovy 提供脚本逻辑。规则可手动启停或调度，Global 与 Realm 有不同访问范围。[S6]

When-Then 属性条件需启用 Rule state，支持 AND／OR、持续时长、时钟／太阳事件、属性写入、通知、Webhook、创建报警。[S7] Manager 文档还描述触发频率，`Always` 并非条件持续为真时连续触发，而需先变假后再次触发；因此不能笼统说没有重触发语义。**未核验：** 这不证明跨事件去重、顺序确定性和完整审计保证；所读资料也未定义 Flow 的状态、守卫及转移语义，不能把它替代 FastIOT 决策表或状态机。[S6][S7][D3][C]

### 4. 页面、控制与实际状态

**内建：** Insights 有折线／柱状图、KPI、地图、表格及属性控制等组件；仪表盘可独立分享为应用，公开访问仍受资产／属性公开权限约束。移动控制台可访问仪表盘；自定义组件属于扩展。[S8]

协议契约区分用户写属性产生设备动作，与协议通过 `AttributeEvent` 更新观测值，但不能把所有属性更新都称为设备实测反馈。[S5] **源码补充：** `AgentLink.updateOnWrite` 明确表示“不等待协议响应，写入时立即更新属性”；`AbstractProtocol.processLinkedAttributeWrite` 在调用具体写入处理后，只要 Agent 或 Agent Link 任一开启该选项，就用转换后的写值调用 `updateLinkedAttribute`。该分支是本地写值回填，不是独立设备回读；两处选项缺省按 `false` 判断，也不保证所有具体协议或配置都不回填。[S10][S11]

另有可执行属性：写入 `REQUEST_START` 后，协议转换层会使用 Agent Link 的 `writeValue` 发起动作；这证明存在动作入口，不证明通用命令关联、设备确认或物理到位闭环。[D2] **未核验：** UI 提交、接口应答或单次属性变化，都不足以证明设备达到目标；本次证据不足以确认统一闭环保证，也不据此断言所有协议都没有确认机制。FastIOT 仍需独立目标、反馈与超时未知语义，不能让 Three.js 动画跟随按钮或写值回填冒充实态。[S5][S10][S11][C]

### 5. 历史、报警和事件

**内建：** 属性可配置持久化，资产页有历史区域；后台存在 `asset_datapoint` 历史存储与压缩清理机制，不只是展示样图。报警可由规则创建，带严重度、处理人、关联资产；UI 状态包括 Open、Acknowledged、In progress、Resolved、Closed。[S2][S7][S8]

**文档冲突：** Manager 页称默认保留一个月，运维页称默认不清理，由 `OR_DATA_POINTS_MAX_AGE_WEEKS` 全局控制并按七天 chunk 删除。本次不裁定实际默认值，也不承诺逐属性独立保留。[S2][S8]

协议属性事件及 WebSocket 事件订阅有文档；事件订阅不等于持久事件日志。缺样／质量保真、报警去重、确认与物理恢复隔离、导出及交付可靠性未在本次证据中闭环，不据此认定缺失或已满足。[S5][S7][S8]

### 6. 当前许可及适用范围

固定提交的 `LICENSE.txt` 开头明确：本程序适用 **GNU AGPL 第 3 版或后续版本**，无担保；同时声明软件汇集独立作品并列其许可证。因此核心许可不应外推到所有容器、依赖或自定义内容。[S9]

同文件收录 Timescale 条款，区分开源部分与 TSL 部分，并包含数据库服务限制、增值产品条件及向客户提供条款等要求；是否适用于具体交付须查实际组件版本与使用方式。本文不认定 FastIOT 付费 EMS 可以闭源或必须开源，也未核验另行商业授权，不替代法律审查。[S9]

## FastIOT：最多五项最小建议

以下区分研究建议及吸收结果；“已覆盖／已吸收”仅指规格，不等于已实现或验收通过。[M][C]

2026-11-14 经用户要求吸收：保留已有的业务／协议分离、可选 Unit、模板配置边界，不重复新增模块；在能力细则 T31-A 与总规格 T31 明确写值回填不算设备反馈，在两份规格 T32 补齐历史保留边界、清理粒度与查询／导出一致性验收。未照搬 OpenRemote 技术栈或新增脚本能力。

| 判断 | 用户价值／对应规格 | 最小采用与反例验收 | 依据 |
| --- | --- | --- | --- |
| 已覆盖 | 换地址不重画页面；物模型 §5、T28/T40 | 保留稳定业务引用，Profile 单独绑定；改地址后规则／画面引用不变，量纲不等价则拒绝绑定 | [S3][S4] |
| 已覆盖 | 普通传感器不承担能源复杂度；模型 §2–4 | 直接 Device 可用，按需 Unit；无 Unit 温度计可建模，展示 Group 不获得广播控制 | [S3][M] |
| 已覆盖 | AI 画面不误报成功；T30/T31 | 页面只调用已有命令接口并显示独立反馈；启动已应答但无运行反馈时显示未知，不转成运行 | [S5][S8] |
| 已覆盖 | 交付人员填参而非编程；T29/T30 | 坚持现有决策表／状态机及模板；需要任意 Groovy 的例子明确不支持，不偷偷塞后台脚本 | [S6][S7] |
| 已吸收：细化验收 | 用户可预测历史保存；T32 | 为现有保留设置补边界测试，不加新流程；设置到期后核对查询范围、清理粒度与说明一致，不能只看 UI 默认文字 | [S2][S8] |

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
- [S10] [AgentLink.java，固定提交](https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/model/src/main/java/org/openremote/model/asset/agent/AgentLink.java)：`updateOnWrite` 的含义。
- [S11] [AbstractProtocol.java，固定提交](https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/agent/src/main/java/org/openremote/agent/protocol/AbstractProtocol.java)：`processLinkedAttributeWrite` 与 `updateLinkedAttribute` 的写值回填路径。
- [D1] [Assets, Agents and Attributes，官方文档源码快照](https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/030-assets-agents-and-attributes.md)：`Asset Type Model` 明确的 Java 配置边界、描述符与 Thing。
- [D2] [Agents & protocols overview，官方文档源码快照](https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/040-agents-protocols/010-overview.md)：Agent Link、转换、可执行属性、协议特定发现／导入。
- [D3] [Manager UI，官方文档源码快照](https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/020-manager-ui/10-manager-ui.md)：创建资产／Agent、自定义属性、父子群控、触发频率、Insights。
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
[S10]: https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/model/src/main/java/org/openremote/model/asset/agent/AgentLink.java
[S11]: https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/agent/src/main/java/org/openremote/agent/protocol/AbstractProtocol.java
[D1]: https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/030-assets-agents-and-attributes.md
[D2]: https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/040-agents-protocols/010-overview.md
[D3]: https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/020-manager-ui/10-manager-ui.md
[R]: https://api.github.com/repos/openremote/openremote/commits/c7652131fe2b36f7b4341a0d8a20a94fc8107667
[M]: specs/2026-11-13-generic-thing-model.md
[C]: specs/2026-11-13-core-capabilities.md
