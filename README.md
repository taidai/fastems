# easyems

面向光储充 EMS 的单机产品底座：用业务树组织现场，用有序决策表表达控制与联动，在一台工控机上完成采集、控制、报警、历史与交付。生产运行时由 Encore 服务承载，内嵌 Simple IoT、NATS/JetStream、EMS 运行时和前端；不依赖外部数据库或第二业务服务。

> 2026-11-06 起生产交付形态改为 Linux 容器（Encore 构建），Windows 单 exe 仅保留为本地调试/回滚入口。`/api/health`、`/api/login`、`/api/device-ledger`、`/api/alarm-stats` 已是 Encore 命名原生路由，其余 UI/API 由 fallback 在进程内承载；没有第二个 HTTP 监听或反向代理。迁移边界、取舍与分阶段计划见 [`docs/encore-migration.md`](docs/encore-migration.md)。


## 目标与规格

- 产品目标、v1 范围与验收标准：[`docs/product-spec-v1.md`](docs/product-spec-v1.md)（2026-11-04 冻结）。
- M4 部署与验收步骤：[`docs/m4-acceptance.md`](docs/m4-acceptance.md)。
- 一页看懂的总图：[`docs/master-map.html`](docs/master-map.html)。
- 厂商 Profile 库：[`docs/vendor-profiles.md`](docs/vendor-profiles.md)；M5 现场执行：[`docs/m5-site-acceptance.md`](docs/m5-site-acceptance.md)。
- 最终验收状态：[`evidence/final-acceptance.md`](evidence/final-acceptance.md)。
- 新需求进入 v1.1 候选；冻结期内只修缺陷，不扩大范围。

## 当前代码状态

下列能力来自上一版“通用 IoT 底座”，作为新产品的骨架继续演进，**不等于 v1 已交付**：

- 任意层级分组、测试变量、本人账号登录和管理员/操作员/查看者权限。
- Modbus TCP/RTU 连接、真实只读测试、寄存器类型/缩放/偏移、单点添加与中文 CSV 整批导入。
- 决策表：多条件 AND、多输出顺序执行、首条命中、上下排序、默认不动作。
- 保存草稿、发布、启用和纯后端模拟相互独立；模拟不写设备点位。
- 人工接管只暂停目标点位的自动输出，恢复自动立即重新评估；换来源只改绑定，不改业务 ID 与规则。
- Modbus 最近一次完整成功轮询形成心跳；禁用、失效或过期的输入不会触发新动作。
- 产品模式关闭通用 `/v1`、旧工程界面和浏览器 NATS 通道；内嵌 NATS 只监听回环地址。
- 首次启动在产品 API 就绪前替换上游默认 `admin/admin`，HTTP 入口通过自身健康检查后才报告 ready。

## 实施状态

- 上一版 P0 已逐项通过 API、真实协议和浏览器验收，详见 `docs/p0-acceptance.md`；它证明骨架可用，不代表 v1 范围已完成。
- v1 M1（语义核心）、M2（运行内核）、M3（报警、历史趋势、运行画面）与 M4（持久化、扫描识别、点表、验收）已完成；新规则引擎、分配器运行接线与分配器配置 UI 已完成，4 台 Modbus 模拟 PCS 验收通过；玻璃主题、10.1 单屏、图2像素级总览（图1背景、原始等距场站、八项指标、双工具栏、事件条与底部导航）与本地能源管家完成；交付检查双列、决策表模拟/发布栏固定；设备页新增点位工作台（批量停用/启用/删除/导出、批量改缩放/偏移、按通道批量新增、实时值与质量、写值回读、行内编辑）。报警页新增可配置告警类型（阈值/离线、延时、自动确认、记录复归）；通道页融合运行状态、端口配置与点表模板，统一组织 Modbus 端口、单元与点位；RTU 端口可配置数据位、校验位和停止位；通道单元可绑定 Profile、映射点位并执行固定值/参数化命名命令（回读/超时/结果未知）；管理员可查看登录、配置与控制审计日志并导出 CSV；新增设备台账统一设备、通道/站号、Profile、健康与验证；通道可查看实时原始 Modbus 收发报文、成功率/延迟/错误统计；历史页可设置每个点位的保留条数；报警可设置维护期抑制、声音/静默时段并查看今日/昨日与趋势统计；设备模型可记录现场验证人和序列号并导出验证记录；新增设备数据查询页，按站点查看概览、实时值、告警、历史趋势与 24 小时 CSV 报表。本地能源管家只读本机点位、规则与报警，按确定性规则回答，不联网、不调用云端模型、不下发控制。经营口径采用最小实现：管理员配置单一站点电价，今日电费仅由“今日用电 kWh × 电价”估算；缺少点位或电价时显示“—”，不推测收益或效率。v1 补齐类型化条件（数值/开关/枚举）、只读虚拟聚合（求和/平均/最值/计数、质量与部分有效传播）、“实施”角色和普通输出统一安全门（范围、充放电上限、允许条件、`lastCheck` 原因）。厂商 Profile 库已入库 6 个未验证 Profile（含精简点位与 CSV 下载），M5 现场执行手册与只读工具已就绪。**真实设备现场验收仍未完成**，不能宣称 v1 最终签收。证据与逐项状态见 [`GOAL.md`](GOAL.md) 和 [`evidence/final-acceptance.md`](evidence/final-acceptance.md)。

## 构建与运行

生产链路需要 Node 24.12.0、Encore CLI 与 Docker；旧单 exe 链路需要 Go 1.27.3。

### Encore（生产）

```bash
export EASYEMS_DATA="$PWD/data/encore"
encore run --port 4000
```

访问 `http://localhost:4000`；Encore 开发面板在 `http://localhost:9400`。

构建容器镜像：

```bash
./scripts/encore-build.sh
```

镜像运行：

```bash
docker run -d --name easyems -p 8080:8080 -v easyems-data:/data -e EASYEMS_DATA=/data easyems:latest
```

### 旧单 exe（仅本地调试/回滚）

```bash
cd frontend && npm ci && npm run build
```

```bash
go test ./...
go build -o easyems.exe ./cmd/easyems
```

### 本地开发热更新

前端改动不需要重建嵌入产物：

```bash
./scripts/dev.ps1
```

```bash
./scripts/dev.sh
```

脚本启动旧 Go 后端（安装 `air` 时自动热重载）和 Vite（`http://127.0.0.1:5173`，`/api` 代理到后端）。没有 `air` 时使用现有 `easyems.exe`，只热更新前端。Encore 开发时把 `EASYEMS_API` 指到 `http://127.0.0.1:4000`。生产由 `npm run build` + `encore build docker` 提供。

首次运行前设置仅用于本机的管理员密码（至少 12 位）：

```bash
EASYEMS_ADMIN_PASSWORD='replace-with-a-local-password' encore run --port 4000
```

Windows PowerShell 使用 `$env:EASYEMS_ADMIN_PASSWORD='replace-with-a-local-password'`。如果未设置，应用生成随机密码并写入数据目录的 `initial-password.txt`（权限 0600），不会打印到日志。已有数据目录不会重置密码。

访问 `http://localhost:4000`，账号为 `admin`。当前 HTTP 监听适用于本机或受信网络验收，不宣称可直接暴露公网。

## 完整备份与恢复

已验证的支持方式是**离线复制整个数据目录**：停止 easyems，复制 `data`（包括 JetStream、设备密钥和初始化标记），恢复时停止目标实例并用备份替换其数据目录，再启动同一个应用。不要只复制 YAML 或部分文件。

备份包含账号哈希和设备密钥，应按敏感数据保管；不再需要本地初始密码文件时应先删除它。当前不支持也不宣称在线热备份。`frontend/restore-check.mjs` 是本机恢复验收脚本。

## 验证边界

Encore 生产验证使用 `./scripts/verify-encore.sh`（构建容器、健康检查、原生路由、浏览器回归）；旧单 exe 的完整回归仍由 `scripts/verify.sh` 覆盖。软件侧证据见 [`evidence/encore-migration.md`](evidence/encore-migration.md)。

真实 Simple IoT/JetStream 集成测试覆盖策略执行与重启恢复；产品 API 集成测试覆盖登录、权限、CSV 与通用接口关闭；TCP/RTU 协议级读写、断线输入阻断、浏览器操作和离线完整恢复均已实测，见 `evidence/verification.md`。

真实物理设备、真实品牌 A/B 硬件更换、独立目标用户和公网部署仍需现场验收，不能用模拟或单元测试替代。

`third_party/simpleiot` 是固定上游源码及 easyems 集成补丁，详见 `third_party/README.md`。
