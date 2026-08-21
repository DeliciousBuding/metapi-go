<p align="center">
  <img src="docs/assets/hero.png" alt="Metapi Go" width="720">
</p>

<h1 align="center">Metapi Go</h1>

<p align="center">
  <strong>中转站的中转站 —— 把分散的 AI API 站点聚合为一个统一网关</strong>
</p>

<p align="center">
  将你在各处注册的 New API / One API / OneHub / Sub2API 等站点，<br>
  汇聚成<strong>一个 API Key、一个入口</strong>：自动发现模型、智能路由、成本最优。
</p>

<p align="center">
  <a href="README.md"><strong>中文</strong></a> ·
  <a href="README_EN.md">English</a>
</p>

<p align="center">
  <a href="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml"><img alt="CI" src="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml/badge.svg?branch=master"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/DeliciousBuding/metapi-go?logo=github&label=release&color=blue"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/pkgs/container/metapi-go"><img alt="Docker" src="https://img.shields.io/badge/ghcr-latest-2496ED?logo=docker&logoColor=white"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26.6-00ADD8?logo=go">
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-3DA639?logo=opensourceinitiative&logoColor=white"></a>
</p>

---

## 界面预览

<table>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/dashboard.webp" alt="仪表盘" style="width:100%;height:auto;"/>
      <div><b>仪表盘</b> — 余额分布、签到与定时任务健康</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/models.webp" alt="模型市场" style="width:100%;height:auto;"/>
      <div><b>模型市场</b> — 跨站模型覆盖、品牌与实测指标</div>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/routes.webp" alt="智能路由" style="width:100%;height:auto;"/>
      <div><b>智能路由</b> — 多通道概率分配、成本优先选路</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/accounts.webp" alt="账号管理" style="width:100%;height:auto;"/>
      <div><b>账号管理</b> — 多站点多账号、健康状态追踪</div>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/sites.webp" alt="站点管理" style="width:100%;height:auto;"/>
      <div><b>站点管理</b> — 上游站点配置与状态一览</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/proxy-logs.webp" alt="使用日志" style="width:100%;height:auto;"/>
      <div><b>使用日志</b> — 代理请求日志与成本明细</div>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/model-tester.webp" alt="模型操练场" style="width:100%;height:auto;"/>
      <div><b>模型操练场</b> — 在线对比不同通道输出</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/settings.webp" alt="系统设置" style="width:100%;height:auto;"/>
      <div><b>系统设置</b> — 全局参数、主题与安全配置</div>
    </td>
  </tr>
</table>

---

## 什么是 Metapi

AI 生态里基于 New API / One API 系列的聚合中转站越来越多，多站点的余额、模型列表和 API 密钥往往分散在各处。**Metapi** 是这些中转站之上的**元聚合层（Meta-Aggregation Layer）**：把多个站点统一到**一个入口**，下游所有工具（Cursor、Claude Code、Codex、Open WebUI 等）即可无感接入全部模型。

支持的上游：

- **聚合面板**：New API、One API、OneHub、DoneHub、Veloera、AnyRouter、Sub2API
- **通用兼容接口**：OpenAI / Claude / Gemini 兼容端点，以及 `cliproxyapi` / CPA
- **OAuth 连接**：Codex、Claude、Gemini CLI、Antigravity

| 痛点                               | Metapi 怎么解决                                          |
| ---------------------------------- | -------------------------------------------------------- |
| 每个站点一个 Key，下游工具配置一堆 | **统一代理入口 + 多下游 Key**，模型自动聚合到 `/v1/*`    |
| 不知道哪个站点用某个模型最便宜     | **智能路由**按成本、余额、使用率自动选最优通道           |
| 站点挂了要手动切换                 | **自动故障转移**，失败通道冷却、请求自动重试下一通道     |
| 余额分散，不知道还剩多少           | **集中看板**一目了然，余额不足自动告警                   |
| 每天得去各站签到领额度             | **自动签到**定时执行，奖励自动追踪                       |
| 不知道哪个站有什么模型             | **自动模型发现**，上游新增模型零配置进入路由             |

本项目是 [Metapi（TypeScript 版）](https://github.com/cita-777/metapi) 的 Go 重写，客户端可见行为保持兼容：

|             | Node.js（原版） | Go（本版）     |
| ----------- | --------------- | -------------- |
| 内存占用    | ~85 MB          | ~20 MB         |
| Docker 镜像 | ~250 MB         | ~15 MB         |
| 启动时间    | 5-10 秒         | 即时           |
| 部署方式    | Node 运行时     | 单个二进制文件 |

---

## 快速开始（3 分钟）

发布页提供 Linux / macOS / Windows 预编译二进制，单文件即跑。

**1. 安装**（Linux / macOS；Windows 直接从 [Releases](https://github.com/DeliciousBuding/metapi-go/releases/latest) 下载 `metapi-windows-amd64.exe`）：

```bash
curl -fsSL https://github.com/DeliciousBuding/metapi-go/releases/latest/download/install.sh | bash
```

脚本自动校验 SHA-256 并安装到 `/usr/local/bin/metapi`。没有 `/usr/local` 写权限时，
用 `METAPI_INSTALL_PREFIX` 指定安装目录（如 `METAPI_INSTALL_PREFIX=~/.local` 会安装到 `~/.local/bin/metapi`）。

**2. 启动**（仅需两个令牌）：

```bash
export AUTH_TOKEN=$(openssl rand -hex 16)      # 管理后台登录令牌
export PROXY_TOKEN=sk-$(openssl rand -hex 24)  # 下游客户端调用 /v1/* 的 Key
metapi
```

**3. 验证**：

```bash
curl http://localhost:4000/health
# {"status":"ok"}
curl http://localhost:4000/ready
# {"status":"ok","database":"ok"}
```

打开 `http://localhost:4000`，用 `AUTH_TOKEN` 登录。数据默认存放在 `./data`（SQLite），删除令牌环境变量不会丢失数据。

> 以上为实测路径；端口被占用时用 `PORT=<端口>` 覆盖（默认 4000）。接下来在界面里添加站点与账号、自动重建路由，即可发出第一个代理请求——完整流程见 [快速上手](docs/getting-started.md)。

## 部署

### Docker

> 以下命令与仓库 `Dockerfile` / `docker-compose.prod.yml` 逐项核对（本环境无 Docker，未实跑）。

```bash
docker run -d --name metapi \
  -p 4000:4000 \
  -e AUTH_TOKEN=your-admin-token \
  -e PROXY_TOKEN=your-proxy-sk-token \
  -e ACCOUNT_CREDENTIAL_SECRET=$(openssl rand -hex 32) \
  -e TZ=Asia/Shanghai \
  -v metapi_data:/app/data \
  --restart unless-stopped \
  ghcr.io/deliciousbuding/metapi-go:latest
```

要点：

- 镜像以非 root 用户（uid 1001）运行。**命名卷**（如上 `metapi_data`）首次挂载自动继承镜像内属主，零配置；改用 `./data:/app/data` 这类 bind mount 时需先在宿主机执行 `chown -R 1001:1001 ./data`，详见 [部署指南](docs/deployment.md)。
- `ACCOUNT_CREDENTIAL_SECRET` 用于加密存储的上游凭据，建议独立生成 32+ 字节随机串；不设置会回退为 `AUTH_TOKEN`。
- 生产环境建议固定版本标签（如 `:v0.16.6`）而非 `latest`。

### Docker Compose

`docker-compose.prod.yml`（GHCR 镜像 + 生产硬化）或 `docker-compose.yml`（本地构建）：

```bash
cp .env.example .env   # 填入 AUTH_TOKEN / PROXY_TOKEN / ACCOUNT_CREDENTIAL_SECRET
docker compose -f docker-compose.prod.yml up -d
```

### 从源码构建

需要 Go 1.26+ 与 Bun 1.x（前端需先构建，产物经 `go:embed` 打包进二进制）：

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
cd web && bun install --frozen-lockfile && bun run build:web && cd ..
go build -o metapi ./cmd/server
AUTH_TOKEN=your-admin-token PROXY_TOKEN=your-proxy-sk-token ./metapi
```

反向代理、PostgreSQL、升级与回滚见 [部署指南](docs/deployment.md)。

---

## 第一个代理请求

在界面里添加至少一个上游站点与账号、执行「重建全部路由」后（见 [快速上手](docs/getting-started.md)），像调用 OpenAI 一样调用 Metapi：

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<任意已路由模型>",
    "messages": [{ "role": "user", "content": "hello" }]
  }'
```

Metapi 自动在所有上游站点中选择成本最优、状态健康的通道；通道失败自动冷却并重试下一个。Claude 原生格式（`/v1/messages`）、Responses、Embeddings、Images、`/v1/models` 与 `/v1/files` 同样支持；完整端点清单见 [HTTP API](docs/api.md)，客户端接入见 [client-integration](docs/client-integration.md)。

未配置任何路由时，代理请求如实返回 503（已实测）：

```json
{ "error": { "message": "No available channels", "type": "server_error", "request_id": "…" } }
```

---

## 核心功能

### 统一代理网关

兼容 **OpenAI** 与 **Claude** 下游格式。Chat Completions、Responses、Messages、Completions（Legacy）、Embeddings、Images、Models 与 `/v1/files`，完整 SSE 流式传输，自动格式转换（OpenAI ⇄ Claude）。

### 智能路由引擎

- 自动发现所有上游模型，**零配置**生成路由表
- 四级成本信号：实测成本 → 账号配置成本 → 目录参考价（models.dev）→ 默认兜底
- 多通道概率分摊，按成本、余额、使用率加权分配
- 失败通道自动冷却与避让，请求自动重试其他通道
- 运行时熔断器 + half-open 探测，恢复中的通道受控重新进入候选集

### 多平台聚合管理

共 **16** 个适配器（`platform/`）：`new-api`、`one-api`、`one-hub`、`done-hub`、`veloera`、`anyrouter`、`sub2api`、`openai`、`claude`、`gemini`、`gemini-cli`、`codex`、`antigravity`、`grok`、`cliproxyapi`、`sensetime`。模型枚举、余额查询、Token 管理、代理接入为通用能力；登录、签到等按平台而异。

### 账号与 Token 管理

多站点多账号，每个账号可持有多个 API Token。`healthy` / `unhealthy` / `degraded` / `disabled` 四级状态机；凭据加密存储；Token 过期自动重新登录；禁用站点级联禁用关联账号。

### 模型广场与操练场

跨站模型覆盖总览、各站定价对比、延迟与成功率实测指标；交互式操练场可强制指定通道对比输出。

### 自动签到与余额

Cron 定时签到（默认每日 08:00），奖励解析与失败通知，并发锁防重复；定时余额刷新（默认每小时），收入追踪与消费趋势分析。

### 告警通知

九种渠道：Webhook、Bark、Server酱、Telegram Bot、SMTP 邮件、飞书、钉钉、企业微信、ntfy。覆盖余额不足、站点/账号异常、签到失败、代理失败、Token 过期与每日摘要，可按类型静音，冷却防重复。

### 运营与审计

管理操作审计日志；实时 QPS / 成功率面板（WebSocket 推流）；批量模型验证、模型倍率编辑、模型重定向映射、标签；仪表盘快照 PNG 导出。

### 轻量部署

单二进制 + 本地数据目录即可运行，也可外接 PostgreSQL；SQLite / PostgreSQL 双 dialect，启动自动执行幂等 schema 升级；数据完整导入导出。

---

## 配置

全部配置由环境变量驱动，启动只需两个必填项：

| 变量          | 说明                          |
| ------------- | ----------------------------- |
| `AUTH_TOKEN`  | 管理后台登录令牌              |
| `PROXY_TOKEN` | 下游客户端调用 `/v1/*` 的 Key |

常用项：

| 变量                   | 默认值        | 说明                                             |
| ---------------------- | ------------- | ------------------------------------------------ |
| `ACCOUNT_CREDENTIAL_SECRET` | 回退 `AUTH_TOKEN` | 上游凭据加密密钥，建议 32+ 字节随机串      |
| `PORT`                 | `4000`        | 监听端口                                         |
| `HOST`                 | 平台相关      | Windows 默认 `127.0.0.1`，其他平台 `0.0.0.0`     |
| `DATA_DIR`             | `./data`      | 数据目录（SQLite 库与上传文件）                  |
| `DATABASE_URL`         | 空            | PostgreSQL 连接串；留空使用 SQLite               |
| `LOG_LEVEL`            | `info`        | 日志级别：`debug` / `info` / `warn` / `error`    |
| `CHECKIN_CRON`         | `0 8 * * *`   | 签到时间                                         |
| `BALANCE_REFRESH_CRON` | `0 * * * *`   | 余额刷新频率                                     |

完整清单（约 150 项：PostgreSQL 池预设、代理限额、速率限制、CORS、可信代理、通知渠道等）见 [配置参考](docs/configuration.md) 与 [`.env.example`](.env.example)。

### 健康检查

- `GET /health` — liveness，HTTP 进程存活即 200
- `GET /ready` — readiness，检查数据库；不可用或关停中返回 503
- Docker 镜像内置 `metapi healthcheck`（等价探测 `/ready`，实测 exit code 正确）

---

## 文档

| 文档                                                   | 用途                                  |
| ------------------------------------------------------ | ------------------------------------- |
| [docs/getting-started.md](docs/getting-started.md)     | **快速上手**：安装到第一个代理请求    |
| [docs/deployment.md](docs/deployment.md)               | 部署 / 反向代理 / PostgreSQL / 升级   |
| [docs/configuration.md](docs/configuration.md)         | 环境变量完整参考                      |
| [docs/client-integration.md](docs/client-integration.md) | 客户端接入（Cursor / Claude Code / Codex / Open WebUI） |
| [docs/api.md](docs/api.md)                             | HTTP API 端点清单                     |
| [docs/migration.md](docs/migration.md)                 | TS → Go 迁移（SQLite / PG / MySQL）   |
| [docs/faq.md](docs/faq.md)                             | 常见问题                              |
| [docs/architecture.md](docs/architecture.md)           | 包结构与请求路径（开发者）            |
| [docs/README.md](docs/README.md)                       | 文档地图                              |
| [CHANGELOG.md](CHANGELOG.md)                           | 版本变更                              |

## 从 TypeScript 版迁移

数据库 Schema 完全一致：停止旧服务，用同样的环境变量启动 Go 版即可，启动时自动执行幂等迁移。Go 镜像以 uid 1001 运行，旧版 root 写入的 bind mount 数据目录需先 `chown -R 1001:1001 ./data`（命名卷无需处理）。SQLite / PostgreSQL 直接接管，MySQL 需先经 TypeScript 版内置迁移转出。完整步骤、`metapi-migrate` 工具与回滚方案见 [迁移指南](docs/migration.md)。

## 已知限制

- 少量管理端点当前如实返回 `501`（未实现），清单见 [api.md](docs/api.md) 中的「501 残留」标注；版本更新提示请走 GitHub Releases / GHCR 镜像 tag。
- 代理上游未配置时返回 503，不做合成成功响应。
- 单进程语义为主；多实例部署的共享语义（Redis 共享 RPM/TPM 准入、PostgreSQL advisory lock）见 [FAQ](docs/faq.md)。

---

## 开发

### 后端（Go）

```bash
make build    # 构建
make test     # 全部测试（含 -race）
make vet      # go vet
make lint     # golangci-lint
make vuln     # govulncheck 漏洞扫描
```

### 前端（`web/`，Bun）

```bash
cd web
bun install
bun run dev         # 本地开发（/api /v1 代理到后端 :4000）
bun run typecheck   # tsgo 类型检查
bun run test        # vitest 全量
bun run build       # rsbuild 构建（产物经 go:embed 打包进 Go 二进制）
```

贡献流程（分支模型、PR 门禁）见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 贡献与安全

- [CONTRIBUTING.md](CONTRIBUTING.md) — 分支模型、PR 流程、本地门禁
- [SECURITY.md](SECURITY.md) — 漏洞报告（Security Advisory）
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — 社区行为准则

## 相关项目

- [Metapi (TypeScript)](https://github.com/cita-777/metapi) — 原版 Node.js 实现，本项目为其 Go 重写
- [New API](https://github.com/QuantumNous/new-api) — 主要上游之一
- [One API](https://github.com/songquanpeng/one-api) — 经典 OpenAI 接口聚合

## 许可证

[MIT](LICENSE)。Metapi 完全自托管：所有数据存储在你自己的部署环境中，代理请求仅在你的服务器与上游站点之间直连传输。
