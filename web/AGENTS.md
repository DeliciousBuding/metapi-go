# 前端工程规则

继承仓库[工程规则](../AGENTS.md)。依赖版本和可执行命令以
[package.json](package.json) 为准；使用 Bun 和已提交的 lockfile，
不无依据移除安全 override。

## 职责与状态

- 功能专属代码留在对应 feature。共享组件和 `lib` 不导入 feature 或 route。
  跨 feature 通过所属 feature 的 `index.ts`，data-table 消费者通过
  `@/components/data-table`。详细约束和例外见
  [模块边界](../docs/internal/web-package-boundaries.md)，由
  `scripts/check-boundaries.mjs` 检查。
- 服务端状态属于 TanStack Query；可导航的表格、搜索和筛选状态属于 URL。
  使用现有 table hooks，不再建立一份本地镜像和写回 effect。
  修改这类状态或回调前读[状态稳定性](../docs/internal/design/state-stability.md)。
- 使用共享 `@/lib/api`、HTTP 和 session helpers。后端使用 HttpOnly session，
  没有 refresh endpoint；不存储 token 或模拟刷新流程。
  非敏感的 session 过期时间元数据不属于凭据。
- 通用错误由 transport 处理，feature mutation 只补领域反馈，避免重复 toast。
  非法 API 数据不得静默变成成功的空列表；有依据的兼容行为保持明确。

## 渲染与表单

- 用户文案使用 `en`、`zh-CN` 双向完整的 i18n key。需要随语言切换更新的
  组件用 `useTranslation()` 渲染；常量存 key，不预先翻译。
- 复用 Base UI/shadcn 原语和语义主题 token。视觉约束见
  [设计系统](../docs/internal/design/DESIGN.md)，组件职责见
  [组件地图](../docs/internal/design/components.md)，不建立平行原语或 token 体系。
- 使用 React Hook Form + Zod 及现有 `Form`、`FormField`、`FormControl`。
  复合控件把注入的 ID、ARIA 属性传给可访问根节点，保留校验聚焦和 label 关联；
  验证 accessible name、键盘行为和 invalid 状态。
- 不可信内容按文本渲染，不把业务数据注入 `dangerouslySetInnerHTML`。
  icon-only 控件有可访问名称，状态不只靠颜色；细节见
  [可访问性检查](../docs/internal/design/a11y-checklist.md)。
- 构建常量/dev proxy 来自 `config/build-shared.ts`；Rsbuild 的 TanStack Router
  plugin 是路由生成和生产拆包入口。提交 `src/routeTree.gen.ts`，
  不再增加另一套生成器或拆包机制。

## 验证

- 修改后运行定向行为测试、`bun run typecheck`、`bun run lint`；
  push 前仍须完整门禁。格式、死代码和构建命令由 package scripts 维护。
- 新测试放模块的 `__tests__/`，通过真实代码验证用户可见输出和交互。
  只在必要的网络、时间、存储、浏览器 API 边界 mock，不 mock 被测模块。
- 修改相关流程时验证 loading、empty、error 和异步失败状态，不更新 snapshot
  来掩盖未解释的失败。测试分层和门禁负例方法见[测试策略](../docs/testing.md)。
