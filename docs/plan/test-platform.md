# 测试平台与完整测试链路（v0.13.0+ 实测战役）

> 目标：真实上游 + 真实前后端实测，以测促修；分层测试链 + 隐私边界 + SOP 沉淀。
> 公开层（本文件 + `testbed/compose.template.yml` + `scripts/e2e/smoke.sh`）不含任何
> 主机 IP / 真实凭据；真实值只存在于测试主机的私有 git 仓库（见「隐私边界」）。

## 一、平台架构（测试主机私有层）

```
/opt/metapi-testbed/            # 私有 git 仓库（host-local，不推公开仓）
├── compose.yml                 # 全部服务（真实端口/凭据引用 .env）
├── .env                        # 真实凭据（chmod 600，git-ignored）
├── .env.example                # sanitized 样例（可进私有 git）
├── data/<service>/             # 各上游 SQLite 数据
└── run/                        # 原生二进制迭代位（:4100，fresh data）
```

服务矩阵（全部 127.0.0.1 绑定，本机 SSH 隧道访问）：

| 服务 | 端口 | 角色 | 适配器 |
|------|------|------|--------|
| metapi（ghcr 镜像） | 4000 | 被测系统·镜像一致性参考 | — |
| metapi（原生二进制） | 4100 | 被测系统·快速迭代位 | — |
| new-api v1 | 3001 | 上游（v1 形状） | NewApiAdapter |
| one-api | 3002 | 上游（v0 形状） | OneApiAdapter |
| onehub | 3003 | 上游（新 fork 形状） | NewApiAdapter 变体 |
| sub2api | 3004 | 上游（JWT 形状） | Sub2ApiAdapter |
| cliproxyapi | 3005 | 上游（CLI 桥） | CliProxyApiAdapter |

上游安装方式：镜像有 arm64 的直接用镜像；开源自托管（sub2api/cliproxyapi）在测试主机
`git clone` + `go build`（Go 工具链在主机上；代理用 `GOPROXY=https://goproxy.cn`，主机
出站到 github 受限时用该镜像源）。每个上游首次启动后按其文档初始化 admin（new-api v1
无默认密码，须 `POST /api/setup`）。

## 二、测试链分层

1. **Go 单元测试**：`go test ./... -count=1 -race`（pre-push 门禁）
2. **仓内集成测试**：`e2e/`（fixture upstream：backup/cascade/flow）
3. **真实平台 e2e**：`scripts/e2e/smoke.sh` — 对真实实例执行全链
   （health → admin auth → site detect/create → account login/verify →
   models → balance → checkin → token CRUD → /v1 proxy），幂等可重复跑，
   每步 PASS/FAIL + 失败证据（响应体截断）。env 驱动，不硬编码任何真实值。
4. **前端真实浏览器冒烟**：Playwright 逐页点击（真实后端 + 隧道），
   枚举渲染崩溃 / API 契约 mismatch / console 错误。scratch 目录与脚本在
   operator-local 临时区，不进仓库（沉淀后的固化脚本再评估入仓）。
5. **CI 门禁**：GitHub Actions 12 项 + 容器 e2e（后续接入 smoke.sh）。

## 三、迭代循环（fix → verify）

1. 修 bug（worktree 分支）→ 本地门禁全绿 → PR
2. 交叉编译 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/server`
   （大文件 scp 慢时改传源码 tar 包在主机原生编译）
3. 部署到迭代位 `run/metapi`（:4100，fresh data dir）→ `smoke.sh` 复测
4. 全绿后合 PR → GHCR 镜像更新 → `docker compose pull && up -d` 升级参考位
5. 前端冒烟复跑受影响页面 → 收尾更新文档

## 四、隐私边界（强制）

- **公开仓（metapi-go）只含**：`testbed/compose.template.yml`（占位符）、
  `scripts/e2e/smoke.sh`（env 驱动）、本文档。禁止出现：测试主机 IP/hostname、
  真实 AUTH_TOKEN/密码、真实仓库路径、私有上游地址。
- **私有层**：测试主机的 `/opt/metapi-testbed/`（host-local git）；本地机侧
  隧道脚本 + 主机清单在 operator-local（不进公开仓）。
- **约定**：新增凭据一律进 `.env`（chmod 600）；compose 只引用 `${VAR}`；
  提交前自检 `git diff` 不含敏感值。

## 五、SOP 摘要

- **拉起平台**：拷贝模板 → 填 .env → `docker compose up -d` → 各上游初始化 admin
  → 本机起 SSH 隧道（私有脚本）→ 验证 `/health` 200。
- **全链实测**：`METAPI_URL=… METAPI_AUTH_TOKEN=… UPSTREAM_URL=… bash scripts/e2e/smoke.sh`
- **前端冒烟**：Playwright（scratch 目录，复用 `smoke_lib.py`），逐页记录
  console/pageerror/failed-request，只对失败截图。
- **收敛标准**：smoke.sh 无 FAIL；前端冒烟无 crash 级问题；flaky 测试单列跟踪。

## Waves

### Wave 1
- [x] 关 #765（被 #764 取代）；merge #767（前端 URL 崩溃修复）
- [x] #768 修 new-api v1 登录兼容（BaseAdapter+NewApiAdapter）+ smoke.sh 首版
- [x] 测试床私有层落位（one-api/sub2api/cliproxyapi 部署 + 迁移 + 私有 git）
- [x] 前端 Playwright 全页冒烟收尾（复用 scratch，快速批量）

### Wave 2
- 按两份 bug 清单批量修复 → 迭代位复测 → 参考位升级
- flaky 测试硬化（TestProxyLogBatchWriter_BackpressureFallsBackToSync CI 超时）
- 补齐 v1 其余形状差异（审计清单见 #768 PR body）

### Wave 3
- 覆盖率缺口分析（go test -cover + vitest coverage）
- smoke.sh 接入 CI（容器 e2e）；前端冒烟脚本沉淀评估
- 文档沉淀（docs/api.md 实测修正、STATE/MASTER 更新）
