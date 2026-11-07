# Encore 迁移

日期：2026-11-06
决策：生产运行时迁移到 Encore；接受 Linux/Docker 部署，不再把 Windows 单 exe 作为生产交付物。

## 1. 为什么这样做

Encore 不是一个可以 import 的库。它要求 `encore.app`、服务注解和由 CLI 生成的入口，生产构建只有 `encore build docker`。因此迁移的边界不是“加一个依赖”，而是：

- Encore 拥有进程、HTTP 监听、构建和部署；
- 现有 Simple IoT/NATS/JetStream 与 EMS 运行时作为 Encore 服务内的长生命周期组件；
- 现有 HTTP 路由通过 Encore 的 fallback 原始端点继续提供服务；
- 后续把健康检查、登录、设备台账、报警等端点逐个迁成 Encore 原生 API；
- 公共 URL 不变，前端不需要跟着改。

## 2. 当前结构

```text
encore.app
go.mod
service/ems/ems.go          Encore 服务：命名原生路由、fallback、关闭
internal/app/app.go         进程生命周期与进程内 UI/API 处理器
cmd/easyems/main.go         旧入口，仅保留本地调试/回滚
deploy/encore-infra.json    自托管最小基础设施配置
scripts/encore-dev.sh       本地 Encore 启动
scripts/encore-build.sh     前端构建 + Encore 镜像构建
scripts/verify-encore.sh    镜像 + 容器 + 浏览器回归
internal/web/dist/          前端构建产物，go:embed 嵌入
```

启动顺序：

1. Encore 调用 `initService`。
2. `internal/app` 选择回环空闲 NATS 端口，启动 Simple IoT、JetStream、EMS runtime、报警 watcher、bridge；内置 HTTP 监听关闭。
3. 等待存储与业务初始化完成。
4. Encore 在 4000 端口提供 HTTP：`/api/health`、`/api/login`、`/api/device-ledger`、`/api/alarm-stats` 走命名原生路由；其余路径由 fallback 在进程内调用现有 UI/API 处理器，无反向代理、无第二个 HTTP 监听。
5. Encore 关闭时调用 `Shutdown`，停止内部运行时。

## 3. 本地运行

在 WSL/Linux 中安装 Encore CLI 后：

```bash
curl -fsSL https://encore.dev/install.sh | bash
```

```bash
export EASYEMS_DATA="$PWD/data/encore"
export GOFLAGS=-buildvcs=false
encore run --port 4000
```

或直接使用：

```bash
./scripts/encore-dev.sh
```

`GOFLAGS=-buildvcs=false` 用于没有首个 commit 的仓库；生产构建脚本已默认设置。若 `encore build docker` 仍报 VCS status，先用 Encore 自带的 Go 持久化该设置，或先提交首个 commit：

```bash
go env -w GOFLAGS=-buildvcs=false
```

访问 `http://localhost:4000`。Encore 开发面板在 `http://localhost:9400`。

Windows 主机上的旧入口仍可用于快速调试，但它不再是生产路径：

```bash
go run ./cmd/easyems -port 8080 -data data/dev
```

## 4. 构建与部署

```bash
npm --prefix frontend run build
encore build docker easyems:latest
```

或：

```bash
./scripts/encore-build.sh
```

容器必须挂载数据目录，并设置数据目录环境变量：

```bash
docker run -d \
  --name easyems \
  -p 8080:8080 \
  -v easyems-data:/data \
  -e EASYEMS_DATA=/data \
  easyems:latest
```

Encore 自托管镜像默认监听 8080；需要换端口时同时设置 `-e PORT=4000 -p 4000:4000`。`deploy/encore-infra.json` 是自托管所需的最小基础设施配置（没有 SQL/Redis/Pub/Sub）；如果部署在反向代理后，把 `metadata.base_url` 改成实际外部地址再构建。

容器内的 `127.0.0.1` 指向容器本身，不是工控机。连接工控机上的本地设备或模拟器时，使用 `--network host`、设备所在主机的 LAN 地址，或显式配置 `host.docker.internal`；不能把现场设备地址写成容器回环。

## 5. 已接受的取舍

- 不再产出 Windows 单 exe 作为生产交付物；旧的 `cmd/easyems` 仅用于本地调试和回滚。
- 生产镜像基于 Linux；工控机需要能运行 Docker，或由部署方提供等价的 OCI 运行时。
- Encore 的本地开发面板、tracing 和构建缓存会成为开发链路的一部分。
- 内嵌 NATS/JetStream 仍在同一进程内，不引入外部消息队列或第二业务服务。
- 当前仍然没有外部数据库；Encore 只接管 HTTP/运行时，不强制使用 SQL。

## 6. 分阶段迁移

**Phase 0（本次）**
- 加入 `encore.app`、`service/ems`、`internal/app`。
- fallback 接管全部现有路由。
- 保留 `cmd/easyems` 作为回滚入口。
- 验证 `/api/health`、登录、页面、写值链路。

**Phase 1（本次完成）**
- `/api/health`、`/api/device-ledger`、`/api/alarm-stats` 已迁成 Encore 原生 raw 路由；请求通过进程内处理器复用现有逻辑，鉴权、状态码和响应体不变。
- 类型化请求/响应和生成客户端仍未做；当前先保证行为等价，避免在迁移中复制权限逻辑。
- 保留 fallback 给尚未迁移的路由。

**Phase 2（本次完成）**
- `/api/login` 已迁成命名原生路由；写值、命令、规则发布和其余 API 由 fallback 在进程内交给现有处理器。
- Encore 不再反向代理到第二个 HTTP 服务；Simple IoT 内置 HTTP 监听关闭，UI 由 Encore 直接提供，带原有安全头、SPA 回退和缓存策略。
- 命名原生路由覆盖健康、登录、设备台账、报警统计；其余路由保留 fallback 以维持现有契约，后续可按需拆成命名端点。

**Phase 3（本次完成）**
- 生产部署使用 Encore Docker 镜像；旧单 exe 只作为离线回滚包。
- 新增 `scripts/verify-encore.sh`：构建镜像、启动容器、检查原生只读路由并跑浏览器回归。
- 更新验收文档与证据；现场工控机部署仍需单独验收。

## 7. 回滚

- 代码回滚：直接构建并运行 `cmd/easyems`，不需要恢复旧启动代码；`encore.app` 与 `service/` 不参与该入口。
- 部署回滚：继续运行上一版 Windows exe 或上一版容器镜像；数据目录格式未变。
- 数据目录仍是唯一持久化边界；迁移不改变 JetStream、审计、报警维护和历史文件的位置。

## 8. 未完成/风险

- Encore 镜像的自托管运行方式需要在目标工控机上实测；不能只用 `encore run` 代替。
- Simple IoT 内置 HTTP 监听已关闭；Encore 是唯一 HTTP 入口，fallback 只是进程内 long-tail 分发器，不是第二服务。
- `encore.app` 的 `id` 为空，表示未绑定 Encore Cloud；当前只验证了本地运行和自托管 Docker。若以后使用 Encore 平台部署，需要先 `encore app link`。
- Encore 版本升级、构建缓存和 Go 工具链版本要固定，避免“本地能跑、现场不能构建”。
- Encore 本地守护进程占用 9400、9500、9600、9700、9800、9900；集成测试不能使用这些端口（`internal/decision` 已改到 19700/19702/19703）。
- 迁移期间保留两套入口，测试必须明确当前跑的是 Encore 还是旧 exe，不能混用证据。
