# IoT 平台能力基准与 FastIOT 取舍

> **历史调研（2026-11-03 至 2026-11-04）**：本文以 Home Assistant / ZIoT 时期的问题为起点整理外部平台对比，仅用于能力边界参考。FastIOT 当前运行不依赖 Home Assistant；产品目标与验收以 `docs/product-spec-v1.md`、`docs/m4-api.md` 和 `GOAL.md` 为准。

> 2026-11-04 更新：产品目标已重新确认为光储充 EMS 底座，决策表为首要能力。本文关于 Simple IoT 能力边界与复用取舍的调研仍然有效；其中“不做可配置画面”等早期收敛建议不再代表最终产品边界。

## 调研口径

- 资料访问与本地源码核对日期：**2026-11-03 至 2026-11-04**。外部平台优先采用官方文档、官方仓库和官方源码，不以二手文章作为能力依据；本地结论以固定于 **Simple IoT v0.31.0** 的源码及 FastIOT 当前实现为准（`go.mod:5-10,74`，`third_party/README.md:1-14`，`third_party/simpleiot/CHANGELOG.md:12-15`）。
- 本文本地相对路径均以 `C:\Users\chent\Desktop\2026成果\AAAAA开源\FastIOT` 为根目录。
- “未从官方资料确认”只表示本次未找到官方证据，**不等同于断定平台没有该能力**。“上游有源码/有客户端”也不等于 FastIOT 已注册、开放或验收该能力。
- 外部平台版本以官方 Release 或文档页明确标识为准；指向 `main`/`master` 的源码链接是访问时的分支快照，不视为固定版本。商业能力不并入社区/开源版结论。

## 结论先行

1. **不照搬综合平台。** OpenRemote 的 Realm、规则、告警、仪表盘和边缘体系完整，但标准部署包含 Manager、PostgreSQL/TimescaleDB、Keycloak、Proxy 等；ThingsPanel 的物模型、模板和通知更贴近成品平台，但 All-in-One 仍是多服务 Compose，且社区版明确为单租户；Hummingbird 的多项高级能力属于镜像/企业版。它们适合用来校验能力边界，不适合作为 FastIOT 的功能清单（证据见下文各平台“部署架构”和版本边界）。
2. **Simple IoT 最匹配 FastIOT 的不是功能数量，而是单进程 Node/Point + NATS/JetStream 内核。** FastIOT 当前显式注册的客户端管理器只有上游 Modbus 与 FastIOT 自有决策表两类；其他协议、上游规则、外置数据库、同步、通知和宿主管控均未因源码存在而自动启用（`cmd/fastiot/main.go:36-62`，`third_party/simpleiot/client/client.go:26-101`）。
3. **当前主要差距不是“再做一个物模型/规则画布/多租户”，而是把已有内核变成可用、可查、可恢复的产品闭环。** 历史读取已有后端 API，但当前 React 页面没有历史/趋势入口；告警数据结构与监视器已有源码，但监视器未注册、HTTP 路由未挂载，不能算已交付告警中心（`internal/appapi/history.go:12-85`，`frontend/src/App.tsx:62,154-196`，`internal/alerts/monitor.go:74-105`，`internal/appapi/handler.go:44-125`）。
4. **通用节点/API、任意同步、宿主更新与网络配置等不应为了“榨干上游”而开放。** 上游自己的安全文档明确：在默认客户端模型下，账号可在其组内触发更新、NTP、NetworkManager、同步等宿主级副作用；FastIOT 的产品边界恰恰应阻断这类绕过（`third_party/simpleiot/docs/ref/security.md:53-87`，`cmd/fastiot/main.go:43-51`）。

## 外部平台基准

### OpenRemote

**项目身份、版本与授权。** 官方核心仓库为 [openremote/core](https://github.com/openremote/core)，官方将其描述为“100% open source IoT platform”。访问时最新 Release 与当前文档均为 **1.33.0**（[Release](https://github.com/openremote/core/releases/tag/1.33.0)、[1.33.0 文档](https://docs.openremote.io/docs/introduction/)）。核心代码采用 [AGPLv3](https://github.com/openremote/core/blob/main/LICENSE.txt)。

**开源与商业边界。** 官方开放源码页说明核心平台为 AGPLv3，同时提供付费开发/三线支持、托管及不希望遵循 AGPLv3 时的商业许可；未据此推断存在另一套闭源核心功能（[官方说明](https://openremote.io/open-source/)）。

1. **设备/物模型**：Asset 表示物理或逻辑对象，Attribute 可保存任意可 JSON 表示的值；模型由 Asset、Attribute、Meta-item、Value descriptors 描述，资产可组成层级树。当前自定义资产模型需要编写 Java，而不是只在 Manager UI 中配置（[Assets, Agents and Attributes](https://docs.openremote.io/docs/user-guide/assets-agents-and-attributes/)、[默认模型源码目录](https://github.com/openremote/core/tree/main/model/src/main/java/org/openremote/model/asset/impl)）。
2. **协议接入**：Agent 将外部设备或服务接入 Manager；官方列出 MQTT、HTTP、TCP、UDP、WebSocket、Modbus、KNX、Z-Wave、SNMP、Serial、ChirpStack、The Things Stack 等。`AgentLink` 连接资产属性与协议，并支持入站/出站转换；自定义协议可围绕 Agent/Protocol 接口扩展（[协议总览](https://docs.openremote.io/docs/user-guide/agents-protocols/overview/)、[Agent.java](https://github.com/openremote/core/blob/main/model/src/main/java/org/openremote/model/asset/agent/Agent.java)、[Protocol.java](https://github.com/openremote/core/blob/main/model/src/main/java/org/openremote/model/asset/agent/Protocol.java)）。
3. **规则**：提供 When-Then、Flow、Groovy 三类规则。When-Then 面向事件条件与动作，Flow 用于属性转换/联动，Groovy 用于复杂脚本；Realm 规则只能访问本 Realm 资产，Global 规则可跨 Realm（[Create Rules](https://docs.openremote.io/docs/user-guide/rules-and-forecasting/create-rules/)）。When-Then 动作还包括通知、Webhook 和属性写入（[When-Then Rules](https://docs.openremote.io/docs/user-guide/rules-and-forecasting/when-then-rules/)）。
4. **告警**：When-Then 规则可生成带处理人、严重级别、状态、标题、消息及关联资产的告警（[When-Then Rules](https://docs.openremote.io/docs/user-guide/rules-and-forecasting/when-then-rules/)）。Manager UI 提供 Open → Acknowledged → In progress → Resolved → Closed 状态流转、筛选和分派查看；高严重级别告警可邮件通知处理人（[Manager UI：Alarms](https://docs.openremote.io/docs/user-guide/manager-ui/#alarms)）。
5. **历史/存储**：属性的 `storeDataPoints` 开启历史数据点；平台使用 PostgreSQL 与 TimescaleDB（[资产模型](https://docs.openremote.io/docs/user-guide/assets-agents-and-attributes/)、[系统管理](https://docs.openremote.io/docs/developer-guide/system-administration/)）。历史数据默认无限期保留，`OR_DATA_POINTS_MAX_AGE_WEEKS` 配置全局保留周数，清理按 TimescaleDB chunk 执行；不是逐属性独立保留策略（[Datapoint retention](https://docs.openremote.io/docs/developer-guide/system-administration/#datapoint-retention)）。
6. **可视化**：Manager UI 提供地图、资产树、属性及历史视图；Insights 可建立多仪表盘，包含折线图、柱状图、KPI、仪表盘、图片、地图、表格、属性控制和 Gateway 等组件，并支持私有、共享及公开仪表盘（公开读取需另行开放资产/属性权限）（[Manager UI / Insights](https://docs.openremote.io/docs/user-guide/manager-ui/#insights)）。
7. **权限/多租户**：Realm 是租户边界，隔离用户、资产、规则和 UI 样式；角色组合各系统区域的读写权限。普通用户与 Service User 分别用于交互登录和程序访问；Restricted User 只能访问关联资产，并受属性级 restricted read/write 配置约束（[Realms, Users and Roles](https://docs.openremote.io/docs/user-guide/identity-and-security/realms-users-and-roles/)、[ClientRole.java](https://github.com/openremote/core/blob/main/model/src/main/java/org/openremote/model/security/ClientRole.java)）。
8. **部署架构**：官方 Docker Compose 栈包括 PostgreSQL、Keycloak、Manager、Proxy 和可选 Map 服务（[系统管理](https://docs.openremote.io/docs/developer-guide/system-administration/)）。定制项目可基于官方模板扩展资产类型、协议、Manager/UI，而不直接修改核心仓库（[Custom Deployment](https://docs.openremote.io/docs/user-guide/deploying/custom-deployment/)、[custom-project](https://github.com/openremote/custom-project)）。
9. **边缘能力**：边缘侧运行独立 OpenRemote Manager，由边缘主动通过 WebSocket 连接中心 Manager，使用 Keycloak OAuth Client Credentials；每个边缘 Realm 最多配置一个中心连接。边缘是资产权威来源，资产从边缘单向同步到中心，中心发起的交互再转发到边缘处理；文档没有给出断网期间所有本地功能均可用的笼统保证（[OpenRemote as Edge Gateway](https://docs.openremote.io/docs/user-guide/gateways-and-devices/edge-gateway/)）。

### ThingsPanel

**项目身份、版本与授权。** 官方组织为 [ThingsPanel](https://github.com/ThingsPanel)，社区版主要仓库为 [thingspanel-backend-community](https://github.com/ThingsPanel/thingspanel-backend-community) 与 [thingspanel-frontend-community](https://github.com/ThingsPanel/thingspanel-frontend-community)。访问时社区后端最新 Release 为 **v1.3.0**（2026-10-29，[Release](https://github.com/ThingsPanel/thingspanel-backend-community/releases/tag/v1.3.0)），后端采用 [Apache-2.0](https://github.com/ThingsPanel/thingspanel-backend-community/blob/main/LICENSE)。下述在线文档未逐页标软件版本，页面显示的最近更新日期为 2026-10-19。

**社区与商业边界。** 官方角色文档明确：Community Edition 仅支持单租户，不提供租户级用户管理、角色及按用户分配设备；这些属于 Enterprise Edition（[Role Management](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/system-managment/role_managment)）。官方提供[在线演示](https://www.thingspanel.io/docs/introduction/demo)，但本次未从官方资料确认其属于通用托管 SaaS 产品。

1. **设备/物模型**：物模型定义遥测、属性、事件和指令，可配置 Web/App 图表并供设备模板复用（[Thing Model](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/device-mgt/device_template)）。设备模板可绑定产品/物模型、关联设备、选择接入协议，并配置数据预处理、自动化和告警；预处理脚本可将二进制或非标准 JSON 转为平台数据格式（[Device Template](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/device-mgt/device_config)）。
2. **协议接入**：MQTT 直连文档给出 Basic 认证及 `devices/telemetry` 遥测上报（[MQTT](https://www.thingspanel.io/docs/device-connectivity/mqtt)）；Modbus 文档覆盖 RTU/TCP 网关接入，并链接独立 Modbus TCP 直连指南和官方插件仓库（[Modbus](https://www.thingspanel.io/docs/device-connectivity/modbus)、[modbus-protocol-plugin](https://github.com/ThingsPanel/modbus-protocol-plugin)）。自定义接入服务通过 HTTP API 与 MQTT 和平台交互，可处理遥测、属性、事件、指令及上下线状态（[Custom Protocol](https://www.thingspanel.io/docs/developer-guide/developing-plug-in/customProtocol)）。
3. **规则**：场景联动支持时间条件和设备条件；设备条件可基于属性、事件、在线/离线状态，规则可针对单设备或某设备模板下的设备，并提供触发日志（[Scene Linkage](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/automation/scene_linkage)）。架构文档还描述流处理、规则计算及向外部系统转发，但这里没有把架构图中的所有处理能力等同于社区版开箱功能（[System Architecture](https://www.thingspanel.io/docs/introduction/architecture)）。
4. **告警**：可创建、启停、编辑和删除告警规则，告警记录支持按时间与级别筛选；该页未描述完整的认领/解决工作流（[Alarm Information](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/warning/warning_message)）。通知策略包含服务、接收人和内容，支持租户默认策略及告警规则覆盖；文档以邮件/SMTP 为例，并说明告警与待通知记录会持久化以便故障后恢复处理（[Notification Group](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/warning/Notification_group)）。
5. **历史/存储**：快速开始包含实时/历史数据查看（[Quick Start](https://docs.thingspanel.cn/zh-Hans/docs/quick_start)）；v1.3.0 All-in-One 部署中的 `postgres` 服务使用 TimescaleDB（[Installer 架构](https://github.com/ThingsPanel/thingspanel-installer)）。架构文档描述遥测历史、最新值、属性和事件存储及历史查询，并列出其他数据库选项；具体保留期限和清理策略**未从官方资料确认**（[System Architecture](https://www.thingspanel.io/docs/introduction/architecture)）。
6. **可视化**：租户仪表盘提供设备、告警、最新数据、趋势及多种图表/控制卡片（[Dashboard](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/visualization/visualization-kanban)）。ThingsVis 编辑器提供拖拽画布、组件属性与数据绑定、预览和发布（[ThingsVis Editor](https://www.thingspanel.io/docs/user-guide/system-admin-operation-manual/visualization/thingsvis-editor)）。
7. **权限/多租户**：官网有系统管理员创建和管理租户的文档（[Tenant Management](https://www.thingspanel.io/docs/user-guide/system-admin-operation-manual/system-managment/tenant-management)），但角色页明确社区版仅单租户；企业版才提供租户用户、角色、菜单/页面/功能权限以及按用户分配设备的数据范围（[Role Management](https://www.thingspanel.io/docs/user-guide/tenant-operation-manual/system-managment/role_managment)）。因此社区版结论以角色页的明确版本声明为准，不把通用租户文档当作社区版多租户证明。
8. **部署架构**：官方 v1.3.0 All-in-One 以 Docker Compose 部署，统一入口为 `gateway:8080`，包含 Vue 前端、Go 后端、TimescaleDB、Redis、MQTT、自动化/通知、Modbus、HTTP adapter 和 ThingsVis 等服务（[Installer README 与架构](https://github.com/ThingsPanel/thingspanel-installer)、[配置参考](https://github.com/ThingsPanel/thingspanel-installer/blob/main/docs/configuration.md)）。另有 Nginx/HAProxy、VerneMQ、Redis、TDengine 等分层集群设计文档，但该页未提供完整集群 Manifest（[Cluster Deployment](https://www.thingspanel.io/docs/installation/cluster_deploy)）。
9. **边缘能力**：官方文档明确支持 MQTT 网关及绑定子设备，使用独立模板和子设备地址（[MQTT Gateway](https://www.thingspanel.io/docs/device-connectivity/gateway)）。该能力证明的是网关拓扑接入；可安装的独立边缘运行时、本地规则执行、断网缓存/续传**未从官方资料确认**。

### Hummingbird（蜂鸟）

**名称与官方项目身份核实。** 名称相近的 [`hummingbird-iot/hummingbird`](https://github.com/hummingbird-iot/hummingbird) 在访问时返回 **404**，因此不能把它作为已确认的官方仓库。可核实的蜂鸟 Golang 物联网平台是 [`winc-link/hummingbird`](https://github.com/winc-link/hummingbird)：其 README 明确写作“蜂鸟物联网平台是由 Golang 编写”，并链接同一[官方文档站](https://doc.hummingbird.winc-link.com/guide/product/introduce.html)。本文只以后者为调研对象，不声称前者已改名或迁移。

**版本、授权与版本边界。** GitHub 仓库访问时[没有 Releases](https://github.com/winc-link/hummingbird/releases)；Compose 中的 `hummingbird-core:v1.0` 只是该文件所用镜像标签，不能据此认定为全项目最新版本。仓库 [LICENSE](https://github.com/winc-link/hummingbird/blob/master/LICENSE) 是标准 Apache-2.0 文本，但 [README 开源协议段](https://github.com/winc-link/hummingbird/blob/master/README.md#开源协议)和[版本页](https://doc.hummingbird.winc-link.com/guide/price/plans.html)另写商用需先联系授权；这里只并列记录官方表述，不作法律解释。

官方版本表区分免费社区/开源版、付费镜像版和企业版：社区版包含设备接入管理、告警、OpenAPI、MQTT/HTTP/TCP；规则引擎、场景联动、Modbus/GB28181、网关/子设备、数据权限、多数据库和可视化大屏被标为镜像/企业版能力（[版本与功能表](https://doc.hummingbird.winc-link.com/guide/price/plans.html)）。需注意，社区 MQTT 指南仍列出子设备 Topic，与版本表“网关/子设备不属社区版”的口径不完全一致；本稿保留该差异，不据此推断开源仓库已完整交付相关功能（[社区 MQTT 指南](https://doc.hummingbird.winc-link.com/guide/use-cases/access/mqtt-protocol.html)）。

1. **设备/物模型**：物模型通过 `product_id` 关联产品，包含 Properties、Events、Services；属性有读写模式，事件分 info/alert/error，服务支持同步/异步调用，参数类型包括数值、文本、布尔、枚举、结构体和数组（[物模型管理 API](https://doc.hummingbird.winc-link.com/guide/api/thing-model.html)）。
2. **协议接入**：版本表将 MQTT、HTTP、TCP 列入社区版，将 Modbus、GB28181 列入镜像/企业版（[版本表](https://doc.hummingbird.winc-link.com/guide/price/plans.html)）。社区 MQTT 文档覆盖设备认证、属性/事件上报、属性设置/查询、服务调用及对应 Reply Topic；协议消息Envelope的 `version: 1.0` 不是产品版本（[MQTT 接入](https://doc.hummingbird.winc-link.com/guide/use-cases/access/mqtt-protocol.html)）。
3. **规则**：规则引擎文档描述从消息总线取数，以查询字段和条件形成 SQL 过滤，再按转发方式输出，并可查看运行日志（[Rule Configuration](https://doc.hummingbird.winc-link.com/guide/advance/engine/rule-configuration.html)）。场景文档支持设备数据/状态/事件及定时触发，动作可设置多个设备属性并记录执行日志（[Scene Linkage](https://doc.hummingbird.winc-link.com/guide/advance/scene.html)）。两类能力在版本表中均**不属于社区版**（[版本表](https://doc.hummingbird.winc-link.com/guide/price/plans.html)）。
4. **告警**：告警规则可由设备数据、事件或在线/离线状态触发，支持紧急/重要/次要/提示四级、聚合条件、静默期、处理/忽略及数据定位；通知渠道列出站内告警列表、企业微信、钉钉和飞书机器人（[Alarm Center](https://doc.hummingbird.winc-link.com/guide/operate/alert.html)）。版本表将告警中心和告警通知列入三个版本（[版本表](https://doc.hummingbird.winc-link.com/guide/price/plans.html)）。
5. **历史/存储**：官方 API 提供属性、事件和服务调用历史查询，支持时间范围及分页；数据保留期限**未从官方资料确认**（[物模型使用 API](https://doc.hummingbird.winc-link.com/guide/api/use-thing-model.html)）。README 称默认使用 SQLite/LevelDB，并可选择 MySQL/TDengine；但版本表把“多数据库支持”划给镜像/企业版，因此社区版可替换数据库的范围不按 README 的概括表述扩张（[README 产品简介](https://github.com/winc-link/hummingbird/blob/master/README.md#产品简介)、[版本表](https://doc.hummingbird.winc-link.com/guide/price/plans.html)）。
6. **可视化**：数据大屏文档要求 **Hummingbird Enterprise 且平台版本 3.0+**，提供基于 GoView 的图表编辑、设备数据绑定及当前值/趋势类图表；不能计作社区版能力（[Data Visualization](https://doc.hummingbird.winc-link.com/guide/dataviz/large_screen.html)、[版本表](https://doc.hummingbird.winc-link.com/guide/price/plans.html)）。
7. **权限/多租户**：版本表只明确“数据权限”属于镜像/企业版；社区版角色/权限模型及三个版本的多租户能力均**未从官方资料确认**（[版本表](https://doc.hummingbird.winc-link.com/guide/price/plans.html)）。
8. **部署架构**：快速开始使用 Docker 20+ 与 Docker Compose（[Docker Quick Start](https://doc.hummingbird.winc-link.com/guide/quick/quick_start_docker.html)）。官方 Compose 文件包含 `mqtt-broker`、`ekuiper`、`hummingbird-core` 三个服务，Core 依赖前两者并开放管理端口 3000；该 Compose 未声明独立数据库服务（[docker-compose.yml](https://github.com/winc-link/hummingbird/blob/master/manifest/docker/docker-compose.yml)）。
9. **边缘能力**：官方文档把 Octopus、OpenGW 作为配套网关，而非上述核心仓库内已核实的模块。Octopus 页面列出多种工业协议，但未说明离线/本地处理（[Octopus](https://doc.hummingbird.winc-link.com/guide/gateway/octopus.html)）；OpenGW 页面明确协议转换、公式/Lua、本地多设备场景联动及 MQTT/HTTP/TCP/UDP 上报，但未给出云端断连时完整自治保证，也未链接其独立源码仓库（[OpenGW](https://doc.hummingbird.winc-link.com/guide/gateway/opengw.html)）。

## 外部平台能力对照（克制摘要）

下表只是上述官方证据的索引式摘要；“有文档”不等于所有版本均可用，也不表示性能或成熟度排名。

| 能力 | OpenRemote 1.33.0 | ThingsPanel | Hummingbird |
|---|---|---|---|
| 设备/物模型 | Asset/Attribute/Meta/Value descriptors；Java 扩展模型 | 遥测/属性/事件/指令物模型；设备模板 | 产品级属性/事件/服务模型 |
| 协议接入 | 多类 Agent/Protocol；支持自定义扩展 | MQTT、Modbus、自定义接入服务及第三方接入 | 社区版列 MQTT/HTTP/TCP；Modbus/GB28181 属付费版本表 |
| 规则 | When-Then、Flow、Groovy；Realm/Global 范围 | 时间/设备场景条件与日志 | SQL 过滤转发、场景联动；版本表列为付费能力 |
| 告警 | 规则生成告警；分派、严重级别及完整处理状态 | 告警规则、级别筛选和通知策略 | 数据/事件/状态触发，四级告警与机器人通知 |
| 历史/存储 | PostgreSQL/TimescaleDB；可配置全局保留期 | 历史查询；All-in-One 使用 TimescaleDB；保留策略未确认 | 属性/事件/服务历史 API；社区多库范围需谨慎按版本表理解 |
| 可视化 | Manager UI 与 Insights 多组件仪表盘 | 仪表盘与 ThingsVis 拖拽编辑 | Enterprise 3.0+ 数据大屏；非社区版结论 |
| 权限/多租户 | Realm 隔离、角色、Restricted User 属性级权限 | 社区单租户；企业版租户用户、角色和设备数据权限 | 数据权限属付费版本；多租户未确认 |
| 部署架构 | Compose：Manager、PostgreSQL、Keycloak、Proxy 等 | All-in-One Compose；另有集群架构文档 | Compose：Core、MQTT Broker、eKuiper |
| 边缘能力 | 边缘 Manager 单向资产同步到中心，中心交互转发边缘 | 网关/子设备接入；独立边缘运行时及离线机制未确认 | 配套 Octopus/OpenGW；不能等同于核心仓库边缘模块 |

## Simple IoT v0.31.0：原生能力清单

这里的“原生”指固定上游源码中已有的机制；默认 `siot` 可执行程序会注册 `DefaultClients`，但 FastIOT 是否使用要另看下一节。原生清单也不代表每项都已达到同一成熟度，例如 Sparkplug B 文档明确标为 preliminary（`third_party/simpleiot/cmd/siot/main.go:145-155`，`third_party/simpleiot/docs/user/mqtt.md:330-386`）。

| 领域 | v0.31.0 原生能力与边界 | 本地源码/文档证据 |
|---|---|---|
| 设备/数据模型 | 以 **Node + Point + Edge/DAG** 表达设备、虚拟对象、配置和测量值；Point 携带时间、值/文本、Key、Origin，节点可移动、镜像和按层级组织。它是通用的扁平点模型，不是 ThingsPanel/Hummingbird 那种固定“产品—属性/事件/服务”物模型。 | `third_party/simpleiot/docs/ref/data.md:12-69,216-321`；`third_party/simpleiot/docs/adr/8-iot-data-models.md:7-15,181-199` |
| 协议与硬件接入 | 默认客户端表包含 Serial MCU、CAN、Rule、DB、SignalGenerator、Sync、Metrics、Modbus、OneWire、MQTT、Particle、Shelly、NTP、User、MsgService、GPS、GPIO、IIO、Browser、NetworkManager、Update、File。Modbus 支持 TCP/RTU 及 client/server；串口支持 COBS 二进制与 Zephyr shell；GPIO/IIO 面向 Linux 硬件。 | `third_party/simpleiot/client/client.go:26-101`；`third_party/simpleiot/docs/user/modbus.md:8-41,80-99`；`third_party/simpleiot/docs/user/mcu.md:27-75`；`third_party/simpleiot/data/schema.go:336-420,524-707` |
| MQTT | 可选内嵌 MQTT 3.1.1 Broker、显式 Topic 订阅、JSON 路径映射、Topic Schema 自动建节点和 Sparkplug B；Broker 默认关闭，外部 Broker 客户端仍属规划，MQTT 5 与逐客户端 Topic 权限也未完成。 | `third_party/simpleiot/docs/user/mqtt.md:10-55,57-124,126-183,330-386,460-472`；`third_party/simpleiot/server/server.go:53-56` |
| 规则 | 上游 Rule 由 condition/action 子节点组成，支持数值/文本/布尔条件、 schedule、最小持续/解除时间，动作包括通知、写点和播放音频；规则状态持久化。它不是 FastIOT 的“从上到下、首条命中、多输出顺序执行”决策表。 | `third_party/simpleiot/docs/user/rules.md:12-59,61-159,182-266`；`third_party/simpleiot/client/rule.go` |
| 告警/通知 | 原生机制是规则产生 Notification，再由 User/MsgService 投递；支持 Twilio SMS、SMTP、ntfy、去重与限流。**确认、升级/逐级通知未实现**，不能直接等同于完整告警中心。 | `third_party/simpleiot/docs/ref/notifications.md:18-47,49-97`；`third_party/simpleiot/docs/user/messaging.md:1-41` |
| 当前状态与历史 | 内嵌 NATS JetStream 同时保存配置、当前状态和点历史；默认每个 subject 保留最近 20,000 条、默认 S2 压缩，当前值始终保留；可调保留和 fsync，`-1` 表示不限量。`PointHistory` 可直接读现有流，无需第二数据库。 | `third_party/simpleiot/docs/ref/store.md:1-29,100-170,172-211`；`third_party/simpleiot/store/history.go:15-99` |
| 长期时序存储 | Db client 可向 VictoriaMetrics 或 InfluxDB 2.x 写入，使用 durable consumer 在客户端/数据库故障后续传，但受本地流保留上限约束；新 Db 节点不回填既有历史，高频点不走持久流。TimescaleDB 只是 planned，未实现。 | `third_party/simpleiot/docs/user/database.md:1-43,45-106`；`third_party/simpleiot/client/db.go` |
| 可视化 | 自带 Svelte 管理 UI，提供树、节点编辑、实时更新和原始点视图；高级图表/仪表盘官方建议接 Grafana，内置大屏不是现有能力。 | `third_party/simpleiot/docs/user/ui.md:7-70,141-167`；`third_party/simpleiot/docs/user/graphing.md:1-19` |
| 用户/权限/设备凭证 | 用户挂在组/设备树中获得相应范围，密码使用 bcrypt；用户 JWT、设备 NKey、Enrollment Token 由同一授权层约束，支持设备凭证撤销。上游角色是边上的 admin/user，不应直接当成 FastIOT 的 admin/operator/viewer 产品角色或多租户方案。 | `third_party/simpleiot/docs/user/users-groups.md:1-76`；`third_party/simpleiot/docs/ref/security.md:106-205,223-275` |
| 部署与运行 | 单个 Go 二进制内嵌 UI、NATS 与 JetStream，支持 Linux/macOS/Windows 及多种架构；也可连外部 NATS，但官方明确应用 active/active 不可用、完整 HA 尚未解决。 | `third_party/simpleiot/docs/ref/architecture-app.md:7-32`；`third_party/simpleiot/docs/user/installation.md:1-25,82-127`；`third_party/simpleiot/docs/ref/high-availability.md:1-39,66-120` |
| 边缘与同步 | 下游实例通过 NATS 或 NATS WebSocket 主动连接上游，复制 JetStream stream；断网时本地采集、规则和配置继续，重连按 durable consumer 补传，受保留上限约束。当前只复制实例 root boundary，嵌套 device boundary 尚未实现。 | `third_party/simpleiot/docs/user/sync.md:1-85,127-179`；`third_party/simpleiot/docs/ref/sync.md:15-35,51-84,116-170` |
| 配置与宿主机运维 | YAML export/import/provisioning 共用节点文件模型；另有系统/应用/进程/Prometheus 指标、文件传输、应用/OS 更新、NetworkManager、NTP、kiosk Browser 等。它们能力大、副作用也大。 | `third_party/simpleiot/docs/user/configuration.md:80-157,229-260`；`third_party/simpleiot/docs/user/metrics.md:1-11,67-165`；`third_party/simpleiot/docs/user/update.md:1-32`；`third_party/simpleiot/docs/ref/security.md:53-87` |

## FastIOT：实际复用、改造与关闭边界

### 已复用或在其上产品化的能力

| 能力 | 当前实现，不把计划当完成 | 证据 |
|---|---|---|
| 单进程内核与持久化 | 复用 `server.NewServer`、内嵌 NATS/JetStream、Store 和客户端生命周期；同一进程承载产品 HTTP API 与内嵌 React UI。设置 `StoreSyncAlways=true` 以每次写入 fsync，保留和压缩未在 FastIOT 参数中覆盖，因而使用上游默认值。 | `cmd/fastiot/main.go:35-65`；`internal/web/embed.go:1-17`；`third_party/simpleiot/server/server.go:297-305`；`third_party/simpleiot/docs/ref/store.md:128-211` |
| 业务树与稳定绑定 | 复用 Node/Point、树查询、写点、移动/删除及用户 anchor；产品 API 只显示 device/group/variable/modbus/modbusIo/decisionTable/deviceTemplate。自有 `businessId` 与决策表 binding 隔离换源，避免规则直接绑死硬件地址。 | `internal/appapi/handler.go:202-298,592-640`；`internal/appapi/resources.go:54-181`；`internal/decision/client.go:27-43,231-303` |
| Modbus 接入 | 复用上游 Modbus client 的 TCP/RTU 轮询、线圈/寄存器、数据格式、缩放/偏移和错误计数；产品创建时固定 `ClientServer=client`。集成补丁在一次完整成功轮询后写 `lastGoodPoll`，供决策表拒绝失效输入。 | `internal/appapi/modbus.go:23-79,117-133`；`cmd/fastiot/main.go:59-62`；`third_party/simpleiot/client/modbus.go:186-209`；`third_party/README.md:12-14` |
| 点表与型号模板 | 没有开放上游任意 YAML 节点导入，而是实现中文 CSV 预检/整批导入导出和仅含 Modbus 点位的模板；模板不包含源节点 ID、业务 ID 或连接凭据。 | `internal/appapi/modbus.go:136-230`；`internal/appapi/templates.go:12-85,88-140`；`docs/p0-acceptance.md:11-12,26-28` |
| 规则/控制内核 | **未复用上游 Rule client**，而是注册自有 `decisionTable`：多条件 AND、首行命中、多输出顺序、草稿/发布/启用分离、纯函数模拟、人工接管、输入 freshness 和范围校验。发布历史也存为 Point。 | `cmd/fastiot/main.go:59-62`；`internal/decision/table.go:34-138`；`internal/decision/client.go:14-43,90-199,315-395`；`internal/appapi/rules.go:50-146` |
| 人工操作反馈 | 在产品 API 上实现请求 ID 防重、已接受/已回读/失败/结果未知、只读和停用检查、回读等待；当前只保存每点 `lastCommand`，不是完整操作审计中心。 | `internal/appapi/handler.go:300-415`；`internal/appapi/commands.go:11-87` |
| 登录与树范围权限 | 复用 `client.UserCheck`、Store 的 JWT/anchor/树关系与密码散列；产品再加 `easyemsAccess`，形成 admin/operator/viewer 和 RootNodeIDs，且每个产品请求重新鉴权。首次启动在 ready 前替换上游默认 admin/admin。 | `internal/appapi/handler.go:127-199,544-624`；`internal/appapi/bootstrap.go:17-82`；`third_party/simpleiot/docs/user/users-groups.md:41-76` |
| 历史读取与导出 | 直接调用上游 `Store.PointHistory`，产品 API 限定 variable/modbusIo 及 value/valueSet，限制每次 1—2000 点，并提供 CSV；已有集成测试覆盖越权类型拒绝。**当前前端没有历史/趋势页面或调用。** | `internal/appapi/history.go:12-85`；`internal/appapi/handler_integration_test.go:178-219`；`frontend/src/App.tsx:62,154-196`；`frontend/src/api.ts` |
| 交付与恢复 | 已有启动健康检查、连接/规则/接管/命令交付检查，以及停止后复制完整数据目录的离线恢复验收；没有宣称在线热备份或物理设备已验收。 | `cmd/fastiot/main.go:68-115`；`internal/appapi/delivery.go:22-127`；`README.md:44-54`；`evidence/verification.md:17-19,30-37` |
| 产品 UI | 上游 Svelte/Elm 工程界面被产品 UI 替代；当前页面只有工作台、决策表、账号与权限、交付检查，使用 Ionic/React 并每 2 秒轮询快照。 | `frontend/src/App.tsx:59-89,151-196`；`frontend/package.json:1-21`；`third_party/README.md:6-14` |

### 未复用、未运行或有意关闭的能力

| 上游能力 | FastIOT 当前状态与原因 | 证据 |
|---|---|---|
| 默认客户端全集 | FastIOT 没有调用 `client.DefaultClients`，只注册 `NewModbusClient` 和 `NewDecisionTableClient`。因此上游 Rule、Db、Sync、Metrics、MQTT、Serial、CAN、OneWire、Shelly、Particle、GPIO、IIO、GPS、User、MsgService、Browser、NetworkManager、NTP、Update、File 等 manager 均不在产品运行路径。 | `cmd/fastiot/main.go:59-62`；`third_party/simpleiot/client/client.go:26-101` |
| 通用工程界面与通用接口 | `AppOnly=true` 关闭通用 `/v1`、`/old` 和浏览器 NATS WebSocket 代理；只保留产品 `/api` 与产品 UI。这是为了防止用户绕过产品级角色、节点类型和目标校验。 | `cmd/fastiot/main.go:43-51`；`third_party/simpleiot/api/server.go:30-85,119-146`；`third_party/README.md:6-14` |
| NATS/WS/MQTT 对外监听 | NATS client listener 绑定回环，AppOnly 同时禁用 WS listener；FastIOT 未设置 `NatsMQTTPort`，其上游默认 0 即关闭。因而不能把内嵌 Broker 或远程 NATS 当作现有接入能力。 | `cmd/fastiot/main.go:36-51`；`third_party/simpleiot/server/server.go:43-56,132-176`；`third_party/simpleiot/server/nats-server.go:45-115` |
| YAML Provisioning 与文件节点应用 | `ProvisioningDisable=true`，连树中的 provisioning 文件也关闭；产品只提供受校验的 CSV/模板，不让配置文件绕过 UI/API 边界。 | `cmd/fastiot/main.go:47-52`；`third_party/simpleiot/server/server.go:76-86,490-519` |
| 上游告警/通知 | 上游 Rule/User/MsgService 均未注册。FastIOT 虽有自己的 `Alert`、去重式 `Sync` 和 `Monitor` 源码，但 main 未添加 `alerts.NewMonitor`，`Handler.ServeHTTP` 也没有调用 `listAlerts`/`alertAction` 的路由，前端没有告警入口；所以当前是**未接线代码，不是可用告警功能**。 | `internal/alerts/monitor.go:12-105`；`internal/appapi/alerts.go:16-125`；`cmd/fastiot/main.go:59-62`；`internal/appapi/handler.go:66-125`；`frontend/src/App.tsx:154-196` |
| 外置时序数据库与 Grafana | Db client 未注册，产品也未部署 VictoriaMetrics/InfluxDB/Grafana；当前存储只有内嵌 JetStream。这样保持单应用，但长期分析和超本地保留期历史不是现有能力。 | `cmd/fastiot/main.go:59-62`；`Dockerfile:1-21`；`third_party/simpleiot/docs/user/database.md:1-12` |
| 边缘同步与设备 fleet | Sync client 未注册，NATS 又只监听回环；设备 NKey/凭证基础仍由 server 初始化，但没有上游连接、离线补传或中心—边缘产品流程。 | `cmd/fastiot/main.go:36-62`；`third_party/simpleiot/server/server.go:375-443`；`third_party/simpleiot/docs/user/sync.md:36-85` |
| Modbus server 与宿主控制 | 产品创建 Modbus 时固定 client，未开放 server/listener；更新、网络、时间、kiosk、任意文件和音频等宿主侧能力未注册、无产品 API。 | `internal/appapi/modbus.go:49-79`；`cmd/fastiot/main.go:59-62`；`third_party/simpleiot/docs/ref/security.md:53-87` |

## 哪些能力不应为了“榨干 Simple IoT”而开放

以下是产品边界建议，不是对上游能力的否定；上游面向可自控实例，FastIOT 面向按人、按树授权的设备操作产品，信任模型不同。

1. **不开放通用 `/v1`、原始节点编辑、浏览器 NATS 或共享管理员令牌。** 它们会绕过当前 admin/operator/viewer、RootNodeIDs、只读目标和规则范围检查；保持关闭是安全边界，不是功能欠缺（`internal/appapi/handler.go:153-199,300-350,617-640`，`third_party/simpleiot/docs/ref/security.md:298-372`）。
2. **不开放 Update、NetworkManager、NTP、Browser、任意文件/Provisioning 等宿主管控。** 上游官方明确这些节点可替换程序、重启、改写时间/网络/浏览器；产品操作员不应因此获得宿主权限。未来若确有升级需求，应做独立、签名、可回滚的管理员流程，而不是注册通用 client（`third_party/simpleiot/docs/ref/security.md:53-87,391-415`，`third_party/simpleiot/docs/user/update.md:13-32`）。
3. **不把 Sync 当成“打开开关即得边缘能力”。** 任意上游 URI 会带出实例数据并接受回写；当前产品又未定义设备身份、审批、冲突、权限映射、断网控制和中心恢复语义。没有真实多站点需求及整套验收前应保持关闭（`third_party/simpleiot/docs/ref/security.md:68-83`，`third_party/simpleiot/docs/ref/sync.md:135-170`）。
4. **不批量开放 MQTT、Modbus server、Metrics、Shelly 等网络入口/出站连接。** 它们会引入监听器、凭据、Topic 隔离、SSRF、目标校验和控制回读问题；上游文档也说明 Modbus server 监听所有网卡、多个客户端可主动连接外网/私网。每个产品协议都应单独建模与验收（`third_party/simpleiot/docs/ref/security.md:43-49,73-75`，`third_party/simpleiot/docs/user/configuration.md:19-25`）。
5. **不开放无限保留、原始高点数节点或无边界指标抓取来“充分利用存储”。** 上游 `-1` 是不限量历史，而单节点过多 Point 会让整棵树的读取超过 1 MB 响应限制；产品需要可解释的保留、用量和上限，而非无限增长（`third_party/simpleiot/docs/ref/store.md:128-170,213-272`）。
6. **不直接并行启用上游 Rule 与 FastIOT 决策表。** 两套写点语义并存会增加重复输出、冲突和排障成本；应继续完善当前有序决策表的可观察性与安全，而不是再提供自由脚本/流程画布（`third_party/simpleiot/docs/user/rules.md:101-159`，`internal/decision/client.go:315-351`）。

## 与“小而美：接入、操作、存储，运行保障贯穿”的差距与优先级

以下 P0/P1/P2 是**本调研对后续差距的排序**，不改写 `docs/p0-acceptance.md` 已通过的 P0 验收。完成标准是用户任务闭环和失败路径可验证，不是多一个菜单或多注册一个 client。

### 当前差距总表

| 主线 | 已有基础 | 真正差距 | 建议优先级 |
|---|---|---|---|
| 接入 | Modbus TCP/RTU client、真实只读测试、CSV、模板、错误计数、完整轮询心跳、断线阻断自动输入 | 真实物理设备、RS485 现场和真实品牌 A/B 换绑尚未验收；MQTT/HTTP 仍无产品实现 | **P0** 做现场接入与故障验收；**P2** 才按真实设备需求增加一种协议（`evidence/verification.md:13-19,30-37`） |
| 操作 | 有序决策表、模拟、发布/版本、人工接管、只读/停用、命令防重与回读状态 | 只有 `lastCommand`，缺少可检索的人/规则/对象/结果统一记录；账号只有创建，缺少停用、重置密码等生命周期；高风险操作仍需明确确认与权限边界 | **P0** 补齐操作可追踪和账号安全最小闭环（`internal/appapi/commands.go:11-87`，`internal/appapi/handler.go:120-122,544-590`） |
| 存储 | JetStream 当前状态与历史、S2、每次写 fsync、历史 API 和 CSV、离线完整目录恢复 | 历史 API 没有产品界面；默认 20,000 点/subject 不等于按天保留，当前没有用量/保留可解释性；长期分析没有外置库 | **P0** 用现有 API 做简单趋势/查询/导出；**P1** 做用量、保留和恢复演练；不默认新增数据库（`internal/appapi/history.go:16-85`，`frontend/src/App.tsx:154-196`，`cmd/fastiot/main.go:36-53`） |
| 运行保障 | ready 检查、启动密码替换、连接/规则诊断、交付检查、离线恢复、输入过期保护 | 告警监视器与处理 API 未接入；备份没有产品化预检/记录；缺少面向运行的存储与资源可视性；公网 TLS/证书不在已验收范围 | **P0** 先接通现有少量异常；**P1** 做恢复、用量和资源门槛；公网前安全验收是阻断项（`internal/alerts/monitor.go:74-178`，`internal/appapi/handler.go:44-125`，`internal/appapi/delivery.go:109-117`） |
| 可视化 | 工作台展示当前值、目标值、单位、错误和最近变化 | 缺的是单点/多点历史趋势及异常解释，不是拖拽大屏、BI 或数字孪生 | **P0** 仅补与操作/排障直接相关的趋势（`frontend/src/App.tsx:215-280`） |
| 权限/多租户 | 本人登录、三类产品角色、按树授权、服务端拒绝越权 | 缺账号全生命周期、权限变更/会话失效验证和可检索审计；不需要因此引入 Realm 式多租户 | **P0/P1** 完成账号与审计；多租户不列差距（`internal/appapi/handler.go:153-199,544-590`） |
| 边缘 | 单实例本地采集、控制、存储和离线恢复 | 没有中心—边缘同步；但“小而美”目标本身没有证明多站点是必需 | **P2/暂不做**，只有明确断网多站点需求才立项（`cmd/fastiot/main.go:36-62`） |

### P0：先闭合并验证现有主链路

1. **让存储可消费。** 直接基于 `/api/history` 与 `/api/history/export` 做点位选择、时间范围、简洁趋势和 CSV；同时显示数据时间、过期/缺失和“本地历史受保留上限约束”。验收是浏览器能回答“某点何时变成什么值、谁/哪条规则触发”，不是新建一套时序平台（`internal/appapi/history.go:16-85`，`third_party/simpleiot/store/history.go:15-99`）。
2. **接通最小运行异常闭环。** 只覆盖现有 Monitor 已识别的规则异常、Modbus 心跳过期、命令失败/未知，完成 active → resolved、ack/assign/close、去重和权限测试；在做到之前不宣称告警中心。通知渠道、复杂阈值编排另按需求，不随 P0 捆绑（`internal/alerts/monitor.go:105-178`，`internal/appapi/alerts.go:21-125`）。
3. **让操作可追踪、账号可收回。** 在现有点历史/Origin 上形成统一操作记录，区分人工与规则、请求与回读、失败与未知；补齐用户停用/重置密码、权限变更及会话失效验证。高风险控制先验证对象、值、权限、确认和结果，不增加通用脚本动作（`internal/decision/client.go:337-350`，`internal/appapi/commands.go:11-87`，`internal/appapi/handler.go:544-590`）。

### P1：证明能长期运行和恢复

1. **把保留、磁盘和备份说清并实测。** 用真实采样周期测量 20,000 点/subject 能覆盖多久，展示存储用量和增长，制定保留阈值与超限告警；继续以已验证的离线完整目录备份为基线，增加恢复前兼容性/空间检查和演练记录，不冒充热备份（`third_party/simpleiot/docs/ref/store.md:128-170`，`README.md:44-48`，`internal/appapi/delivery.go:114-117`）。
2. **只采运行保障需要的指标。** 优先应用存活、存储写入/用量、Modbus 连续失败、规则错误和命令异常；不要为了“有 Metrics client”就采集全系统/全进程，避免高基数点反而撑大 Store（`third_party/simpleiot/docs/user/metrics.md:133-165`，`third_party/simpleiot/docs/ref/store.md:213-272`）。
3. **完成现场与安全验收。** 真实 Modbus 设备、RS485、电气环境、品牌 A/B 换绑、独立目标用户，以及需要公网时的 HTTPS、登录限流和会话撤销，都必须以现场结果关闭；模拟器与浏览器截图不能替代（`evidence/verification.md:30-37`，`docs/product-functional-plan.md:232-240`）。

### P2：由可证明的现场需求触发，而非平台对标触发

- **MQTT/HTTP：** 仅当有具体设备/网关和明确 Topic/载荷、认证、在线状态、读写/回读语义时，一次做一种并独立验收；不能直接打开内嵌 Broker 就当完成产品接入（`third_party/simpleiot/docs/user/mqtt.md:21-55,460-472`）。
- **长期外置存储：** 仅在本地保留、查询或聚合经实测不够时引入 Db client；同时接受第二服务的部署、凭据、监控、备份与补传边界，不作为默认架构（`third_party/simpleiot/docs/user/database.md:14-43,45-95`）。
- **边缘同步：** 仅在多站点且断网续传是刚性需求时设计，先解决设备审批、凭证、TLS、冲突、权限映射、中心恢复与版本兼容；不与“单机小而美”同时进行（`third_party/simpleiot/docs/ref/sync.md:135-170`，`third_party/simpleiot/docs/ref/high-availability.md:81-120`）。

## 最终取舍

FastIOT 应对标外部平台的**任务闭环与失败可解释性**，而不是功能格子数量。当前最值得投入的是：把已有历史 API 变成可用趋势，把已有但未接线的告警代码变成经过验收的最小异常闭环，把操作与账号做成可追踪、可收回；其余上游能力继续保持关闭，直到有真实场景证明其收益大于安全、部署和运维成本。

