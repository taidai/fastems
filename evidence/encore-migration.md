# Encore 迁移验证

日期：2026-11-06
范围：软件侧运行时迁移；不替代 M5 真实设备现场签收。

## 结论

生产入口已由 Encore 接管，内置 HTTP 监听已关闭。`/api/health`、`/api/login`、`/api/device-ledger`、`/api/alarm-stats` 是 Encore 命名原生路由；其余 UI/API 由 fallback 在进程内交给现有处理器，没有反向代理或第二个 HTTP 服务。Encore 本地运行与自托管 Docker 镜像均能完成健康检查、登录、前端加载、设备台账和浏览器主流程。Windows 单 exe 仍可运行，但只作为本地调试/回滚入口。

## 已验证

### 1. 旧入口回归

- `go test ./...`：全部通过（迁移、历史修复与原生只读路由后重跑）。
- `scripts/verify.sh`：通过；包含前端构建、Go test/vet/build、browser smoke、15 页布局检查、主题检查。
- `internal/decision` 集成测试端口从 9700/9702/9703 改到 19700/19702/19703，避免与 Encore 本地守护进程端口冲突。

### 2. Encore 本地运行

命令：

```bash
export EASYEMS_DATA="$PWD/data/encore-final"
export EASYEMS_ADMIN_PASSWORD='<local-test-password>'
export GOFLAGS=-buildvcs=false
encore run --port 4408 --browser never
```

结果：

- `GET /api/health` → 200 `{"ready":true,"version":"0.1.0"}`。
- `GET /` → 200，返回内嵌前端 HTML。
- `POST /api/login` → 200，返回 token。
- `GET /api/device-ledger`（带 Bearer）→ 200。
- `frontend/browser-smoke.mjs`：通过，覆盖登录、点位、决策表、dry-run、发布、输出和移动端。
- `frontend/layout-check.mjs`：通过；15 页 × 5 个分辨率，页面/舞台/横向溢出为 0。
- `frontend/theme-check.mjs`：通过；5 个分辨率的参考复刻检查均为 0 溢出。
- 12 个并发 `/api/history` 请求：全部 200（修复历史查询 consumer 后）；集成测试也加入同样的并发回归。
- 启动日志不再出现 `Starting http server` / `Starting portal`；Encore 是唯一 HTTP 监听。
- 原生日志确认 `endpoint=Health`、`endpoint=Login`、`endpoint=DeviceLedger`、`endpoint=AlarmStats` 独立出现，不再统一落到 `Fallback`。
- 原生路由保持原语义：`/api/health` 无鉴权 200；`/api/login` 正确登录 200、错误密码 403；`/api/device-ledger` 无 token 401、有 token 200；`/api/alarm-stats?days=7` 有 token 200。
- fallback 仍服务 `/`、静态资源和未命名 API：`/api/snapshot`、`/api/ems/channels` 带 token 均 200。
- 四个命名原生路由上线后重跑 `browser-smoke`、15 页 × 5 分辨率布局检查和主题检查，全部通过。

### 3. Encore Docker 镜像

命令：

```bash
encore build docker --config deploy/encore-infra.json easyems:encore-final
```

结果：

- 镜像构建成功，111 MB（`easyems:encore-final`）。
- 容器 `-p 4409:8080` 启动，`GET /api/health` → 200，`GET /` → 200；日志中没有 `Starting http server`。
- 使用独立数据卷和初始化密码登录 → 200；`/api/device-ledger`、`/api/alarm-stats?days=7`、`/api/snapshot`、`/api/ems/channels` 均 200；错误密码 403。
- 容器重启后健康检查仍为 200，数据卷保留。

## 修复

- 历史查询并发时，原来的普通临时 consumer 在多个点位同时加载时创建冲突或超时并返回 503。现在改为 ordered ephemeral consumer，12 路并发全部 200；记录在 `third_party/README.md`。

## 未完成

- 四个命名路由（health/login/device-ledger/alarm-stats）是 Encore 原生 raw 端点；其余 API 和 UI 仍走进程内 fallback，尚未按端点拆分 tracing。Encore 不允许 `/api/*` 通配符与静态路由并存，因此没有强行枚举全部路由。
- Encore 类型化 schema 与生成客户端未在本轮做：现有接口依赖 `{"error": ...}` 错误体和精确状态码，强行改成 Encore 类型化错误会改变契约；raw 端点先保证行为等价。
- 未在目标工控机上实测自托管镜像、权限、端口和开机自启动。
- 未验证容器访问现场 RS485/TCP 设备；容器内 `127.0.0.1` 不是工控机，需要 LAN 地址、host 网络或显式 host 映射。
- 真实设备 M5 签收仍未完成，模拟器和容器通过不能替代现场验收。

## 复现命令

```bash
./scripts/verify-encore.sh
```

```bash
./scripts/encore-dev.sh
```

```bash
./scripts/encore-build.sh
```
