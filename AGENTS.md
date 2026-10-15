# Metapi Go 工程规则

Go 网关，React SPA 预构建后嵌入单个服务端二进制。

## 产品契约

- 保持已有 TypeScript Metapi 行为：管理 API 的 camelCase 字段、环境变量及
  alias、持久化数据。代理 API 保留各原生协议的字段、错误和流式语义，不为
  统一风格重命名协议字段。TS 参考检出放在本公开仓外。
- 同时支持 SQLite 和 PostgreSQL。通过 `store.Open(dialect, dsn)` 打开数据库，
  复用 store 的方言处理；裸 `sqlx` 句柄执行参数化查询时须 Rebind。
  修改 SQL 后验证双方言受影响的行为。
- 需要嵌入 SPA 的 Go 构建/测试前，先生成 `web/dist`。生产镜像不依赖
  Node/npm/Bun 运行时。
- 鉴权失败时拒绝访问。凭据和原始私有运行记录不进 Git、日志或前端存储；
  公开测试夹具只用合成凭据。

## 修改方式

- 动手前说明影响结果的假设和取舍；先查源码、调用方与约定，仍有实质疑点时
  澄清。任务明确、局部选择可逆时自行推进，简单任务不增加固定流程。
- 每项修改对应本次目标，沿用既有风格，不顺手重构或清理无关代码；清理本次
  改动造成的无用 import、变量与函数，不为假想需求增加配置或防御分支。
- 新增代码前查已有 owner、helper 和真实调用方。保持一个直接的数据流；
  抽象须对应真实边界或已有消费者。替换实现时同步移除旧路径。
- 清理保持行为，包括错误类型、发生时点、默认值和 wire 输出。先建立通过的
  测试基线；覆盖不足先补行为刻画。修 bug、补未实现功能与保行为重构分开，
  具体方法见[贡献规范](CONTRIBUTING.md#重构与审查)。
- 在信任边界校验输入，不把内部失败静默转换成成功的空结果或默认值。
  有依据的兼容和 best-effort 行为应明确；未支持能力显式失败或标明遗留范围。
- 注释解释约束和取舍。测试验证可观察行为和失败场景，不测试脚手架或
  测试自己赋值的常量。跨层功能核对真实生产者、存储和消费者。
- 保留其他任务的 WIP。使用短命分支；并发工作在独立 worktree 进行，
  不为集成本任务切换或修改主工作树；合并后移除对应 worktree 与本地分支。

## 验证与交付

- push 前运行 `.githooks/pre-push-project` 完整本地门禁：前端检查/构建、
  Go build、vet、全量 race 测试。可直接执行
  `bash .githooks/pre-push-project </dev/null`。CI 是复核，不替代本地验证，
  也不作为调试环境。
  这是 coding agent 的执行要求；公共贡献者的可选 hook 和环境例外见
  [贡献规范](CONTRIBUTING.md#本地验证)，不要求其安装个人工具链。
- 只在输入完全相同且验证范围适用时复用结果。记录 commit、未提交 diff 和
  相关环境；运行态结论还须绑定产物。夹具、真实模型转发和下游客户端验收
  分开报告，见[测试策略](docs/testing.md)。
- 新增或修改门禁时，用真实检测入口验证已知坏例失败、正常对照通过；
  读取/解析失败须报错，意外空扫描不得通过。检测范围和例外必须明确。
- 所有改动经 PR → squash merge 进入受保护的 `master`。不直接 push 主线，
  不绕过失败门禁；发布决策、紧急例外和版本规则见
  [Git 工作流](docs/internal/git-workflow.md)。合并不代表获准发版或部署。

## 按需入口

| 任务 | 权威来源 |
| --- | --- |
| 查找文档 | [文档地图](docs/README.md) |
| 修改后端职责或依赖 | [架构](docs/architecture.md)、[依赖门禁](docs/package_boundary_test.go)、[后端契约](docs/internal/design/BACKEND.md) |
| 修改前端 | [前端规则](web/AGENTS.md) |
| 修改 API、配置或迁移 | [API](docs/api.md)、[配置](docs/configuration.md)、[迁移](docs/migration.md)；环境变量变化同步 `.env.example` |
| 重构、测试或 lint | [贡献规范](CONTRIBUTING.md)、[测试策略](docs/testing.md)；机械规则由工具配置和可执行检查维护 |
| PR、依赖升级或发布 | [Git 工作流](docs/internal/git-workflow.md) |

开放工作记录在 GitHub issues；发布和 `CHANGELOG.md` 记录版本历史。
公开 `docs/` 只放可复用的产品/开发说明，私有部署细节和原始验收记录不进
发布树。`.local/` 草稿须 ignored，不作为任务状态或凭据存储。
开发文档优先简体中文；代码标识、命令和协议字段保留英文。
