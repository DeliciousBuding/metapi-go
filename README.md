<p align="center">
  <img src="docs/assets/hero.png" alt="Metapi Go" width="720">
</p>

<h1 align="center">Metapi Go</h1>

<p align="center">
  <strong>中转站的中转站 — 将分散的 AI API 站点聚合为一个统一网关</strong>
</p>

<p align="center">
  Metapi 的 Go 语言重写 · 单二进制部署 · 与原 TypeScript 版功能对等
</p>

<p align="center">
  <a href="README.md"><strong>中文</strong></a> ·
  <a href="README_EN.md">English</a>
</p>

<p align="center">
  <a href="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml"><img alt="CI" src="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml/badge.svg?branch=master"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/DeliciousBuding/metapi-go?logo=github&label=release&color=blue"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/stargazers"><img alt="Stars" src="https://img.shields.io/github/stars/DeliciousBuding/metapi-go?style=social"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/forks"><img alt="Forks" src="https://img.shields.io/github/forks/DeliciousBuding/metapi-go?style=social"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26.6-00ADD8?logo=go">
  <img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?logo=react">
  <img alt="Bun" src="https://img.shields.io/badge/Bun-≥1.0-000000?logo=bun&logoColor=white">
  <a href="https://github.com/DeliciousBuding/metapi-go/pkgs/container/metapi-go"><img alt="Docker" src="https://img.shields.io/badge/ghcr-latest-2496ED?logo=docker&logoColor=white"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-3DA639?logo=opensourceinitiative&logoColor=white"></a>
</p>

<p align="center">
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-16-blueviolet">
  <img alt="Notifications" src="https://img.shields.io/badge/notifications-9%20channels-success">
  <img alt="DB" src="https://img.shields.io/badge/DB-SQLite%20%7C%20PostgreSQL-informational">
  <img alt="Image" src="https://img.shields.io/badge/image-15MB-orange">
  <img alt="Memory" src="https://img.shields.io/badge/memory-~20MB-9cf">
</p>

---

## 介绍

把你在各处注册的 New API / One API / OneHub / DoneHub / Veloera / AnyRouter / Sub2API 等站点，汇聚成**一个 API Key、一个入口**，自动发现模型、智能路由、成本最优。

Metapi 作为中转站之上的**元聚合层**，把多个站点统一到一个入口，下游所有工具（Cursor、Claude Code、Codex、Open WebUI 等）即可无感接入全部模型。当前支持的上游范围不止传统聚合面板，还包括：

- 聚合面板：New API、One API、OneHub、DoneHub、Veloera、AnyRouter、Sub2API
- 通用兼容接口：OpenAI、Claude、Gemini 兼容端点，以及 `cliproxyapi`
- OAuth 连接：Codex、Claude、Gemini CLI、Antigravity

| 痛点                               | Metapi 怎么解决                                          |
| ---------------------------------- | -------------------------------------------------------- |
| 每个站点一个 Key，下游工具配置一堆 | **统一代理入口**，模型自动聚合到 `/v1/*`                 |
| 不知道哪个站点用某个模型最便宜     | **智能路由**自动按成本、余额、使用率选最优通道           |
| 某个站点挂了，手动切换好麻烦       | **自动故障转移**，一个通道失败自动冷却并切到下一个       |
| 余额分散在各处，不知道还剩多少     | **集中看板**一目了然，余额不足自动告警                   |
| 每天得去各站签到领额度             | **自动签到**定时执行，奖励自动追踪                       |
| 不知道哪个站有什么模型             | **自动模型发现**，上游新增模型零配置出现在你的模型列表里 |

### Go 版有什么不同

和原 TypeScript 版功能完全一致，换个运行时：

|             | Node.js（原版）  | Go（本版）     |
| ----------- | ---------------- | -------------- |
| 内存占用    | ~85 MB           | ~20 MB         |
| Docker 镜像 | ~250 MB          | ~15 MB         |
| 启动时间    | 5-10 秒          | 即时           |
| 部署方式    | 需要 Node 运行时 | 单个二进制文件 |

---

## 快速开始

### Docker

```bash
docker run -d --name metapi \
  -p 4000:4000 \
  -e AUTH_TOKEN=your-admin-token \
  -e PROXY_TOKEN=your-proxy-token \
  -e TZ=Asia/Shanghai \
  -v ./data:/app/data \
  --restart unless-stopped \
  ghcr.io/deliciousbuding/metapi-go:latest
```

启动后访问 `http://localhost:4000`，用 `AUTH_TOKEN` 登录。

> 请务必修改 `AUTH_TOKEN` 和 `PROXY_TOKEN`，不要使用默认值。数据存储在 `./data` 目录，升级不会丢失。

### Docker Compose

```bash
mkdir metapi && cd metapi

cat > docker-compose.yml << 'EOF'
services:
  metapi:
    image: ghcr.io/deliciousbuding/metapi-go:latest
    ports:
      - "4000:4000"
    volumes:
      - ./data:/app/data
    environment:
      AUTH_TOKEN: ${AUTH_TOKEN:?required}
      PROXY_TOKEN: ${PROXY_TOKEN:?required}
      CHECKIN_CRON: "0 8 * * *"
      BALANCE_REFRESH_CRON: "0 * * * *"
      TZ: Asia/Shanghai
    restart: unless-stopped
EOF

export AUTH_TOKEN=your-admin-token
export PROXY_TOKEN=your-proxy-token
docker compose up -d
```

### 从源码

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
go build -o metapi ./cmd/server
AUTH_TOKEN=admin PROXY_TOKEN=proxy-token ./metapi
```

Windows 本地运行且未设置 `HOST` 时，默认仅监听 `127.0.0.1`，避免 `go run` 或临时构建路径反复触发入站防火墙提示。需要局域网访问时显式设置 `HOST=0.0.0.0`，并自行收紧防火墙范围。

---

## 核心功能

### 统一代理网关

兼容 **OpenAI** 与 **Claude** 下游格式，对接所有主流客户端。支持 Chat Completions、Responses、Messages、Completions、Embeddings、Images、Models，以及标准 `/v1/files` 文件接口。完整的 SSE 流式传输，自动格式转换。

### 智能路由引擎

自动发现所有上游站点的可用模型，零配置生成路由表。多通道概率分摊，基于成本、余额、使用率加权分配。失败通道自动冷却与避让，请求失败自动重试切到其他可用通道。

### 多平台聚合管理

| 平台                     | 适配器                          | 说明                 |
| ------------------------ | ------------------------------- | -------------------- |
| New API                  | `new-api`                       | 新一代大模型网关     |
| One API                  | `one-api`                       | 经典 OpenAI 接口聚合 |
| OneHub                   | `onehub`                        | One API 增强分支     |
| DoneHub                  | `done-hub`                      | OneHub 增强分支      |
| Veloera                  | `veloera`                       | API 网关平台         |
| AnyRouter                | `anyrouter`                     | 通用路由平台         |
| Sub2API                  | `sub2api`                       | 订阅制中转平台       |
| OpenAI / Claude / Gemini | `openapi` / `claude` / `gemini` | 标准兼容接口         |

各平台适配器覆盖模型枚举、余额查询、Token 管理、代理接入等通用能力。

### 账号与 Token 管理

多站点多账号，每个账号可持有多个 API Token。凭证加密存储在本地数据库中。Token 过期自动重新登录获取新凭证，禁用站点自动级联禁用所有关联账号。

### 自动签到

Cron 定时执行（默认每日 08:00），智能解析奖励金额，签到失败自动通知。按账号启用/禁用控制，完整签到日志与历史查询。

### 余额管理

定时余额刷新（默认每小时），批量更新所有活跃账号。收入追踪：每日/累计收入与消费趋势分析（含余额流入 vs 消费的会计恒等式推导）。凭证过期自动重新登录。

### 告警通知

支持九种通知渠道：Webhook、Bark、Server酱、Telegram Bot、SMTP 邮件、飞书（HMAC 加签）、钉钉（HMAC 加签）、企业微信、ntfy。告警场景包括余额不足预警、站点/账号异常、签到失败、代理请求失败、Token 过期提醒、每日摘要报告。可按告警类型逐项静音。

### 运营与审计

管理操作审计日志（写入操作留痕，管理员可查）。实时 QPS/成功率运维面板（WebSocket 推流，自动重连）。批量模型验证、模型倍率总览与行内编辑、模型重定向映射（canonical→actual 自动生成与修复）、账号/站点标签系统。余额流入 vs 消费分析（按快照日推导收入与支出）。可分享仪表盘快照 PNG 导出。

### 轻量部署

单 Docker 容器，默认本地数据目录部署，支持外接 PostgreSQL 运行时数据库。Go 单二进制，15MB 镜像，启动即时。数据完整导入导出，迁移无忧。

---

## 配置

所有环境变量与原 TypeScript 版完全一致，无缝替换。

| 变量                                                     | 默认值                                    | 说明                                                                                                                          |
| -------------------------------------------------------- | ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `AUTH_TOKEN`                                             | `change-me-admin-token`                   | 管理员令牌                                                                                                                    |
| `PROXY_TOKEN`                                            | `change-me-proxy-sk-token`                | 代理 API Key                                                                                                                  |
| `PROXY_MAX_BUFFERED_RESPONSE_BYTES`                      | `20971520`                                | 非流式上游响应的最大缓冲字节数，默认 20 MiB，超限返回 502                                                                     |
| `METAPI_ENABLE_PROXY_STUB`                               | 空                                        | 测试/演示用本地代理 stub 开关；生产保持为空，未配置上游转发时返回 503                                                         |
| `PORT`                                                   | `4000`                                    | 监听端口                                                                                                                      |
| `HOST`                                                   | Windows: `127.0.0.1`；其他平台: `0.0.0.0` | 显式值总是优先；容器配置固定为 `0.0.0.0`                                                                                      |
| `DB_TYPE`                                                | `sqlite`                                  | 数据库类型（`sqlite` / `postgres`）；提供 PostgreSQL URL 时可自动推断为 `postgres`                                            |
| `DATABASE_URL` / `DB_URL`                                | 空                                        | PostgreSQL 连接串或 SQLite 文件路径；`DB_URL` 优先，`DATABASE_URL` 用于兼容部署平台                                           |
| `DB_SSLMODE`                                             | 空                                        | PostgreSQL TLS 模式；支持 `disable`、`allow`、`prefer`、`require`、`verify-ca`、`verify-full`；非空时覆盖连接串中的 `sslmode` |
| `DB_PROFILE`                                             | `normal`                                  | 池预设：`shared-tiny`(2/1) / `normal`(10/3) / `dedicated`(20/5)；显式 `DB_MAX_*` 覆盖                                         |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS`                | 跟随 profile                              | PostgreSQL 应用池预算；生产值不得超过数据库 role connection limit                                                             |
| `DB_CONN_MAX_LIFETIME_SEC` / `DB_CONN_MAX_IDLE_TIME_SEC` | `1800` / `300`                            | PostgreSQL 连接寿命与空闲回收时间（秒）                                                                                       |
| `TRUSTED_PROXY_CIDRS`                                    | 空                                        | 允许提供 `X-Forwarded-For` / `X-Real-IP` 的反向代理 CIDR CSV；默认忽略 forwarded headers                                      |
| `ADMIN_CORS_ALLOWED_ORIGINS`                             | 空                                        | 允许跨域访问 `/api/*` 管理接口的精确 `http(s)` origin CSV；默认只支持同源管理 UI，禁止 `*`                                    |
| `CHECKIN_CRON`                                           | `0 8 * * *`                               | 签到时间                                                                                                                      |
| `BALANCE_REFRESH_CRON`                                   | `0 * * * *`                               | 余额刷新频率                                                                                                                  |

当前运行时支持两种数据库形态：单进程 SQLite；PostgreSQL 生产部署。PostgreSQL 模式下，产生外部请求、通知、上传、清理或同步副作用的后台任务使用 PG advisory lock，避免多副本重复执行同一批任务。可选 `REDIS_URL` / `METAPI_REDIS_URL` 仅用于多实例下游 Key 的 **RPM/TPM admission** 共享计数（`auth.ConfigureSharedAdmissionFromRedisURL` + `internal/sharedcount`，不可达时 fail-open 回退进程内窗口）；留空则无需 Redis 进程。Sticky session 仍是进程内绑定，**不会**因配置 Redis 而跨实例共享（STICKY-B 仍为 residual，非产品）。详见 [`docs/analysis/redis-shared-state.md`](docs/analysis/redis-shared-state.md)。

代理转发没有配置路由和上游依赖时，生产默认返回 HTTP 503。`METAPI_ENABLE_PROXY_STUB=1` 只用于测试或演示，避免把本地假响应误当成真实上游调用。

[`.env.example`](.env.example) 中有完整的环境变量清单。

## 运维健康检查

- `GET /health` 是 liveness，只确认 HTTP 进程存活。
- `GET /ready` 是 readiness，会检查数据库；数据库不可用或进程正在关停时返回 HTTP 503。
- Docker 默认执行 `metapi healthcheck`，等价于探测 `http://127.0.0.1:${PORT}/ready`。
- 可用 `METAPI_HEALTHCHECK_URL` 或 `METAPI_HEALTHCHECK_PATH` 覆盖容器健康检查目标。

---

## 技术栈

| 层       | 技术                                                                                                                                                                        |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 后端     | [chi](https://github.com/go-chi/chi) 路由 + `net/http`                                                                                                                      |
| 语言     | Go 1.26.6                                                                                                                                                                   |
| 数据库   | SQLite / PostgreSQL + [sqlx](https://github.com/jmoiron/sqlx)；可选 Redis 仅用于 RPM/TPM admission（非必需）                                                                |
| 定时任务 | [robfig/cron](https://github.com/robfig/cron)                                                                                                                               |
| 容器化   | Docker（Alpine，15MB 镜像）                                                                                                                                                 |
| 前端     | React 19 + Bun + Rsbuild 2 + TanStack Router/Query/Table + Zustand + Tailwind CSS v4 + shadcn Base UI + OKLCH 设计系统 + Recharts + RHF + Zod + i18next（内嵌于 Go 二进制） |

---

## 前端架构

v0.9.0 起前端采用 [New API](https://github.com/QuantumNous/new-api) 同类的 React 技术栈重写；兼容目标是原 TypeScript 版 Metapi 的 API 契约（camelCase 字段、env var 名）与 DB（SQLite/PG dual dialect），而不是复制上游内部结构。预构建产物经 `go:embed` 打包进单二进制，生产镜像不含 Node/Bun 运行时。

### 模块组织

- **领域化 feature 模块**：认证、仪表盘、站点/账号/通道、导入、签到、路由、模型与测试台、OAuth、可观测性、代理日志、设置和关于页各自归属 `src/features/*`；兼容 URL 只保留薄路由，不伪装成独立 feature。
- **data-table 四层渲染架构 + hooks**：`core`（TanStack table 渲染原语）+ `layout`（响应式页面组合）+ `toolbar`（filter/search/批量操作）+ `static`（本地数组轻量渲染）；`hooks` 提供受控状态与 URL 同步。feature 经统一 `index.ts` 导入，专属列/动作留在各 feature 目录。
- **OKLCH 设计系统**：三层 CSS（`theme.css` 语义 token + `theme-presets.css` 10 套预设 + `index.css` Tailwind 4 入口）；5 轴主题（preset/font/radius/scale/content-layout）经 `<body data-theme-*>` 切换；暗色 class-based + cookie 持久化。Recharts 直接读取语义 CSS 变量。
- **key-based i18n**：i18next + react-i18next，支持 `en` + `zh-CN`；React 组件 `useTranslation()` + `t()`，非 React 模块用 `i18n.t()`；双向 key 一致性由 `web/src/i18n/__tests__/i18n-keys.test.ts` 校验，不在文档固化易漂移计数。

---

## 数据与隐私

Metapi 完全自托管，所有数据（账号、令牌、路由、日志）均存储在你自己的部署环境中，不会向任何第三方发送数据。代理请求仅在你的服务器与上游站点之间直连传输。

---

## 从 TypeScript 版迁移

数据库 Schema 完全一致，Go 版启动时自动执行幂等 migration。停止旧服务，用同样的环境变量启动 Go 版即可。

---

## 文档导航

| 文档                                               | 用途                                    |
| -------------------------------------------------- | --------------------------------------- |
| [docs/README.md](docs/README.md)                   | **文档地图**（先看这个）                |
| [docs/architecture.md](docs/architecture.md)       | 包结构与请求路径                        |
| [docs/progress/MASTER.md](docs/progress/MASTER.md) | 三条交付主线与开放结果                  |
| [docs/benchmark.md](docs/benchmark.md)             | 产品对标与方向（New API × All API Hub） |
| [CHANGELOG.md](CHANGELOG.md)                       | 版本变更                                |

---

## 开发

### 后端（Go）

```bash
make build    # 构建
make test     # 运行全部测试
make vet      # go vet
make lint     # 代码检查
make vuln     # govulncheck 漏洞扫描
make bench-routing  # 路由权重选择 benchmark
make verify   # 本地发布门禁
make docker-verify  # 构建完整 Docker 镜像（需要 Docker）
```

### 前端（`web/`，Bun）

前端代码在 `web/` 目录，独立于 Go 后端，需 Bun >= 1.0。

```bash
cd web
bun install            # 装依赖
bun run dev            # 本地开发（rsbuild dev，/api /v1 代理到后端，默认 http://localhost:4000）
bun run typecheck      # tsgo 类型检查
bun run lint           # oxlint
bun run lint:fix       # oxlint --fix
bun run test           # vitest run（全量）
bun run knip           # 未使用代码检测
bun run build          # rsbuild build（产物经 go:embed 打包进 Go 二进制）
bun run build:check    # tsgo + build（发布前完整检查）
bun run format:check   # oxfmt 格式检查
```

Dev proxy 默认指向 `http://localhost:4000`，可经 `DEV_PROXY_TARGET` / `VITE_DEV_PROXY_TARGET` / `PORT` / `VITE_BACKEND_PORT` 覆盖。

### Windows 本地运行

默认监听 `127.0.0.1`（避免 `go run` 或临时构建路径反复触发入站防火墙提示）。历史遗留的防火墙放行规则可先审计、再精确清理：

```powershell
.\scripts\windows-firewall-maintenance.ps1 -Mode Audit
.\scripts\windows-firewall-maintenance.ps1 -Mode Cleanup -Elevate
```

---

## 贡献与安全

- [CONTRIBUTING.md](CONTRIBUTING.md) — 分支模型、PR 流程、本地门禁
- [SECURITY.md](SECURITY.md) — 漏洞报告（Security Advisory）
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — 社区行为准则
- [docs/git-workflow.md](docs/git-workflow.md) — Git 分支与保护规则

---

## 相关项目

- [Metapi (TypeScript)](https://github.com/cita-777/metapi)，原版 Node.js 实现
- [New API](https://github.com/QuantumNous/new-api)，主要上游之一
- [One API](https://github.com/songquanpeng/one-api)，经典 OpenAI 接口聚合

---

## 许可证

[MIT](LICENSE)
