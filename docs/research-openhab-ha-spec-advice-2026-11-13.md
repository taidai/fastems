# openHAB / Home Assistant 对 FastIOT 的规格建议

2026-11-13。对照本地《产品总规格》附录 A/B/D 与《平台能力细则》§1–8；以下判断的是规格覆盖，不是实现完成度。查阅 8 页官方资料，10 次网络读取（含一次重定向和一次来源核对）；未调用模型、设备或本地 HA。

## 可借鉴什么

- **openHAB**：Binding 负责设备接入，Thing 暴露 Channel，Item 是应用使用的状态／命令对象，Link 连接 Channel 与 Item，可多对多。[1] 这支持 FastIOT 的接入、点位、物模型分层，但 Binding 不等于声明式 Profile，Item 也不等于完整设备模型。语义模型主要用 Group 成员关系和标签表达 Location／Equipment／Point／Property；功能组可在物理层级之外。[2]
- **HA**：Integration 接入外部系统，Device 聚合 Entity；Entity 表达观测或功能，不必对应物理设备，Area 组织空间。[5] `supported_features` 是声明能力，`available` 是当前可用性；`assumed_state` 明确允许假定状态。[6] action 是操作入口，不是工业执行确认凭证；概念页中的 service 还可指信息提供者，不能混为同一控制概念。[5]
- 两者借鉴价值是减少重复配置、统一引用和展示，不是证明跨品牌／协议等价。FastIOT 故事 40、T14 的重新绑定和验证不能省略；区域分组也不能替代 EMS 的 PCC／控制归属。

## 六项最小修订

### 1. 给模型补最小用途元数据

**收益**：AI 不靠名称猜测观测和操作。**已覆盖**：故事 65、细则 §3、T28/T34 已有模板、稳定身份及业务树；**缺口**：标签、展示用途与控制归属的界线不够显式。

**最小范围**：仅补观测／状态／设定／操作用途及可选展示分组；复用现有区域和设备身份，不复制整套家居分类。[1][2]

**验收**：T28/T34 增例：同一泵出现在区域页及功能列表，仍是一个实例；改标签不改变绑定，不新增写权限或重复累计。

### 2. 分开“支持操作”和“现在允许操作”

**收益**：操作员知道是设备不支持，还是暂时不能用。**已覆盖**：故事 5/14/22/35、细则 §3/6/8、T28/T31/T35 已规定参数、权限和不可用原因；**缺口**：生成页面消费的能力描述尚不够明确。

**最小范围**：模型提供支持项及参数约束；平台另给当前允许状态与原因，不用动态权限改写型号能力。[6]

**验收**：同一画面绑定只读设备不出现虚构控制；绑定可控设备遇互锁显示原因；伪造请求仍由服务端拒绝。

### 3. 补“部分不可用”的混合场景

**收益**：一个传感器坏了，不误判整台设备全好或全坏。**已覆盖**：故事 7/64、细则 §1/2、T27/T33 已有部分点错和质量；**缺口**：缺少同设备混合质量的明确反例。

**最小范围**：只补 T27/T28/T33 用例，不另建健康中心；借鉴 HA 的实体粒度可用性，但保留 FastIOT 更细的过期／坏值／来源。[5][6]

**验收**：压力失联、运行反馈正常时分别显示；保留旧压力也必须标过期，依赖压力的动作被阻断，正常观测不被抹除。

### 4. 禁止“界面预测”冒充设备反馈

**收益**：按钮成功不再被看成启动成功。openHAB 明确区分 command/status，`autoupdate` 可预测状态；Group 命令还会转发给成员。[3] 这些不是工业执行保证。

**已覆盖**：故事 22/24/37、T04/T30/T31 已很充分。**最小范围**：仅补 UI 反例，不新增命令阶段流程。

**验收**：模拟器拒动时，按钮动画、目标值、规则命中都不能更新实测运行状态；批量动作不得靠显示组隐式广播，部分成功不能显示全部成功。

### 5. 页面按实例参数复用，而非复制绑定

**收益**：两台泵共用展示定义，不误控另一台。**已覆盖**：故事 13/14/21/65、T05/T19/T34 有源码和模型复用；**缺口**：未明确页面的实例参数契约。

**最小范围**：AI 生成的可复用页面／片段声明所需模型成员，使用时绑定实例；仍通过对话或标注修改，不要求手工编辑器。openHAB 的 props 和页面／widget 共享可参照。[4] HA subview 仅提供导航，不是参数化模板；隐藏页签也不是鉴权。[8]

**验收**：T31 增例：一份页面绑定两泵，切换后读写目标一致；旧页面请求和直接访问隐藏页面仍接受对象鉴权。

### 6. 把普通规则复用收敛成“填参数”

**收益**：复用逻辑时只填设备与阈值。**已覆盖**：故事 6/41/68、细则 §4/8、T19/T29/T35 有规则资产及 EMS 模板；**缺口**：普通规则尚未明确模板输入。

**最小范围**：一个液位报警规则模板，声明类型化对象、阈值和持续时间；借鉴 blueprint 同一逻辑绑定不同输入。[7] 不引入 YAML、自由脚本或通用 helper 系统；派生聚合沿用 T33/T34。

**验收**：T29/T26 增例：同模板生成两个独立实例，错类型在保存处指出；改一份参数不改变另一份，模拟零真实写入，保存不等于生效。

以上均嵌入原编辑或使用动作；不恢复独立检查、升级／恢复、加入 EMS 流程，不引入云端依赖或生态市场。

## 官方来源

[1] openHAB — Concepts，Things, Channels, Bindings, Items and Links：
https://www.openhab.org/docs/concepts/

[2] openHAB — Semantic Model，Introduction to the Ontology and Relationships / Modifying the Model / Non-semantic Items and Groups：
https://www.openhab.org/docs/tutorial/model.html

[3] openHAB — Items，Command vs. Status / Parameter autoupdate / Derive Group State from Member Items：
https://www.openhab.org/docs/configuration/items.html

[4] openHAB — Building Pages，Root Components & Props / Widgets: Definition & Usage / Types of Actions：
https://www.openhab.org/docs/ui/building-pages.html

[5] Home Assistant — Concepts and terminology，Integrations / Devices / Entities / Areas / Actions：
https://www.home-assistant.io/getting-started/concepts-terminology/

[6] Home Assistant 官方开发文档源码 — Entity，Generic properties / Additional properties；原 developers 文档地址重定向后，改读官方仓库，未访问重定向站点。文件 blob SHA：`19ccb1b6b222d9f8bfaf8ab201844ab277e1fb01`。
https://github.com/home-assistant/developers.home-assistant/blob/master/docs/core/entity.md

[7] Home Assistant — Using automation blueprints，Creating an automation from a blueprint / Taking control of a blueprint automation：
https://www.home-assistant.io/docs/automation/using_blueprints/

[8] Home Assistant — Views，Visible / Subview：
https://www.home-assistant.io/dashboards/views/
