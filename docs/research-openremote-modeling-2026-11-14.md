# OpenRemote 建模专题：从业务对象到设备绑定

## 结论与范围

**FastIOT 值得借鉴的是“描述业务对象 → 生成编辑与展示 → 单独绑定通信”，不是 OpenRemote 的 Java 类型实现。** 现有规格已经覆盖这条主线；本次不增加建模模块，只提出两项尚未写细的验收建议：多层约束不能被实例配置静默放宽；复制配置不能复制出貌似有效的设备运行态。[R][M][C]

- **事实：** OpenRemote 的 Asset/Attribute、类型描述符、实例 meta 和 AgentLink 分工明确；Manager 可选择已加载类型、配置实例属性，源码也有单资产复制入口。[D1][D2][D3][N1][N2]
- **事实边界：** 单实例复制、协议专用文件导入和类型定义是三件事。官方文档明确类型模型当前在 Java 中配置；实例填参或复制不能被称为无代码发布新类型。[D1][D2][N1]
- **FastIOT 判断：** 业务引用稳定的前提是模型契约及映射等价，不是换品牌自动兼容；Point、Unit、横向 Group、模板和 Profile 也不能与 OpenRemote 对象机械一一等同。[M][C]

核验日：2026-11-14。复用[既有 OpenRemote 研究][R]的模型、链接与 Manager 证据，本次仅补充 **5 份官方来源**：资产树、页面新增处理、创建对话框、属性输入组件和约束验证文档。源码沿用既有研究的固定提交 `c7652131fe2b36f7b4341a0d8a20a94fc8107667`，文档沿用 `c57c4c260daf8fa5ebc32e7c851a9accd9a6a155`；不是新的发布版本声明。

**证据等级：** 下文“事实”指官方文档或固定源码可直接支持；“建议”只针对 FastIOT；“未知”表示本次证据不足，不等于产品没有。未运行 OpenRemote 或 FastIOT，未安装依赖、连接设备或实测 UI；不重查部署、许可和规则引擎，不修改既有研究、规格或代码。

## 1. 先分清用户到底在编辑哪一层

| 层次 | OpenRemote 的已证实含义 | 对用户工作的影响与边界 |
| --- | --- | --- |
| Asset | 物理或逻辑对象的实例，可按父子树组织；有自身属性与当前状态。[D1] | “泵房”“泵”“测量设备”都可作为资产表达，但名字或树位置不自动赋予组合控制能力。 |
| Attribute | Asset 上的命名数据成员，保存值；属性名用于关联描述符。值形状来自系统已知的值类型。[D1][D3] | 编辑实例中的 `pressure`，不等于修改所有泵的类型定义；业务属性也不必对应一个原始寄存器。 |
| AssetTypeInfo / AssetDescriptor | 类型信息汇集适用描述符，作为校验和 UI 生成的 schema；AssetDescriptor 提供类型名、图标、颜色等。[D1] | 先选已加载类型再建实例。图标、颜色是表达，不是能力或安全契约。 |
| AttributeDescriptor | 命名成员的值类型、约束、格式、单位、默认 meta、是否 optional。[D1] | 可要求某成员存在，并给新属性带入默认配置；`optional=false` 只要求属性存在，不等于必须有非空值或已绑定设备。 |
| ValueDescriptor | 描述数据形状、JSON 类型、数组维度及适用的约束等。[D1] | “数值”“布尔”“复合值”决定数据与输入形状，不单独决定它是观测、配置还是控制目标。 |
| MetaItemDescriptor / 实例 meta | 前者定义已知配置项的类型与约束；后者是在属性上实际配置的值，如单位、只读、历史、规则用途和 Agent link。[D1][D3] | 描述符是“可配置什么”，实例 meta 是“这一个属性怎么用”。meta 不只是备注，能影响系统行为。 |
| Agent / AgentLink | Agent 是特殊 Asset，保存连接配置，与协议实例一对一；属性上的 `agentLink` 指定 Agent ID、链接类型与协议参数。[D1][D2] | 业务树回答“属于哪里”，链接回答“值从哪里来、往哪里写”；通信绑定不由父节点名称推导。 |

**类型与实例不能混称：** 官方 `Asset Type Model` 明确当前类型模型由 Java 配置，Manager UI 配置类型是长期目标；另一方面，Manager 已能为实例选择可选属性、增加系统已知值类型的自定义属性。[D1][D3] 两句话同时成立。通用 `Thing` 允许灵活组织属性，但配置十个相似 Thing，仍不能据此声称已经定义并发布了一个可复用“泵类型”。

## 2. 用户如何建模、复用、再绑定

### 2.1 OpenRemote 的已证实路径

以下是文档／源码还原的用户任务，不是本次实机操作记录。

1. **决定业务对象。** 先确定资产的含义和父级，例如“厂房 → 泵房 → 泵”；没有对应已加载类型时，可用通用 `Thing` 表达实例，不假称已经创建新类型。[D1][D3]
2. **Assets → `+` 创建。** 选择已加载类型、填名称、选父级，查看固定成员并勾选可选成员。创建对话框以描述符的 `optional` 区分这两类；新增处理构造属性名、类型和默认 meta，而不是把已有现场资产的当前读数作为新类型默认值。[D3][N3][N1]
3. **Edit asset → Add attribute 补充实例属性。** 使用已有可选成员或系统已知值类型的 custom attribute；为属性配置 label、units、constraints、format、read only、store data points 等，再保存。[D3] 例如压力读数与压力设定分成两个属性，不能仅靠两者都叫“压力”推断用途。
4. **复用已有实例时，明确选择的是复制。** 在资产树单选已有资产，满足可新增及非只读条件时出现 Duplicate 按钮。它读取该资产并复制数据，去掉 `id/path/createdOn/version`，名称加 ` copy`，进入新增处理。页面在编辑模式下加载为待编辑资产，非编辑模式下走保存与加载；不能把这条路径描述成总有“改绑定后才创建”的专用模板向导。[N1][N2]
5. **创建并配置 Agent。** 同样通过 `+` 选择协议 Agent，填写该协议的连接参数。对不支持发现／导入的协议，在业务属性上添加 Agent link，选择 Agent 和正确的链接类型，填写协议专属定位及转换字段。[D2][D3]
6. **对照入站与出站语义。** 入站 `valueFilters` 提取消息，`valueConverter` 做声明式映射；出站另有 `writeValueConverter`、`writeValue` 等配置。布尔 `true` 与设备字符串 `ACTIVE` 的映射可以配置，但“返回值可转换”不证明物理行为、范围或反馈等价。[D2]
7. **查看属性值与用途对应的展示。** 历史开启后可选择属性查看历史；权限和只读配置影响 UI 输入。[D1][D3][N4] 需要操作时仍须区分写值与设备真实反馈；已有研究已核实 `updateOnWrite` 的本地回填边界，此处不重查控制链路。[R]

**另一条入口：协议发现／导入。** 对支持的协议，用户在 Agent 页面发现设备，或选择专用项目文件导入；官方明确列出 Velbus `.vlp`、KNX `.etsproj`，生成资产会挂在 Agent 下面。[D2][D3] 这说明“协议导入树”可作为建模起点，不说明通信拓扑就是业务拓扑，也不证明导入后业务关系、规则引用与现场语义已完整。

### 2.2 复制、导入与“模板”到底证明了什么

| 复用方式 | 已证实能力 | 不能推出的结论 |
| --- | --- | --- |
| 从已有类型创建 | 依据 AssetTypeInfo 选择类型，建立必需及勾选的可选属性并带入默认 meta。[N1][N3] | 不能推出交付人员能在 UI 编辑、发布新的类型定义。 |
| Duplicate 单资产 | `_onCopyClicked` 复制一个资产响应，清除上述身份／版本字段，再派发新增事件。[N1][N2] | 不是对子树递归复制的证据；不是带模板来源、实例覆盖、版本应用规则的模板机制。 |
| 复制时的配置保留 | 该复制函数没有逐项清除 `attributes` 中的值、时间戳或 AgentLink；保留范围取决于源 API 实际返回的数据。[N1] | 不证明服务端不做其他处理，也不能声称所有秘密都会复制或一定产生重复写入；但足以说明不能把该函数当“安全参数化模板导出”。 |
| 协议文件导入／发现 | 特定协议可生成资产／属性，导入文件格式由协议决定。[D2][D3] | 不是通用设备模板导入，不证明任意 CSV／JSON 模型包、跨项目依赖重映射或品牌转换。 |
| 同类型多实例 | 已加载类型可以反复建实例；实例能分别配属性和链接。[D1][D3] | 不能直接等同 FastIOT 的模板批量填差异、显式覆盖、版本引用和应用语义。 |

**未知且本次不扩搜：** 通用资产包导入导出的覆盖范围、整棵子树复制的引用重写、复制时全链路秘密过滤、模板与实例的版本联动、导入冲突预览是否与 FastIOT 要求一致。本次五份新增来源不足以裁定这些能力；不写成“不支持”，也不把协议专用导入升级成通用模板能力。

## 3. 单位、约束与用途怎样驱动编辑和展示

### 3.1 已证实的机制

- **类型驱动输入形状。** 属性输入组件解析 AttributeDescriptor / ValueDescriptor，选标准输入 provider；对适用的复杂类型，获取值描述符 schema 并交给 JSON Forms，保留校验接口。它证明描述可驱动表单，不证明任意领域动作已自动生成。[N4]
- **单位和格式驱动表达。** 描述符有 `units`、`format`，实例也可配置 Units、Format、Label；单位可由字符串数组组合，例如 `km/h`。[D1][D3] 修改显示单位不等于已经转换设备数值；入站／出站转换另属链接及协议配置。[D2]
- **约束不是“最后配置者覆盖”。** 官方架构文档说明 `ValueUtil.validateValue()` 累积检查四个来源：值类型约束、属性描述符约束、描述符 meta 中的 constraints、实例 meta 中的 constraints；值须通过全部约束。[N5] 这是该文档所定义的验证逻辑，不是本次运行验证了所有 UI/API 入口。
- **用途由配置项影响行为。** Read only、Momentary、Multiline、Rule state、Store data points 等分别参与输入、规则可选项或历史行为；属性输入组件也检查写属性角色和只读信息。[D3][N4] 这些配置不等同一套明确区分“观测／配置／目标／反馈”的领域用途枚举。

### 3.2 对 FastIOT 的含义：复用机制，不照搬字段

FastIOT 已在 [C] §3 规定类型、单位、枚举、用途、历史方式、命令与当前可用性；[M] §4–5 已规定语义模型与 Profile 分层。因此以下是**现有要求的任务化表达，不作为新增建议计数**：

| 用户在定义什么 | 编辑与展示应由什么驱动 | 不能从什么猜出来 |
| --- | --- | --- |
| 压力观测 | 数值类型、工程单位、允许范围、来源／质量／时效；生成读数和趋势用途 | 属性名字含“压力”，不等于可写设定；最后有效值不等于当前有效。 |
| 运行反馈 | 已声明布尔或枚举含义、反馈来源、只读方向 | 两个厂商都用 `1`，不证明 `1` 都表示运行；名为“运行”的可写字段也不是反馈。 |
| 目标设定 | 目标用途、参数单位／范围、明确命令入口及当前允许原因 | 有数值输入框或协议支持写入，不等于该用户可以控制。 |
| 现场配置 | 模板默认与实例覆盖的来源、允许修改范围 | 配置变化不能记成设备实测；单位标签不能替代缩放。 |

同样是数值，`ratedPressure`、`pressureTarget`、`pressureActual` 的角色不同；同样是布尔，“允许启动”“启动请求”“运行反馈”也不能共用一个含混成员。OpenRemote 提供“描述驱动行为”的机制参考，FastIOT 的业务用途、Command 和质量契约仍以本地规格为准。[C][M]

## 4. 与 FastIOT 对象的映射及不可等同处

以下是**设计对照**，不是 OpenRemote 已提供全部 FastIOT 概念的声明。

| FastIOT 概念 | 可借鉴的 OpenRemote 表达 | 不可等同之处／FastIOT 必须保留的边界 |
| --- | --- | --- |
| Site | 顶层业务 Asset 可表达一个现场。[D1] | Site 不等于 Realm：Realm 是隔离范围，业务现场不因此必须新建隔离域；Site 也不自带调度能力。[M] |
| Area | Building、房间等父子 Asset 的组织方式。[D1][D3] | Area 是组织容器，不能因为是父资产就自动控制子设备；层级按实际跳过。[M] |
| Unit | 逻辑 Asset 可作为对外业务对象的承载形式。[D1] | 单个逻辑 Asset 或父子树不能直接证明具备成员角色、必需／可选输入、组合动作、能力可用性和共享控制目标约束。[M][C] |
| Device | 物理对象 Asset 的独立实例身份。[D1] | Device 不是 Agent；Agent 表达协议实例及配置，一条连接也不必须一一对应一台业务设备。[D2][M] |
| Point | Attribute 可承载测量值，AgentLink 可指向数据来源。[D1][D2] | OpenRemote Attribute 还可装连接配置或控制数据，不能全部叫 Point；FastIOT Point 的质量、时间来源、读写方向和转换不可因映射而省略。[M][C] |
| Group | 父资产组织子资产可参考其展示组织；官方另有显式配置的父子群控方案。[D3] | FastIOT Group 是可跨 Area 引用的横向集合，不要求搬动父节点；展示组不自动广播。既有 OpenRemote 父子群控不等同该横向成员契约。[R][M] |
| 模型／模板 | 类型描述符可参考公共定义，实例复制可参考减少重复录入。[D1][N1] | FastIOT 模型定义语义；模板组合模型、默认参数及 Unit 成员角色。不能用“复制一份实例 JSON”替代模板来源、版本引用与显式覆盖。[M][C] |
| Profile | AgentLink 的协议定位与入／出站转换，加上 Agent 的连接配置，可参考映射边界。[D2] | 没有证据证明它就是 FastIOT 的独立厂商 Profile。Profile 还要说明编码、单位、枚举与命令如何实现模型语义；协议适配负责通信，实例填现场连接，三者不合并。[M][C] |

**业务／协议解耦的准确说法：** 属性通过明确链接访问协议，所以业务父节点不必就是通信节点；但某些协议导入会先把资产放在 Agent 下，且用户配置的父子群控本身依赖层级。[D2][D3] 因此不能泛称“OpenRemote 任意拖动树完全不影响逻辑”。FastIOT 已明确把展示、业务归属、反馈来源与控制关系分开，应该沿用此契约，而不是照搬父子树承载所有关系。[M] §3；[C] §3。

## 5. FastIOT 的具体用户任务路径

**这是按现有规格组织的目标工作流，不是新增功能承诺或已实现操作说明。** 场景：交付人员建立两台同模型泵及压力测量；需要组合业务能力时才建立“供水 Unit”。使用已支持协议与 Profile，不引入新的 MQTT、KNX 等支持承诺。[M][C]

| 步骤 | 用户完成的任务 | 必须可见的结果／停下来的条件 | 对应现有验收 |
| --- | --- | --- | --- |
| 1. 定对象 | 选择 Site，按需建泵房 Area；建立 Device。只有实际需要组合功能才选供水 Unit 模板。 | 普通 Device 可直接使用；Area 不要求动作和能源字段。Unit 不是创建每台设备的强制前置。 | T40 |
| 2. 定语义 | 选已有设备模型／模板；核对运行反馈、压力观测及启动／停止命令等实际支持成员，声明类型、用途、单位、约束、必需性与反馈依据。 | 明确哪些是属性、哪些是动作；型号不支持的操作不出现为可用能力。这里是 FastIOT 模型编辑要求，不借 OpenRemote 实例编辑冒充已证明的无代码类型能力。 | T28、T40 |
| 3. 建实例 | 由同一模板创建泵 A、泵 B，填写独立名称、现场参数、型号／Profile 版本与覆盖项。 | 公共定义与实例差异可区分；模板草稿不改变已生效实例，现场凭据及控制授权不随公共定义复用。 | T34 |
| 4. 配接入 | 选已支持协议和 Profile，填写连接、站号及实际点位；导入点表时先看变化与错误。 | 连接是连接、设备是设备；错站号、编码与点位错误定位到对象，不以通道打开判定设备可用。 | T27、T33；服务 T28 |
| 5. 绑数据 | 把解码后的 Point 绑定到 Device 属性；核对原始值、工程值、类型、单位、枚举、质量和时间来源。 | 例如假设 A 输出 `250 kPa`、B 输出 `2.5 bar`，两个已适配 Profile 都显式转换为 `0.25 MPa`。只把单位文字改成 MPa 不算完成。缺失必需映射或反馈语义不等价时不能自动通过。 | T28、T34 |
| 6. 绑动作 | 为模型命令选择实际设备动作、参数和独立反馈；如使用 Unit，再绑定执行、测量等角色与对外数据／动作。 | 列出未绑成员及影响；运行枚举成功映射不证明启停时序或反馈等价。缺项仅限制依赖功能；同一受控目标不能从多个入口取得独立竞争写权。 | T28、T40；组合复用 T36–T39 |
| 7. 看与复用 | 画面、规则、报警和历史引用稳定业务成员；查看同一模板两台设备的读数、用途、覆盖和可用命令。 | 改显示名或通信地址不需要改画面／规则代码；A 的参数不改变 B。展示 Group 可跨 Area 引用，但不复制 Device 或隐式扩大控制范围。 | T28、T34、T40 |
| 8. 证明绑定成立 | 在规定的模拟／验收条件下核对样值和命令反馈，再明确应用配置；更换实际设备时重新核对映射与能力。 | 数据可读不等于控制已获准；应答／写值回填不算到位。换机有新 Device 身份、旧历史仍属旧设备，不接收旧命令；有 Unit 时可在语义等价前提下保留对外引用。 | T28、T34、T40；动作结果沿用 T31 |

无 Unit 的直接设备换机引用处理，仍遵守 [M] §7 对 US-40 尚待场景确认的保留，不在本专题偷偷定为“原 ID 改绑即可”。新厂商若缺独立运行反馈、模式不同、范围不足或未有适配 Profile，应明确未满足项；不能让“同名属性”“同一值类型”或复制成功代替适配证明。

## 6. 只补两项建议，不重复已覆盖项

业务／协议分离、稳定引用、可选 Unit、横向 Group、模板与实例差异、缺失／不等价映射、用途驱动控件，以及模板草稿隔离，均已在 [M]、[C] 覆盖；不再列成新建议。以下仅建议把两个边缘场景纳入原编辑与验收，**尚未吸收进规格，更未实现**。

| 建议与依据 | 最小采用 | 反例验收 | T 映射 |
| --- | --- | --- | --- |
| **1. 让有效约束及来源可解释，实例不能静默放宽模型硬约束。** OpenRemote 官方验证文档明确多个约束来源累积生效；FastIOT 已有“范围＋实例覆盖”，但未写清冲突时怎样解释。[N5][C] | 只处理当前已支持的类型／枚举／数值范围：区分可覆盖默认值与不可放宽的模型约束，在原编辑位置展示模型范围、实例限制和最终有效范围；冲突无解则阻止该配置生效。无须引入约束语言、继承框架或新校验中心。 | 模型允许 `0–100`，实例填写最大值 `150`，不能让 `120` 通过；实例缩为 `0–80`，`90` 应说明被哪个来源限制；实例最大值 `-1` 与模型最小值 `0` 冲突时不能保存成貌似有效配置。属性存在但值为空时，还须按非空要求判定，不能用“必需成员已建”代替值有效。 | T28：类型／范围一致；T34：覆盖可解释；T40：同一机制用于非能源对象。 |
| **2. 复制配置与复制运行态明确分界。** OpenRemote 单资产复制函数未逐项清除属性当前值／时间戳；FastIOT 已要求独立身份与现场参数，但尚未把这一“看似已在线”的反例写细。[N1][N2][C] | 在既有复制／批量建实例能力里，保留声明的配置默认值；不把源设备最近观测、观测时间、命令执行状态作为新设备有效运行态。新实例的数据有效性必须来自其实际绑定及新的采集；不新增模板中心、升级或清洗向导。 | A 当前运行、压力 `0.25 MPa` 且反馈有效；复制创建尚未接入的 B 后，B 不得显示“实测运行”或继承有效反馈，不得因复制而发送命令。B 接入后只显示 B 的观测；模板中的配置默认值可以保留，但不能进入 B 的实测历史。秘密／授权／旧绑定的隔离仍执行已有要求，不另立建议。 | T34：独立实例与复用；T28：值、质量和反馈含义一致；T40：Device 与 Unit 复用都不伪造观测。 |

## 7. 保留的未知与决策边界

1. **无代码类型发布：** 当前证据支持“Java 配置类型＋UI 配实例”，不支持把参数化实例改称可视化类型发布；也不以此否认未来版本或其他扩展路径。[D1]
2. **模板／导入完整性：** 已证实单资产复制和协议专用导入；通用模板包、依赖重写和版本应用保证本次未知，不为填满表格继续扩搜。
3. **约束的一致执行：** [N5] 给出验证逻辑，[N4] 给出 UI 描述驱动路径；本次未跨 UI、API、协议更新逐一实测，不宣称所有入口、所有类型都已端到端验证。
4. **业务等价与设备可控性：** 格式、单位标签、枚举和值类型都只是等价检查的一部分；范围、模式、反馈、物理行为及当前授权仍需独立核对。[M][C]
5. **采用边界：** 借鉴描述、校验、实例配置与绑定的分工，不复制 Java 类层次，不改变 FastIOT 对象职责、不增加自由脚本，也不把 OpenRemote 的协议清单变成 FastIOT 首版承诺。

## 来源

### 既有研究与本地规格

- [R] [OpenRemote：对 FastIOT 的可借鉴边界](research-openremote-2026-11-14.md)：复用其中 §模型与接入、用户路径、控制反馈及 D1–D3 证据；部署、许可、规则不重新展开。
- [M] [通用物模型](specs/2026-11-13-generic-thing-model.md)：重点 §2–5、§7–8，Site/Area/Unit/Device/Point/Group、模型／模板／Profile／实例与 T40。
- [C] [平台能力细则](specs/2026-11-13-core-capabilities.md)：重点 §2–3、T28/T34，必要处交叉引用 T27/T33/T31；是需求基线，不是实现证据。

### 复用的一手来源（不计新增）

- [D1] [Assets, Agents and Attributes，固定文档快照](https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/030-assets-agents-and-attributes.md)：Asset Type Model、四类描述符、optional、Thing 与 Java 配置边界。
- [D2] [Agents & protocols overview，固定文档快照](https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/040-agents-protocols/010-overview.md)：AgentLink、入／出站转换、协议发现／专用文件导入。
- [D3] [Manager UI，固定文档快照](https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/020-manager-ui/10-manager-ui.md)：创建与编辑实例、meta 配置、协议导入入口、父子组织与显式群控。

### 本次新增的五份一手来源

- [N1] [资产树 `or-asset-tree/src/index.ts`](https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/component/or-asset-tree/src/index.ts)：`render` 的 Duplicate 按钮条件；`_onCopyClicked`（1527–1560 行）复制与字段清除；`_onAddClicked`（1589–1627 行）从描述符创建属性。
- [N2] [Manager `page-assets.ts`](https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/app/manager/src/pages/page-assets.ts)：`_onAssetAdd`、`_doSaveAndLoad`，复制／新增在编辑模式与非编辑模式的不同处理。用于确认复制确实接入新增流程，不据函数名推测模板能力。
- [N3] [创建资产对话框 `or-add-asset-dialog.ts`](https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/component/or-asset-tree/src/or-add-asset-dialog.ts)：`getTypeTemplate` 区分固定与可选属性，用户选择已加载类型，不是编辑类型 schema。
- [N4] [属性输入组件 `or-attribute-input/src/index.ts`](https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/component/or-attribute-input/src/index.ts)：`_updateDescriptors`、`_updateTemplate`、`jsonFormsInputTemplateProvider`、`isReadonly`，描述驱动输入、复杂值 schema 与只读边界。
- [N5] [Asset Validation，官方架构文档](https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/architecture/40-asset-validation.md)：四个约束来源累积生效，静态描述符与运行时实例 meta 的区别。只引用其定义的验证逻辑，不外推为本次实测结果。

[R]: research-openremote-2026-11-14.md
[M]: specs/2026-11-13-generic-thing-model.md
[C]: specs/2026-11-13-core-capabilities.md
[D1]: https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/030-assets-agents-and-attributes.md
[D2]: https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/040-agents-protocols/010-overview.md
[D3]: https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/user-guide/020-manager-ui/10-manager-ui.md
[N1]: https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/component/or-asset-tree/src/index.ts
[N2]: https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/app/manager/src/pages/page-assets.ts
[N3]: https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/component/or-asset-tree/src/or-add-asset-dialog.ts
[N4]: https://github.com/openremote/core/blob/c7652131fe2b36f7b4341a0d8a20a94fc8107667/ui/component/or-attribute-input/src/index.ts
[N5]: https://github.com/openremote/documentation/blob/c57c4c260daf8fa5ebc32e7c851a9accd9a6a155/docs/architecture/40-asset-validation.md
