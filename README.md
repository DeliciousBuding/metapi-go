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

<p align="center">
  <a href="#特性"><strong>特性</strong></a> ·
  <a href="#界面预览">界面</a> ·
  <a href="#快速开始3-分钟"><strong>快速开始</strong></a> ·
  <a href="#部署与配置">部署与配置</a> ·
  <a href="#从-typescript-版迁移">迁移</a> ·
  <a href="#已知限制">限制</a>
</p>

---

## 什么是 Metapi

AI 生态里基于 New API / One API 系列的聚合中转站越来越多，多站点的余额、模型列表和 API 密钥往往分散在各处。**Metapi** 是这些中转站之上的**元聚合层（Meta-Aggregation Layer）**：把多个站点统一到**一个入口**，下游所有工具（Cursor、Claude Code、Codex、Open WebUI 等）即可无感接入全部模型。

支持的上游：

- **聚合面板**：New API、One API、OneHub、DoneHub、Veloera、AnyRouter、Sub2API
- **通用兼容接口**：OpenAI / Claude / Gemini 兼容端点，以及 `cliproxyapi` / CPA
- **OAuth 连接**：Codex、Claude、Gemini CLI、Antigravity

## 特性

| 能力               | 说明                                                                                                   |
| ------------------ | ------------------------------------------------------------------------------------------------------ |
| **16 个上游适配器** | New API / One API / OneHub / DoneHub / Veloera / AnyRouter / Sub2API / OpenAI / Claude / Gemini / Gemini CLI / Codex / Antigravity / Grok / CLIProxyAPI / SenseTime |
| **统一代理**        | OpenAI 与 Claude 双协议：Chat / Responses / Messages / Embeddings / Images / Models / Files，全量 SSE 流式，自动互转 |
| **路由与容错**      | 模型自动发现、零配置建路由；按成本/余额/使用权重分配多通道；失败通道自动冷却并重试下一通道；运行时熔断 + half-open 探测 |
| **计费真值**        | 四级成本信号（实测 → 账号配置 → models.dev 目录参考价 → 兜底）；使用日志逐请求记录 Token 与成本 |
| **管理 UI**         | 站点 / 账号 / 路由 / 模型 / 日志 / 告警一站管理，React SPA 预构建后嵌入二进制，无需额外前端服务 |
| **运营自动化**      | 定时签到、定时余额刷新、九种告警渠道、模型批量验证、审计日志、实时 QPS 面板 |
| **轻量部署**        | 单二进制即跑；SQLite 默认、PostgreSQL 可选；启动自动执行幂等 schema 升级 |
| **TS 版无缝接管**   | 同数据库 Schema、同环境变量名、同 API 契约；停旧服务、用同样环境变量启动即完成接管 |

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

## 快速开始（3 分钟）

三种等价方式，任选其一。启动只需要两个令牌：`AUTH_TOKEN`（管理后台登录）与 `PROXY_TOKEN`（下游调用 `/v1/*` 的 Key）。

### 方式一：Release 二进制（推荐）

发布页提供 Linux / macOS / Windows 预编译二进制，单文件即跑：

```bash
curl -fsSL https://github.com/DeliciousBuding/metapi-go/releases/download/v0.16.6/install.sh | bash

export AUTH_TOKEN=$(openssl rand -hex 16)      # 管理后台登录令牌
export PROXY_TOKEN=sk-$(openssl rand -hex 24)  # 下游客户端调用 /v1/* 的 Key
metapi
```

脚本自动校验 SHA-256 并安装到 `/usr/local/bin/metapi`（默认安装最新发布，`METAPI_VERSION` 可钉住版本，`METAPI_INSTALL_PREFIX` 可换安装目录）。Windows 直接从 [Releases](https://github.com/DeliciousBuding/metapi-go/releases/latest) 下载 `metapi-windows-amd64.exe`。

### 方式二：Docker（命名卷，零配置）

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

命名卷首次挂载自动继承镜像内属主，开箱即用；改用 bind mount（`./data:/app/data`）需先在宿主机 `chown -R 1001:1001 ./data`。Compose 方式（生产硬化配置）：

```bash
cp .env.example .env   # 填入 AUTH_TOKEN / PROXY_TOKEN / ACCOUNT_CREDENTIAL_SECRET
docker compose -f docker-compose.prod.yml up -d
```

### 方式三：源码构建

需要 Go 1.26+ 与 Bun 1.x。前端必须先构建——产物经 `go:embed` 打包进二进制，跳过会报 `pattern dist: no matching files found`：

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
cd web && bun install --frozen-lockfile && bun run build:web && cd ..
go build -o metapi ./cmd/server
AUTH_TOKEN=your-admin-token PROXY_TOKEN=your-proxy-sk-token ./metapi
```

### 验证

```bash
curl http://localhost:4000/health
# {"status":"ok"}
curl http://localhost:4000/ready
# {"status":"ok","database":"ok"}
```

打开 `http://localhost:4000`，用 `AUTH_TOKEN` 登录。数据默认存放在 `./data`（SQLite）。端口被占用时用 `PORT=<端口>` 覆盖（默认 4000）。

## 第一个代理请求

在界面里添加至少一个上游站点与账号、执行「重建全部路由」后（完整流程见 [快速上手](docs/getting-started.md)），像调用 OpenAI 一样调用 Metapi：

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<任意已路由模型>",
    "messages": [{ "role": "user", "content": "hello" }]
  }'
```

Metapi 自动在所有上游站点中选择成本最优、状态健康的通道；通道失败自动冷却并重试下一个。未配置任何路由时返回如实的 503：

```json
{ "error": { "message": "No available channels", "type": "server_error", "request_id": "…" } }
```

Claude 原生格式（`/v1/messages`）、Responses、Embeddings、Images、`/v1/models` 与 `/v1/files` 同样支持；完整端点清单见 [HTTP API](docs/api.md)，客户端接入见 [client-integration](docs/client-integration.md)。

---

## 部署与配置

全部配置由环境变量驱动，启动只需两个必填项：

| 变量          | 说明                          |
| ------------- | ----------------------------- |
| `AUTH_TOKEN`  | 管理后台登录令牌              |
| `PROXY_TOKEN` | 下游客户端调用 `/v1/*` 的 Key |

常用项：

| 变量                        | 默认值              | 说明                                          |
| --------------------------- | ------------------- | --------------------------------------------- |
| `ACCOUNT_CREDENTIAL_SECRET` | 回退 `AUTH_TOKEN`   | 上游凭据加密密钥，建议 32+ 字节随机串         |
| `PORT`                      | `4000`              | 监听端口                                      |
| `DATA_DIR`                  | `./data`            | 数据目录（SQLite 库与上传文件）               |
| `DATABASE_URL`              | 空                  | PostgreSQL 连接串；留空使用 SQLite            |
| `LOG_LEVEL`                 | `info`              | 日志级别：`debug` / `info` / `warn` / `error` |
| `CHECKIN_CRON`              | `0 8 * * *`         | 签到时间                                      |
| `BALANCE_REFRESH_CRON`      | `0 * * * *`         | 余额刷新频率                                  |

完整清单（约 150 项）见 [配置参考](docs/configuration.md) 与 [`.env.example`](.env.example)。

**健康检查**：`GET /health`（liveness）、`GET /ready`（readiness，检查数据库）；Docker 镜像内置 `metapi healthcheck` 等价探测 `/ready`。

**深入阅读**：

| 文档                                                     | 用途                                                  |
| -------------------------------------------------------- | ----------------------------------------------------- |
| [docs/getting-started.md](docs/getting-started.md)       | **快速上手**：安装到第一个代理请求                    |
| [docs/deployment.md](docs/deployment.md)                 | 反向代理 / TLS / PostgreSQL / 备份 / 升级回滚         |
| [docs/configuration.md](docs/configuration.md)           | 环境变量完整参考                                      |
| [docs/client-integration.md](docs/client-integration.md) | 客户端接入（Cursor / Claude Code / Codex / Open WebUI）|
| [docs/api.md](docs/api.md)                               | HTTP API 端点清单                                     |
| [docs/migration.md](docs/migration.md)                   | TS → Go 迁移（SQLite / PG / MySQL）                   |
| [docs/faq.md](docs/faq.md)                               | 常见问题                                              |
| [docs/architecture.md](docs/architecture.md)             | 包结构与请求路径（开发者）                            |
| [CHANGELOG.md](CHANGELOG.md)                             | 版本变更                                              |

## 从 TypeScript 版迁移

数据库 Schema 完全一致：停止旧服务，用同样的环境变量启动 Go 版即可，启动时自动执行幂等迁移。SQLite / PostgreSQL 直接接管，MySQL 需先经 TypeScript 版内置迁移转出。Go 镜像以 uid 1001 运行，旧版 root 写入的 bind mount 目录需先 `chown -R 1001:1001 ./data`（命名卷无需处理）。完整步骤、`metapi-migrate` 工具与回滚方案见 [迁移指南](docs/migration.md)。

## 已知限制

- 少量管理端点当前如实返回 `501`（未实现），清单见 [api.md](docs/api.md) 中的「501 残留」标注。
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

## 验证说明

本项目的文档命令按以下口径核实，集中说明一次，正文不再重复：

| 路径                                  | 核实口径                                             |
| ------------------------------------- | ---------------------------------------------------- |
| Release 二进制：安装 → 启动 → 健康检查 | 端到端实测（v0.16.6）                                |
| 源码构建：前端构建 → go build → 启动   | 端到端实测                                           |
| Docker / Compose 命令                  | 与仓库 `Dockerfile` / `docker-compose.prod.yml` 逐项核对 |
| 未配置路由返回 503、健康检查退出码     | 实测                                                 |

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
