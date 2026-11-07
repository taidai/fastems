# easyEMS v1 最终验收状态

日期：2026-11-05  
构建：单 Go 应用 + 内嵌前端，数据目录 `data/preview`（本机）  
结论：**软件侧验收通过；M5 真实设备现场签收未完成，不能宣称 v1 最终验收通过。**

## 1. 完成定义对照

| 完成定义 | 状态 | 证据 / 说明 |
| --- | --- | --- |
| 8.1 四柜模拟：2 正常、1 离线、1 限功率，800 kW 正确分配、重分、解释 | **通过** | `evidence/m5-allocation.json`、`m5-allocation-restart.json`、`m5-allocation-tests.txt`；真实 Modbus TCP 模拟器，不是实机 |
| 8.2 真实链路：恩玖 PCS、ESBCM BMS、至少一台电表、华为逆变器，采集/控制/回读/断线/恢复/过期阻断 | **未完成（缺硬件）** | 现场执行手册 `docs/m5-site-acceptance.md`；工具 `frontend/m5-site-acceptance.mjs`；尚无真实设备证据 |
| 8.3 权限、审计、备份恢复、断线超时、重复提交 | **通过（软件侧）** | P0/API/浏览器回归；离线完整数据目录恢复；在线备份仍为“未验证”，验收报告如实标 warning |

## 2. 软件侧里程碑

| 里程碑 | 状态 | 证据 |
| --- | --- | --- |
| M1 语义核心 | 通过 | `evidence/m1-semantic-core.txt` |
| M2 运行内核 | 通过 | `evidence/m2-runtime.txt` |
| M3 报警/历史/画面 | 通过 | `evidence/m3-*.txt`、`m3-*.png` |
| M4 部署与验收 | 通过 | `evidence/m4-*.txt/json/png` |
| 规则引擎、分配器运行接线 | 通过 | `m5-allocation*.json`、`allocator-ui.*` |
| 玻璃主题与 10.1 单屏 | 通过 | `glass-console.json`、`theme-*.png`、`10in-*.png` |
| 决策表编辑器交互 | 通过 | `editor-*.png`、完整 browser-smoke |
| 设备与点位工作台 | 通过 | `points-workbench.png`、`points-1024.png`、`points-detail.png`、`points-write.png`、`points-workbench.json`、`points-workbench-batch.md` |
| 紧凑控制台与本地能源管家 | 通过（软件侧） | `layout-check.json`、`layout-overview-1600.png`、`layout-delivery-1600.png`、`assistant-1600.png`、`assistant-1024.png`、`rules-1024.png` |
| 楼顶储能玻璃风 6 步确定值 | 通过（软件侧） | `theme-rooftop-check.json`、`theme-rooftop-1600.png`、`theme-rooftop-1280.png`、`theme-rooftop-1024.png` |
| 图2 像素级总览复刻 | 通过（软件侧） | `reference-replica-check.json`、`reference-replica-1669.png`、`reference-replica-1280.png`、`reference-replica-1024.png` |
| 告警类型管理 | 通过（软件侧） | `alarm-types-1669.png`、`alarm-types-1024.png`、`alarm-type-modal-1024.png` |
| RTU 完整串口参数 | 通过（软件侧） | `serial-params-1669.png`、`serial-params-modal-1024.png`、`serial-params-detail-1024.png`；`go test ./...` |
| 分配器目标入规则编辑器 | 通过（软件侧） | `allocator-rule-1669.png`、`allocator-rule-1024.png` |
| 报警声音/静默/统计 | 通过（软件侧） | `alarm-stats-1669.png`、`alarm-stats-1024.png`、`alarm-sound-1669.png`；统计 API 实测 |
| 报警维护 | 通过（软件侧） | `alarm-maintenance-1669.png`、`alarm-maintenance-modal-1024.png`、`alarm-maintenance-active-1669.png`；引擎单测 |
| Profile 现场验证记录 | 通过（软件侧） | `profile-verification-1669.png`、`profile-verification-1024.png`；CSV 导出实测 |
| 通信质量统计 | 通过（软件侧） | `quality-1669.png`、`quality-1024.png`；质量 API 实测 |
| 历史保留上限 | 通过（软件侧） | `history-limit-1669.png`、`history-limit-1024.png`；启动 retention 5000 实测 |
| 原始报文查看器 | 通过（软件侧） | `frames-1669.png`、`frames-tx-1669.png`、`frames-1024.png`；帧缓存单测 |
| 审计日志 | 通过（软件侧） | `audit-1669.png`、`audit-detail-1669.png`、`audit-1024.png`；CSV 导出实测 |
| 设备台账与导航归位 | 通过（软件侧） | `device-ledger-1669.png`、`device-ledger-detail-1669.png`、`device-ledger-1024.png` |
| 命名命令执行（固定值+参数化） | 通过（软件侧） | `unit-model-1669.png`、`unit-commands-confirmed-1669.png`、`parameter-command-confirmed-1669.png`、`parameter-command-1024.png`；`go test ./...` |
| 开发工具链 | 通过 | Vite proxy、tygo、JSON 模拟器、`scripts/verify.sh` |
| 通道工作区（融合通讯组态） | 通过（软件侧） | `channel-1669.png`、`channel-config-1669.png`、`channel-tools-1669.png`、`channel-config-1024.png`、`channel-tools-1024.png`、`channel-status-1600.png`、`channel-config-1600.png`、`channel-tools-1600.png`、`channel-add-port-1024.png`、`layout-check.json` |
| 设备数据查询 | 通过（软件侧） | `device-query-1669.png`、`device-query-1024.png`、`device-query-realtime-1669.png`、`device-query-history-1669.png`、`layout-check.json` |

## 2.1 设备与点位工作台

- 设备页默认展示点位表：实时值、质量、更新时间、通道/站号、类型/地址/格式、缩放/单位、读写状态。
- 支持名称/通道/地址搜索，按通道、质量、读写筛选；统计正常/过期/离线/可写/已选。
- 批量选择、当前筛选全选、导出 CSV、批量停用/启用/删除；批量修改缩放/偏移（非 0 校验，逐点保存）；按通道批量新增点位（连续地址、名称前缀、预览、重复名称校验、失败回滚）；删除需确认。
- 详情抽屉支持写目标值、等待回读、最近命令状态、查看历史、接管/恢复自动和行内编辑。
- 临时点位实测批量停用/启用/删除、写值回读通过；1280×800、1024×768、1366×768 无页面/舞台/横向滚动。
- 证据：`evidence/points-workbench*.png`、`points-batch-edit.png`、`points-batch-create.png`、`points-workbench.json`、`points-workbench-batch.md`。

## 2.2 紧凑控制台与本地能源管家

- 1920×1080、1600×1000、1366×768、1280×800、1024×768 下，11 个页面页面级、舞台级与横向滚动差值均为 0；1024 决策表仅规则行区域 1px 取整差，模拟与发布栏固定可见。
- 能流第二工具栏实测“俯视/夜间/标签”状态切换；今日事件与调度状态位于能流面板底部；“查看调度”可点击。
- AI 管家只读本机数据，按确定性规则回答功率、报警、点位质量、规则四类问题；宽屏默认展开，Esc 关闭，1024 面板不出视口；不联网、不调用云端模型、不下发控制。
- 交付检查双列、决策表新增先加载再编辑，避免误操作上一张规则；1600 交付页无内部溢出。
- 复现：`frontend/layout-check.mjs`；证据：`evidence/layout-check.json`、`layout-overview-1600.png`、`layout-delivery-1600.png`、`overview-tools-1600.png`、`assistant-1600.png`、`assistant-1024.png`、`rules-1024.png`。
- 边界：规则行很多时编辑器内部滚动（滚动条隐藏）；这是列表容量，不是页面滚动。真实设备 M5 仍未完成。

## 2.3 楼顶储能玻璃风 6 步确定值

- 背景：照片保持清晰，整页无 blur；外壳透明；照片上只有指定 `linear-gradient(180deg, rgba(236,245,255,.55), rgba(226,238,250,.18) 40%, rgba(236,245,255,.45))`。
- 卡片：浏览器计算值为 `linear-gradient(125deg, rgba(246,251,255,.86), rgba(225,237,250,.62))`、`backdrop-filter: blur(24px) saturate(140%)`、`1px solid rgba(255,255,255,.34)`、`inset 0 1px 0 rgba(255,255,255,.32)`。
- 数字与版式：主值 28px/700、字距 -0.5px，单位 18px，说明 12px；圆角 10px、卡片间距 14px；页头 h1 24px；KPI 左上语义色块，光伏/充电绿、储能蓝、告警红。
- 复现：`frontend/theme-check.mjs`；证据：`evidence/theme-rooftop-check.json`、`theme-rooftop-1600.png`、`theme-rooftop-1280.png`、`theme-rooftop-1024.png`。
- 1600×1000、1280×800、1366×768、1024×768 实测 stage/inner/横向滚动均为 0；1024 主值保持 28px，8 列标签不截断。

## 2.4 图2 像素级总览复刻（当前版）

- 背景使用用户指定图1；能流使用图2原始等距场站素材，品牌使用原始 Logo；连接线、五个标签、双工具栏、事件条和底部导航按参考稿排布。
- KPI 改为图2的八项：总收益、储能收益、转换效率、今日电费、储能吞吐、光伏发电量、今日用电、最大需量；未接入指标显示“—”，未复制 Mock 数字。
- 1632×1000 设计坐标等比适配；1669×1026 下外框四边约 4px。默认 3D 能流，趋势入口与等距/俯视/夜间/标签/全屏均可用；AI 默认收起。
- 1669×1026、1600×1000、1280×800、1366×768、1024×768 实测无页面/舞台/横向滚动；全页面回归与完整 browser-smoke 通过。
- 复现：`frontend/theme-check.mjs`、`frontend/layout-check.mjs`；证据：`evidence/reference-replica-check.json`、`reference-replica-1669.png`、`reference-replica-1280.png`、`reference-replica-1024.png`。

## 2.5 首屏运行语义

- 当前调度：从最新已启用决策表的 `lastCheck` 读取规则、状态、时间、命中行与动作请求/实际/确认状态；没有评估时显示“等待首次评估”。
- 今日事件：显示最近评估的规则、时间、命中行；有活动报警时优先显示最新报警标题与时间，并保留活动报警/点位计数入口。
- 节点健康：三维/2D 能流节点右上角绿/琥珀/灰徽标表示已绑定、过期、未接入；悬停显示采样时间。
- 验证：`theme-check.mjs` 断言当前调度、最近评估与五个节点状态；五档分辨率无页面/舞台/横向滚动；全页面布局回归通过。证据：`evidence/reference-replica-check.json`、`reference-dispatch-1669.png`。
- 边界：这些是已接入点位的真实状态；没有功率点位时仍显示“—”，不生成模拟数值。

## 2.6 决策表时序字段

- 每行新增“持续确认 / 冷却”秒数输入，保存为 `hold_ms`/`cooldown_ms`；0 表示不等待 / 不冷却，复制行保留时序。
- 临时规则实测 0.5 秒 / 2 秒保存为 500/2000，删除临时规则后预览数据恢复；无控制台错误，五档分辨率无页面/舞台滚动。
- 证据：`evidence/editor-timing-1669.png`。普通输出的统一所有权仍为边界项。

## 2.7 规则求值解释

- 决策表新增“最近一次求值”折叠面板，直接读取 `lastCheck`：求值时间、状态、命中行、输入值/质量/来源时间、动作请求值→实际值/确认状态/拦截原因。
- 桌面端固定页头、解释、模拟与发布栏，仅规则行区域内部滚动；展开时解释内容限制高度可滚动，不遮挡发布。
- 1669×1026 实测解释、模拟与发布不重叠；五档分辨率无页面/舞台滚动；完整 browser-smoke 通过。
- 证据：`evidence/editor-last-check-1669.png`。

## 2.8 普通输出所有权

- 启用决策表时，服务端比较所有输出绑定的“节点 + 点位类型 + key”；同一可控点位不能同时被两张已启用表拥有，已启用表重新发布版本时再次校验。
- 冲突返回 `409 output_owned`，前端提示先停用或修改另一张规则；不把冲突静默成后写覆盖。
- 单元测试覆盖目标归一化与去重；API 实测 A 启用成功、B 冲突 409、停用 A 后 B 可启用；`go test ./...` 全绿。

## 2.9 最小经营口径：站点电价

- 管理员在显示设置中填写站点电价（元/kWh），保存为根节点 `easyemsTariff` 点；不新增第二套配置存储。
- 今日电费 = 今日用电 kWh 点位 × 电价；缺少用电点位或电价时显示“—”并说明缺哪一项。总收益、储能收益、转换效率仍只取明确接入的点位。
- API 校验 0–100 元/kWh；非法 101 返回 400，合法 0.65 保存并回读，UI 改 0.72 后回读一致。
- 边界：仅单一平价，不实现分时电价、上网电价或日界线推断；这些需要另行确认口径。
- 证据：`evidence/tariff-settings-1669.png`。

## 2.10 V1-4 类型化条件

- 条件支持数值/开关/枚举：开关只允许 0/1 的等于/不等于；枚举保存标签并只允许等于/不等于。
- 运行时会保留字符串点位为枚举标签；模拟新增 `text_inputs`，编辑器提供数值、是/否、枚举文本三类输入。
- 实测“运行状态 = grid”模拟命中；空文本不会覆盖数值输入；单元测试覆盖类型转换、枚举匹配、校验和字符串点位。
- 证据：`evidence/editor-enum-1669.png`、`evidence/v1-closeout.json`。

## 2.11 V1-5 虚拟聚合

- 新增“虚拟聚合”页与 `/api/aggregate`，配置保存在业务节点 `easyemsAggregate` 点，成员限定为该节点子树点位。
- 支持求和/平均/最小/最大/计数；质量与时效逐成员传播，全部无效显示“全部无效/过期”，部分有效标记 `partial` 且只统计有效成员；只读，不写设备。
- 实测五类计算 60/20/10/30/3，部分有效 1/3 返回 partial；非法成员和非法操作符返回 400；五档分辨率无页面滚动。
- 证据：`evidence/aggregate-1669.png`、`evidence/v1-closeout.json`。

## 2.12 V1-10 实施角色

- 新增“实施”角色：可配置通道、设备、点位、Profile、规则、画面、分配器、电价和验收，可受控写入/人工接管；不能管理账号或执行交付检查。
- 实测实施账号创建点位/写值/建规则均 200，建账号与交付检查 403；前端目录隐藏账号与交付页。
- 证据：`evidence/implementer-role-1669.png`、`evidence/v1-closeout.json`。
- 仍缺的 v1 项：普通规则输出的统一 SOC/温度/允许功率/爬坡安全门、Profile 厂商标识常量、爬坡控制。

## 2.13 普通输出统一安全门

- 可写输出新增 `easyemsSafety`：范围、放电/充电上限、允许条件（数值/开关/枚举）；入口在规则输出行“安全”按钮与点位抽屉，未配置显示橙点。
- 运行时普通规则动作统一经过夹限与条件校验；条件点位缺失/过期/不满足阻止下发，原因写入 `lastCheck`。
- 实测请求 150、功率上限 60 → 应用 60；SOC 5 不满足 ≥10 → blocked。范围/上限/条件单元测试通过。
- 未配置不阻断存量规则，但入口可见提示；爬坡需连续设定值控制器，尚未实现，不提供不生效字段。
- 证据：`evidence/safety-modal-1669.png`、`evidence/safety-rule-entry-1669.png`。
- 另修：命令回读样本改用 Cycle 时间，不再用墙钟时间；`go test ./...` 全绿。

## 3. 厂商 Profile 库（本轮新增）

内置 7 个条目：

| Profile | 设备 | 精简点位 | 命令 | 验证状态 |
| --- | --- | --- | --- | --- |
| `modbus-generic` | 通用 Modbus | 0 | 0 | 人工占位 |
| `enjo-pcs` | 恩玖 PCS V1.20/V1.27 | 14 | 2 | 未现场验证 |
| `esbcm-bms` | 高特 ESBCM BMS V1.18 | 17 | 0 | 未现场验证 |
| `dtsd3366d-meter` | DTSD3366D 电表 | 10 | 0 | 未现场验证 |
| `yd2037-meter` | 雅达 YD2037Y-F 电表 | 10 | 0 | 未现场验证 |
| `huawei-inverter` | 华为逆变器 50KTL / V3.0 | 8 | 1 | 未现场验证 |
| `jiwa-mppt` | 吉瓦时代 MPPT V2.5 | 8 | 0 | 未现场验证 |

- 每个 Profile 携带来源、识别提示、注意事项和精简点位；未经真机签收不改为已验证。
- 部署页可选 Profile、查看“未现场验证”、下载点表 CSV；CSV 与现有导入格式一致。
- 实测：`enjo-pcs` CSV 14 行，预览导入 14 个点位通过；详见 `evidence/profile-library.png`。
- 自动匹配仍受厂商标识常量缺失限制；未匹配时人工选择，不阻塞接入。

## 4. M5 现场验收工具

- 手册：`docs/m5-site-acceptance.md`（前置条件、Profile 导入、8 项人工检查、通过标准）。
- 工具：`frontend/m5-site-acceptance.mjs`，默认只读；只有 `M5_ALLOW_WRITE=1` 才执行配置中的控制/回读。
- 自检（模拟器、非现场）：`evidence/m5-site-tool-selftest.json`，`failed=0`；`acceptance`、Profile 未验证与同名点位为 warning。
- 自检不能替代现场签收；断线/恢复、过期阻断、重复提交、真实 PCS 分配仍需现场人员完成。

## 5. 最终验收还缺什么

1. 连接恩玖 PCS、ESBCM BMS、电表、华为逆变器真机，核对站号、波特率/端口、地址基准与字序。
2. 用 Profile CSV 预览、导入，逐点核对量级和只读标记；修正厂商地址歧义（例如恩玖 0x500E/0x0D57）。
3. 现场执行 8 项人工检查并保存截图、报告、设备序列号、操作人与时间。
4. 在 4 台真实 PCS（至少 1 台离线、1 台限功率）完成 800 kW 分配、逐台回读、超时未知、恢复与重启不重放。
5. 完成上述证据后，把相关 Profile 的 `verified` 改为 true，并更新本报告与 `GOAL.md`。

在这些完成前，v1 的状态是：**软件、模拟器与交付工具验收通过；真实链路最终签收待现场。**

## 2.14 爬坡控制

- 安全约束新增 `ramp_per_second`；规则触发后按 1 秒周期继续写中间设定值，直到目标，不要求规则重复触发。
- 每步重新校验范围、功率上限和允许条件；条件失效立即停止并写 `lastCheck`，手动接管也停止；达到目标后清除爬坡标记。
- 实测 10/s 从 0 到 100：0→10→30→…→100；SOC 中途降到 5 时停在 14.95，`lastCheck` 记录 `ramp stopped`。
- 证据：`evidence/v1-closeout.json`。

## 2.15 控制策略可编辑表格

- 决策表支持“表格视图 / 详细视图”切换并记住选择；表格一行一条策略，条件/动作以标签摘要展示。
- 内联编辑持续、冷却、上移、下移、复制、删除；点击整行进入详细编辑并定位该行。
- 10.1 下单独放大表格字号和控件；1024×768 页面/舞台/横向/表格内部滚动均为 0。
- 证据：`evidence/rules-table-1669.png`、`evidence/rules-table-1024.png`。

## 2.16 Profile 自动识别

- 每个厂商 Profile 增加特征探测：型号 ASCII、协议版本、机器特征寄存器；识别接口返回候选、分数、证据和强/弱标记。
- 读到型号文本时才自动匹配（强匹配）；只读到特征寄存器时只列候选，必须人工确认，避免误判。
- 独立 TCP 模拟器写入 0xAB80=`YD2037Y-F`，实测自动匹配 `yd2037-meter`；无响应地址返回空候选。部署页显示证据并支持点击候选人工选定。
- 证据：`evidence/profile-identify-1669.png`、`evidence/v1-closeout.json`。
- 边界：除 YD2037 仿真文本外，其余厂商固定识别常量、现场地址、缩放、字序和写授权仍需实机核对。

## 2.17 告警类型管理

- 配置保存在业务节点 `easyemsAlarmType` 点，接口 `GET/PUT/DELETE /api/alarm-types`；字段覆盖编码、名称、分组、级别、触发点位、比较/离线、阈值、延时、确认方式、记录复归和启用。
- Watcher 每轮扫描读取配置：阈值要求点位有值且 15 秒内更新，离线只接受 Modbus 通道；条件持续达到延时才产生告警，自动确认时同时确认，复归关闭活动报警并按“记录复归”保留或关闭记录。
- 删除或停用类型会关闭该类型仍在活动的告警；固定告警（通道离线、命令失败、规则异常）不受影响。
- 实测温度 >40 在 45 报警，回落到 35 复归；界面支持新增、编辑、删除、启用/停用，1024/1669 无页面滚动。
- 证据：`evidence/alarm-types-1669.png`、`alarm-types-1024.png`、`alarm-type-modal-1024.png`。

## 2.18 通道工作区（融合通讯组态）

- 原“通道与设备”健康页和“通讯组态”配置页合并为一个“通道”页，页签“运行状态 / 端口配置 / 点表与模板”；页面目录中不再有独立通讯组态入口。
- 运行状态保留在线/不稳定/离线汇总、端口卡片、轮询周期和最近成功轮询；端口配置保留端口增删改、TCP/RTU 参数、单元与点位新增。
- 端口弹窗内新增保存前的真实只读连接测试；点表与模板页签保留 CSV 预检/导入/导出、CSV 模板下载、型号模板保存/应用/删除。原设备页的重复 Modbus 表单已移除，工作台只保留跳转入口。
- 状态页的“通信配置”和每张端口卡的“配置端口”直接切换到端口配置；新增点位默认沿用已有单元站号，修改站号即新增单元。
- 底层仍是同一批 `modbus` 节点与现有 `/api/modbus`、`/api/ems/channels/{id}` 接口，没有新增模型或服务。
- 1669/1600/1280/1024 页面、舞台、工作区和配置页/点表页横向滚动均为 0；完整 14 页 × 5 分辨率布局回归通过。
- 完整浏览器冒烟在干净数据目录通过，覆盖端口测试、点位新增、写值回读、CSV 导入导出与模板应用。
- 证据：`evidence/channel-1669.png`、`channel-config-1669.png`、`channel-tools-1669.png`、`channel-config-1024.png`、`channel-tools-1024.png`、`channel-status-1600.png`、`channel-config-1600.png`、`channel-tools-1600.png`、`channel-add-port-1024.png`、`evidence/layout-check.json`。

## 2.19 设备数据查询

- 左侧站点/设备树按业务节点子树选择范围；右侧五页签：概览、实时数据、告警事件、历史趋势、数据报表。
- 概览显示点位数、正常/过期/无值、活动告警和实时值列表；实时表显示值、单位、质量、更新时间与来源；告警按所选子树过滤；历史复用现有 `HistoryView`，点击实时行直接打开该点趋势；报表提供每点最近 24 小时 CSV 导出。
- 只读现有节点与接口，没有新增后端。1669/1024 页面、舞台与横向滚动均为 0；报表页内部滚动 23/51px 属于列表容量。全页面布局回归（14 页 × 5 分辨率）通过。
- 边界：历史单次最多 2000 点；质量按 15 秒有效期判定“过期”，与现有实时值口径一致。
- 证据：`evidence/device-query-1669.png`、`device-query-1024.png`、`device-query-realtime-1669.png`、`device-query-history-1669.png`、`evidence/layout-check.json`。

## 2.20 RTU 完整串口参数

- Modbus RTU 端口新增数据位（5–8）、校验位（N/E/O）和停止位（1/2），与波特率一起保存为 `modbus` 节点的 `dataBits`、`parity`、`stopBits` 点。
- 运行时地址编码为 `port@baud@data@parity@stop`，`modbusrt` 以完整 `serial.Mode` 打开串口；旧的 `port@baud` 地址自动按 8N1 解析。
- 端口弹窗、串口详情、只读连接测试和 `/api/ems/channels` 共用同一组参数；非法数据位、校验位或停止位返回 400。
- 实测界面创建 `COM3 / 19200 / 7 / E / 2` 端口，API 快照回读 `dataBits=7`、`parity=E`、`stopBits=2`，串口详情显示 `7 / E / 2`；测试端口已删除。
- 单元测试覆盖旧/新地址解析、默认值、非法参数、桥接地址生成和保存点；`go test ./...` 全绿。
- 边界：本机无真实 RS485 设备，串口打开与现场帧交互仍需 M5 实机确认；默认值仍为 8N1。
- 证据：`evidence/serial-params-1669.png`、`serial-params-modal-1024.png`、`serial-params-detail-1024.png`。

## 2.21 命名命令执行（固定值+参数化）

- 通道单元的“设备模型”把 Profile 绑定到站号，按变量名自动匹配同站号点位；绑定保存在通道节点 `easyemsUnitModel`，重启由节点恢复。
- 命令经 `command.Manager` 执行：幂等 request_id、写队列、读回确认、超时 `unknown`、失败原因；设备离线时拒绝下发。
- 界面显示 Profile、映射状态（已映射/需映射/可选）、命令可用性、执行状态与 request_id；有权限的用户才能修改模型或执行命令。
- Profile 命令可声明参数（名称、标签、点位、单位、范围、默认值）；命令管理器按参数生成写入与动态回读，越界/未知参数拒绝。
- 实测恩玖开机固定命令与华为有功功率百分比参数命令（42%）均写入模拟器并回读确认；测试端口已删除。单元测试覆盖固定命令、参数解析、范围/未知参数、只读点位和未映射原因。
- 边界：参数当前为数值/布尔（内置 Profile 使用数值）；真实 PCS/BMS 仍需 M5 现场验证。
- 证据：`evidence/unit-model-1669.png`、`unit-commands-confirmed-1669.png`、`parameter-command-confirmed-1669.png`、`parameter-command-1024.png`。

## 2.22 开发工具链

- Vite dev server 代理 `/api` 到 Go 后端，`scripts/dev.ps1`/`dev.sh` 启动开发回路；安装 air 时 Go 自动热重载。该验证针对旧单 exe 调试入口；2026-11-06 起生产改为 Encore 容器，见 `docs/encore-migration.md`，旧 exe 不再作为生产交付。
- `tygo` 从 `internal/contract` 生成 `frontend/src/generated/contract.ts`；`scripts/gen-types.sh` 可重复生成。
- `scripts/modbus-simulator.go` 支持 JSON fixture：多端口、寄存器/线圈、延迟、响应破坏和断线注入；basic/commands/faults fixture 已提供。
- `scripts/verify.sh` 统一执行前端构建、Go test/vet/build、browser-smoke、layout-check、theme-check；本机已通过。

## 2.23 审计日志

- 本地追加式文件 `audit.jsonl` 位于数据目录，随数据目录离线备份；超过 5MB 轮换一份 `.1` 备份。
- 记录登录成功/失败、端口/点位配置、命令执行、规则、报警、账号等写操作；字段含操作人、动作、对象、结果、IP、时间，不记录密码、令牌或请求体。
- 管理员在“配置 → 审计日志”按操作人、动作、结果、日期筛选，查看详情并导出 CSV；普通角色不可见，接口返回 403。
- 实测登录、创建/删除端口、命令执行均生成记录；CSV 含表头与记录；1669/1024 页面、舞台、横向滚动为 0。
- 边界：当前审计是本地文件、非防篡改链；操作人使用登录邮箱（历史记录可能显示用户 ID）。更细的对象字段与审计保留策略仍可继续完善。
- 证据：`evidence/audit-1669.png`、`audit-detail-1669.png`、`audit-1024.png`。

## 2.24 原始报文查看器

- Modbus TCP/RTU 传输层记录实际收发的原始帧，内存环形缓存最多 500 帧；记录时间、方向、HEX、对端。
- 通道运行状态卡片新增“报文”抽屉：全部/收/发过滤、暂停、清空、复制；按通道地址匹配对端。
- 缓存不持久化、不改变采集/写值路径；清空只影响当前进程缓存。
- 实测验收总线采集 500 帧，界面按 `127.0.0.1:1502` 展示；1669/1024 抽屉不出视口。帧缓存单测通过。
- 边界：缓存为进程内存，重启归零；多个通道共用同一地址时对端无法区分。
- 证据：`evidence/frames-1669.png`、`frames-tx-1669.png`、`frames-1024.png`。

## 2.25 分配器目标入规则编辑器

- 规则输出下拉同时列出可写点位和带 `easyemsAllocator` 配置的业务分组；分组显示“（分配器）”，选中后动作绑定分配器节点而非单个点位。
- 运行时沿用现有分配器校验：目标必须是已配置分配器、成员在子树内、输出所有权唯一；`lastCheck` 继续记录分配明细。
- 实测临时分组配置单成员分配器后，规则编辑器输出下拉出现“分配器验收组（分配器）”，可选中并进入模拟/发布流程；临时分组与变量已删除。
- 边界：分配器本身的成员配置仍在“功率分配器”页；规则编辑器只负责选择目标，不在规则里复制成员配置。
- 证据：`evidence/allocator-rule-1669.png`、`allocator-rule-1024.png`。

## 2.26 通信质量统计

- 运行时按设备记录请求数、失败数、连续失败、最近成功/错误；每次块读取记录延迟次数、累计、最大、最近值。
- `GET /api/ems/quality` 返回通道聚合和设备明细：成功率、平均/最大/最近延迟、健康、最近错误。
- 通道运行状态卡片显示成功率、平均延迟、错误数；无采样时显示“—”，不生成模拟值。
- 实测验收总线/模板总线 100% 成功率、1 ms 平均、0 错误；离线 #1 显示“—”。
- 边界：统计为进程内存，重启归零；延迟只覆盖读块，不包含写命令；设备 ID 为通道/站号。
- 证据：`evidence/quality-1669.png`、`quality-1024.png`。

## 2.27 历史保留上限

- 历史页管理员可设置每点位保留条数（1000–200000）。设置写入数据目录 `history-limit`，运行时内存历史立即裁剪；持久历史在下次启动写入 JetStream `StoreMaxMsgsPerSubject`，界面明确“持久历史重启后生效”。
- 实测 UI 设置 26000 并回读一致；独立测试实例以 5000 启动，Store 日志显示 `retention: 5000 points per subject`。
- 边界：持久历史策略在启动时应用；不做在线 stream 重建。默认 20000；恢复到默认值已实测。
- 证据：`evidence/history-limit-1669.png`、`history-limit-1024.png`。

## 2.28 报警维护

- 报警引擎新增维护窗口：对已有报警键设置结束时间、原因、维护人；窗口内同一键不会重新激活或产生新的打扰，已有报警记录保留；维护窗口持久化到数据目录 `alarm-maintenance.json`，重启恢复。
- 维护结束后条件仍存在会重新激活；也可手动“结束维护”。维护只针对已有键，不做类型级周期排班。
- API：`POST/DELETE /api/ems/alarms/{id}/maintenance`；界面在报警卡片显示“维护中 · 至… · 原因”，提供维护/结束维护。
- 单测覆盖维护期抑制、恢复后重新激活、手动结束与导出/导入；临时报警类型已删除。
- 证据：`alarm-maintenance-1669.png`、`alarm-maintenance-modal-1024.png`、`alarm-maintenance-active-1669.png`。

## 2.29 Profile 现场验证记录

- 设备模型新增验证人、验证日期、设备序列号、验证说明；勾选“已完成现场验证”时服务端要求验证人/日期/序列号非空。
- 新增 `GET /api/profile-verifications?format=csv`，导出通道、站号、设备名称、Profile、厂商/型号、验证状态、验证人、日期、序列号、备注和点位数。
- 设备模型弹窗显示并保存这些字段，提供“导出验证记录”；内置 Profile 的 `verified` 仍保持未验证，现场验证按单元绑定保存。
- 实测验证记录 PCS 的字段保存/回读和 CSV 导出；临时通道已删除。
- 证据：`profile-verification-1669.png`、`profile-verification-1024.png`。

## 2.30 报警声音、静默与统计

- 报警页新增“统计与提醒”页签：今日/昨日告警数、未处理/已处理、7/30 天趋势、对象排行。
- 新出现的活动且非维护报警触发本地 Web Audio 双音提示；支持音量、试听和跨午夜静默时段，设置保存在本机。声音只在页面打开且有用户手势后可靠播放。
- 报警引擎在 raised/acknowledged/resolved/closed/maintenance/released 时写入 `alarm-events.jsonl`；`GET /api/alarm-stats?days=7|30` 从事件日志计算统计，重启不丢；统计从该功能启用后开始累计。
- 实测今日 4 条、未处理 2、已处理 2，趋势与排行正确；1669/1024 页面、舞台、横向滚动为 0。
- 边界：统计是本地事件日志聚合，不是独立 BI/长期归档；静默只影响声音，不影响记录与统计；声音依赖浏览器音频策略。
- 证据：`alarm-stats-1669.png`、`alarm-stats-1024.png`、`alarm-sound-1669.png`。

## 2.31 设备台账与导航归位

- 新增“设备台账”页与 `GET /api/device-ledger`：按通道/站号列出一台设备一行，包含业务节点、Profile、点位数/命令数、健康、成功率/延迟和验证状态。
- 设备详情提供端口与设备模型、点位表、数据追溯、报警的统一入口；点开不再需要在通道/点位/历史之间找同一台设备。
- 导航归位：运行画面→总览，功率分配器→调度，报警→数据，设备台账→设备；底栏“报表”改为“数据”。
- 实测 1669/1024 页面、舞台、横向滚动为 0；设备台账当前列出 2 台在线单元，验证状态和健康均正确。
- 证据：`device-ledger-1669.png`、`device-ledger-detail-1669.png`、`device-ledger-1024.png`。
