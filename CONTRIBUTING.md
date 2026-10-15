# 参与 Metapi Go 开发

所有改动通过 PR 合入受保护的 `master`。合并表示通过集成门禁，不代表稳定发布。

## 开发环境

- Go 精确版本见 `go.mod`。
- 前端使用 Bun 和 Node.js；版本/依赖由 CI 配置和 `web/package.json` 维护。
  Node.js 用于前端检查脚本，不是生产运行时依赖。
- Go 二进制嵌入 `web/dist`，新 worktree 首次运行构建或集成测试前先生成 SPA：

```bash
(cd web && sh ../scripts/bun-install.sh && bun run build:web)
```

独立 worktree 使用自己的依赖目录，不共享可变的 `node_modules`。
本地 Git hook 可选；不要求安装个人 agent 工具或全局 hook 管理器。
希望每次 push 自动检查时，可启用仓库入口：

```bash
git config core.hooksPath .githooks
```

## 本地验证

开发时按改动运行定向测试、类型检查和 lint。
完整本地检查可直接运行，不依赖 hook 是否安装：

```bash
bash .githooks/pre-push-project </dev/null
```

它运行前端测试、类型、lint、格式、死代码检查和构建，再执行 Go build、vet、
全量 race 测试。Windows 的 race 检查通过 `scripts/go-race.sh` 进入 WSL。
PR 合并须通过完整 CI；未在本地运行的检查如实写明，不把环境限制伪装成通过。
CI 不作为本地调试环境。测试分层、PostgreSQL 夹具和真实上游
验收见[测试策略](docs/testing.md)。

### 规则由谁维护

| 检查 | 实现来源 |
| --- | --- |
| Go 格式、静态检查、未使用代码 | gofmt、vet、`.golangci.yml` |
| TypeScript、React 和格式 | `web/package.json`、`web/.oxlintrc.json`、`web/.oxfmtrc.json` |
| 前端未使用文件/导出 | `web/knip.config.ts` |
| 项目依赖、SQL 和文档约束 | `docs/*_test.go`、前端 `check:*` scripts |
| 本地与 CI 接线 | `.githooks/pre-push-project`、`.github/workflows/main.yml` |

工具配置维护机械规则，文档说明使用方式和检测范围。lint 的 `error` 阻断，
`warning` 是审查提示；不把所有提示升级为门禁，也不重复实现成熟工具已有的检查。
例外限定到具体规则和范围，说明仍成立的理由，清理对应代码时同步删除过期例外。

函数长度、命名偏好、抽象数量和“AI 味”不是自动判定缺陷的依据。
项目特有门禁只保护可复现问题；改变检测器时用同一个入口验证正常对照、
已知坏例和相关语法变体，证明它确实能阻断。不要用文件计数替代负例，
也不要为每句自然语言规则写措辞快照。

## 重构与审查

1. 查清目标包的职责、已有 helper、调用方和行为契约，再判断重复或抽象是否多余。
   第三方协议差异、信任边界和已有性能理由可以保留相似实现。
2. 修改已有代码前运行测试。未覆盖的行为先补刻画测试；基线失败先诊断，
   不能把无法验证的重写当成安全清理。
3. 清理保持正常和失败行为，包括错误类型/时点、空值、默认值、序列化与协议字节。
   发现 latent bug 或未实现功能时单独说明，作为独立修复，不夹进重构。
4. 一次清理提交处理一种问题。合并重复实现后删除旧路径及本次造成的孤儿，
   运行全量测试；若引入回归，恢复该改动，不靠无关修补把它推绿。
5. 测试通过真实路径验证结果和失败场景，只 mock 必要的不可控边界。
   复查剩余调用方、注释和文档，不以测试数量或删除行数证明质量。

保留有调用方和兼容依据的 fallback；无依据的兜底、无用包装、死代码和
重复实现优先删除。新设计先用已有 owner 和直接实现，确有第二个消费者或
架构边界时再抽象。注释解释代码无法表达的限制和理由。

## PR 与发布

使用短命 `feature/`、`fix/`、`refactor/`、`perf/`、`chore/` 或 `docs/` 分支，
不直接提交到 `master`。push 后开 PR，全部必需 CI 检查通过后
squash merge；合并前把临时 WIP 整理成可审查的主题提交。

提交采用 Conventional Commits，type/scope 保留英文，subject、PR 和 issue 正文
使用中文，代码/协议标识不翻译。分支保护、紧急例外和依赖升级流程见
[Git 工作流](docs/internal/git-workflow.md)。

维护者选择候选产物并核对受影响的用户流程后，明确决定是否发布及版本号。
确定发布时才更新 `CHANGELOG.md` 和 `web/package.json` 版本，
本次重构或合并不自动触发稳定发布。

## 文档与隐私

开发文档优先简体中文，英文 README 保留给英文读者。API、配置和迁移变更同步
对应契约；不复制版本、依赖和目录清单到多个规则文件。
管理 API 保持已有 camelCase，代理保留原生协议字段；环境变量名称、
alias 和优先级以配置实现、`.env.example`、配置文档为准。

公开仓只放可复用、脱敏的产品和开发内容；凭据、内部主机、本地路径、
原始运行记录和临时任务流水不进 Git。开放工作使用 GitHub issues。
安全问题通过私密渠道报告，见 [SECURITY.md](SECURITY.md)，不公开发 issue。

参与开发须遵守[行为准则](CODE_OF_CONDUCT.md)，贡献采用仓库 [LICENSE](LICENSE)。
