# FastIOT：成熟平台对标与规格完善建议

研究日期：2026-11-13。对象：当前本地产品总规格与平台能力细则。仅研究官方文档，不操作 ThingsBoard、openHAB、Home Assistant 或真实设备，不修改产品规格、代码或 GitHub。

本报告将成熟产品实践与 FastIOT 建议分开。借鉴的是用户如何接入、建模、复用和操作，而不是复制厂商架构、付费功能范围或管理后台。已删除的独立检查、升级恢复和“加入 EMS”流程不恢复。

## 结论

**不建议照着四个平台补功能菜单。当前规格真正值得新增的主要有两项：参数化设备画面、参数化通用规则模板；其余以语义澄清与验收反例为主。**

建议分三类处理：

- **新增必要复用能力**：一份画面绑定多个设备实例；一份决策表模板绑定不同设备及参数。
- **细化现有能力**：观测与配置用途、支持能力与当前许可、业务关系角色、部分不可用、报警过程身份。
- **保留而不重复扩建**：权限、真实设备反馈、断网运行、历史质量与授权导出；只补反例，不再增加控制／检查后台。

本报告没有修改规格，也没有把建议变成已确认决定。

## 对标对象各自适合回答什么

| 参考对象 | 最有用的参考 | 不应直接复制 |
| --- | --- | --- |
| Ignition Perspective 8.1 | 参数化视图、模板实例复用、设备对象与操作页面衔接 | 专有设计器、整套组件平台或每设备重新生成页面 |
| ThingsBoard | 设备 Profile、属性与时序、实体关系、dashboard alias、规则复用、报警过程 | 把 Profile 当完整物模型；消息 RPC 状态当物理动作成功；消息重试机制套入工业命令 |
| openHAB | Binding／Thing／Channel／Item 分离、语义用途、页面 props | 照搬多对多关联复杂度、组命令隐式广播、预测状态当实际反馈 |
| Home Assistant | Integration／Device／Entity 区别、实体可用性和能力、自动化 blueprint | 把家居空间分组当 PCC 或控制归属；默认引入 YAML、helpers、云依赖或整套生态 |

这些名称没有一一对应关系：ThingsBoard Device Profile 侧重公共传输与运行设置；openHAB Item 是应用使用的状态／命令对象；HA Entity 表达一种观测或功能；Ignition UDT 是可复用标签定义。FastIOT 仍应以自己的 Device／物模型／模板／Profile 分工表达需求，不为了模仿产品而改名。

家庭自动化实践可以帮助降低使用成本，但不能证明工业设备的命令确认、安全保护或跨品牌等价。

## 成熟 SCADA 的明确补充：参数化画面复用

Ignition Perspective 8.1 的 Embedded Views 文档说明：同一个 View 可同时正常显示和嵌入其他 View，嵌入时传入与目标参数名称匹配的值；官方例子描述同类设备列表与单个设备详情视图的复用。这是产品实践，不是要求 FastIOT 实现它的设计器或控件树。

来源：[Ignition 8.1 Embedded Views](https://www.docs.inductiveautomation.com/docs/8.1/ignition-modules/perspective/views-in-perspective/embedded-views)。

**当前规格已覆盖：** 模型／设备模板复用、稳定引用、AI 对话生成网页，以及修改时保留绑定。

**尚需补清：** 这些要求不等于同类设备已经能复用同一画面。若每加一台泵都让 AI 重新生成整页，设备越多，风格和绑定偏差越多。

**建议条款（未应用）：**

> AI 生成的同类设备画面可通过明确的设备／模型参数复用；数据与命令绑定依据当前实例解析，不将某台设备地址写死在画面中。改变实例绑定后，名称、实时值、报警和操作目标一致切换；模型不匹配时明确提示，而不是自动猜测。

**最小验证：** 同一个泵画面展示两台不同实例；对第一台操作只影响第一台，切换到第二台后不能使用第一台的旧目标；换已适配 Profile 不要求重做布局。保留 AI 修改方式，不增加强制手工拖拽编辑器或多层画面继承系统。

对应：故事 11–14、22、65–66；T05、T16、T28、T31、T34。

## ThingsBoard 对标

完整证据见 [ThingsBoard 专项笔记](research-thingsboard-spec-advice-2026-11-13.md)。

1. **Profile 与物模型不能混为一谈。** 官方 Profile 共享传输、规则入口和报警配置；属性与时间序列也有不同用途。当前 FastIOT 模型定义更强调属性、命令、事件和语义，不能因采用类似 Profile 名称就丢掉这些要求。[Device Profiles](https://thingsboard.io/docs/user-guide/device-profiles/) · [Entities and Relations](https://thingsboard.io/docs/user-guide/entities-and-relations/)
2. **Alias 支持复用画面，而不是复制设备路径。** 页面状态可传入实体参数，alias 根据参数解析对象。同一布局换实体可以复用。FastIOT 应规定单个控制目标不能解析成多个不明确对象；这是我们的失败处理建议，不是官方 alias 文档对所有错误情况的保证。[Aliases](https://thingsboard.io/docs/user-guide/aliases/)
3. **规则链可以抽取复用逻辑，但不需要照搬图引擎。** ThingsBoard 的 Rule Chain 是有向处理图；FastIOT 可以借鉴“定义加参数绑定”，保留已经选择的决策表和状态机表达，不引入任意脚本或消息处理平台。[Rule Engine](https://thingsboard.io/docs/user-guide/rule-engine-2-0/overview/)
4. **RPC 返回层次与执行机构结果不同。** 单向 RPC 成功仅表示发送；持久 RPC 的 SUCCESSFUL 表示设备回复。离线积压与自动重试是其可用机制，但不能照搬到已经决定不自动重发的 FastIOT 控制路径。[RPC](https://thingsboard.io/docs/user-guide/rpc/)
5. **报警有稳定的过程身份。** ThingsBoard 按 originator 与 type 维持活动报警，严重度变化可更新原过程；确认与清除分别处理。FastIOT 更适合显式绑定实例与稳定报警定义，避免仅以显示名称去重。[Alarms](https://thingsboard.io/docs/user-guide/alarms/)

**版本与商业限制：** 本次主要读取带 CE 标识的滚动文档，不足以确认所有功能当前是否为 CE／PE 独占。RPC 官方页确有 v4.4 起 source-available 提示，因此不能凭旧印象将当前所有代码或功能称作 Apache 开源、免费或可任意复制。这里借鉴行为设计，不引入其代码或许可义务；FastIOT 自己的许可情况须独立判断。

## openHAB 与 Home Assistant 对标

完整证据见 [openHAB／HA 专项笔记](research-openhab-ha-spec-advice-2026-11-13.md)。

- **连接与应用对象分离。** openHAB 把设备接入、Channel 和应用 Item 分开；语义模型区分位置、设备、点和属性。应借鉴“地址不等于业务身份”，不把所有 Link 或 Group 行为原样搬入。[Concepts](https://www.openhab.org/docs/concepts/) · [Semantic Model](https://www.openhab.org/docs/tutorial/model.html)
- **能力、可用性与权限是不同问题。** HA 的 Entity 可以表达观测或功能；开发文档有支持特性与可用性描述。FastIOT 的设备可能支持启动，但此刻因互锁、权限或关键数据失效而不能启动。型号能力不应随着登录用户改变。[HA 概念](https://www.home-assistant.io/getting-started/concepts-terminology/) · [官方 Entity 文档源码](https://github.com/home-assistant/developers.home-assistant/blob/master/docs/core/entity.md)
- **部分不可用要如实表达。** 同设备的压力无数据、运行反馈正常时，不应把整台设备画成全好或全坏。FastIOT 已有质量概念，主要缺少这种混合场景的验收，而不是缺一个健康中心。
- **预测反馈不应移植成工业确认。** openHAB 的 autoupdate 可以预测命令后的 Item 状态，Group 命令可转发给成员。FastIOT 不应据此让页面预先显示实测运行，也不应由展示组暗中产生批量设备动作。[Items：Command vs. Status](https://www.openhab.org/docs/configuration/items.html)
- **模板让使用者填少量参数。** openHAB 页面 props、HA blueprint 都展示了复用定义与实例输入的价值；HA subview 则只是导航关系，不能当作参数化模板或权限保护。[Building Pages](https://www.openhab.org/docs/ui/building-pages.html) · [Automation blueprints](https://www.home-assistant.io/docs/automation/using_blueprints/) · [HA Views](https://www.home-assistant.io/dashboards/views/)

这些来源没有证明设备跨品牌等价，也没有证明 Area、Group 或 Equipment 必须转成通用 Unit。业务展示归属与设备控制所有权仍须分开。

## 合并后的最小修订清单

以下是建议条款，**尚未写回规格**。新增两项，其余细化现有内容；不新增检查、升级、包安装或健康管理模块。

### P1 — 参数化设备画面【优先新增】

来源：Ignition Embedded Views、ThingsBoard aliases、openHAB props；HA subview 不作为模板证据。

> 同类设备可复用一份 AI 生成画面或片段。画面声明所需模型成员与实例参数，使用时绑定具体设备；目标缺失、模型不匹配或单目标解析为多个对象时，不允许猜测控制目标。切换实例时，名称、数值、报警与操作目标保持一致。

**验收补充：** T05／T16／T31／T34 中用一份画面切换两台泵；操作只影响当前设备，旧页面请求失效。无需复制代码或再让 AI 重画一套；修改仍由 AI 完成，不增加强制手工编辑器。

### P2 — 通用规则模板的类型化输入【优先新增】

来源：HA blueprints、ThingsBoard 规则复用思想；不采用其全部执行机制。

> 通用决策表可保存为模板，声明设备／模型输入、动作目标及可调参数。实例化时只填写实例差异，错误类型或缺失输入在编辑处指出；保存与生效分开，参数不默默引用原模板示例设备。

**验收补充：** T29／T26 中用一份液位报警模板建立两份独立实例；调整 A 的阈值不影响 B，模拟 A 不请求 B。复用已有 FIRST、持续和冷却语义，不引入 YAML、任意脚本、图式消息平台或模板升级合并。

### P3 — 模型成员的最小用途语义【细化】

来源：ThingsBoard 属性／时序区分、openHAB 语义模型。现有规格已有类型、单位、稳定身份和标签，不应再次建设模型体系。

> 模型成员说明是观测、配置、目标设定、反馈还是操作，以及是否记录历史。AI 和通用控件依据声明选择展示与绑定，不从显示名称猜测用途。配置变化可以留痕，但不得冒充传感器实测。

**验收补充：** T28／T34：额定值、设定目标与实时反馈同屏时含义明确，修改额定值不改写实测历史。仅补必要语义，不复制完整家庭分类或本体系统。

### P4 — 展示归属不等于控制关系【细化】

来源：ThingsBoard assets／relations、openHAB 语义及功能分组、HA Areas。现有规格已有同实例去重和 PCC 关系。

> 区域、设备列表和功能视图可以引用同一实例；展示分组、反馈关系与受控资源关系各自含义明确。改变显示位置或标签不隐式改变控制归属、授权或统计口径；展示组不自动广播控制命令。

**验收补充：** T15／T28／T34／T35：同设备出现在区域和 PCC 视图只统计一次；拖换展示归属不会改策略作用对象。这里不要求新增拖拽功能或任意关系图编辑器，只要求既有编辑行为不混淆关系。

### P5 — 支持能力与当前可操作性分开【细化】

来源：HA Entity 能力和可用性描述；权限、互锁判断属于 FastIOT 已有约束，不能由 HA 文档代为保证。

> 型号模型提供支持的操作及参数约束；平台另给当前用户、当前设备状态下是否允许操作及原因。生成页面区分“不支持”与“暂不可用”，不因用户权限不同而修改设备型号定义。

**验收补充：** T28／T31／T35：只读设备没有虚构的启停；可控设备遇互锁或无权限显示相应原因，绕过按钮仍被服务端拒绝。不新增权限 DSL 或能力市场。

### P6 — 部分不可用用具体反例验证【只补用例】

来源：HA Entity 可用性；FastIOT 已有点位质量及设备部分错误定义。

> 同一设备不同观测可以有不同质量；局部失效不能被汇总成全部正常，也不能无条件抹掉其他有效观测。是否允许某动作取决于它所需输入，而不是仅凭一个全局在线图标。

**验收补充：** T27／T28／T33：压力失联、运行反馈仍有效；旧压力标过期，依赖压力的控制受约束，正常反馈继续显示。只是补足现有结果，不做健康评分或新后台。

### P7 — 报警用稳定定义标识过程【细化】

来源：ThingsBoard originator＋type；结合现有 FastIOT 报警过程语义，不机械复制厂商键格式。

> 报警过程关联设备实例与稳定报警定义标识，而非仅用显示名称；同一持续异常的严重度变化更新原过程，恢复后再次触发可区分为新过程。发生、最近触发、接手和恢复的时间含义明确。

**验收补充：** T32：两个同名但不同定义的报警不合并；一次异常升严重度不重复建报警；恢复后再发能够单独追查。确认接手与物理恢复仍分开，不加第二套报警管理流程。

### 已覆盖，不再堆需求

RPC 回复不等于动作成功、按钮禁用不等于授权、数据过期不能伪装正常、历史导出按权限、正式运行不依赖 AI 在线等，当前规格已经明确。只在原测试里补 openHAB 预测状态、ThingsBoard RPC 回复但设备拒动等反例，不增加新的“提交／仲裁”界面。

**建议优先级：** 先完善 P1、P2 的复用行为；P3–P5、P7 补入现有模型与操作定义，P6 只加测试场景。它们都应服务“配置一次、多处复用、当前对象可安全操作”，而不是用于扩大平台目录。

## 证据边界与来源

### 详细研究笔记

- [ThingsBoard：六项对照与来源](research-thingsboard-spec-advice-2026-11-13.md)
- [openHAB／Home Assistant：六项对照与来源](research-openhab-ha-spec-advice-2026-11-13.md)
- [既有行业范式研究](industry-paradigms-2026-11-13.md)：包含 OPC UA、DMN、SCXML、历史数据与能源模型，不在本轮重复扩展标准研究。

### 版本与证据限制

- Ignition 本轮使用明确的 8.1 文档。其他主要为读取当日的官方滚动文档，不冒充冻结发行版。
- ThingsBoard 文档中的 CE 标识不足以证明每项功能当前免费或 PE 独占；v4.4 source-available 提示已由主会话在官方 RPC 页再次核对，未据此作完整许可或版本权益结论。
- Home Assistant Entity 的原开发文档地址重定向后，研究改读官方源码；底稿记录文件 blob。未把重定向网站当官方来源。
- 主会话独立核对 Ignition 参数化视图、ThingsBoard alias／RPC、openHAB command／status 与 autoupdate。其余出处及限定条件详见底稿。
- 没有登录或操作本机 HA、openHAB、ThingsBoard，没有模型调用或设备测试；文档行为不是 FastIOT 的实现证明。
- 本轮只新增研究报告，未修改现有规格、术语表、产品代码或 GitHub。

### 核心一手来源

1. [Ignition Embedded Views](https://www.docs.inductiveautomation.com/docs/8.1/ignition-modules/perspective/views-in-perspective/embedded-views)
2. [ThingsBoard Device Profiles](https://thingsboard.io/docs/user-guide/device-profiles/)
3. [ThingsBoard Entities and Relations](https://thingsboard.io/docs/user-guide/entities-and-relations/)
4. [ThingsBoard Aliases](https://thingsboard.io/docs/user-guide/aliases/)
5. [ThingsBoard Rule Engine](https://thingsboard.io/docs/user-guide/rule-engine-2-0/overview/)
6. [ThingsBoard RPC](https://thingsboard.io/docs/user-guide/rpc/)
7. [ThingsBoard Alarms](https://thingsboard.io/docs/user-guide/alarms/)
8. [openHAB Concepts](https://www.openhab.org/docs/concepts/)
9. [openHAB Semantic Model](https://www.openhab.org/docs/tutorial/model.html)
10. [openHAB Items](https://www.openhab.org/docs/configuration/items.html)
11. [openHAB Building Pages](https://www.openhab.org/docs/ui/building-pages.html)
12. [Home Assistant Concepts](https://www.home-assistant.io/getting-started/concepts-terminology/)
13. [Home Assistant Entity 官方源码](https://github.com/home-assistant/developers.home-assistant/blob/master/docs/core/entity.md)
14. [Home Assistant Automation Blueprints](https://www.home-assistant.io/docs/automation/using_blueprints/)
15. [Home Assistant Views](https://www.home-assistant.io/dashboards/views/)
