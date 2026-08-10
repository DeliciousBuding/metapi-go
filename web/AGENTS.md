最后更新：2026-08-11 12:10

# 前端开发规范

本文档定义 metapi-go 前端项目的开发规范与最佳实践，供开发与 AI 助手共同遵循。具体依赖与脚本以 `package.json` 为准。前端 100% 对齐 newapi 技术栈（详见仓库根 `docs/` 重写规划）。

---

## 一、项目概览

### 技术栈

| 类别       | 技术                                                              |
| ---------- | ----------------------------------------------------------------- |
| 包管理     | Bun                                                               |
| 构建       | Rsbuild 2、tsgo（TS 原生编译器）                                  |
| 框架       | React 19、TypeScript                                              |
| 路由       | @tanstack/react-router（文件路由 + 自动生成 + validateSearch）    |
| 数据与请求 | @tanstack/react-query、axios、Zustand                             |
| 表格与列表 | @tanstack/react-table、@tanstack/react-virtual                    |
| 国际化     | i18next、react-i18next、i18next-browser-languagedetector          |
| 日期       | Day.js                                                            |
| UI 与样式  | Base UI、shadcn/ui base-nova、HugeIcons（免费层）+ lucide、Tailwind CSS 4、clsx / class-variance-authority |
| 字体       | @fontsource-variable/public-sans + lora                           |
| 图表       | @visactor/vchart、recharts（钉版备用）                            |
| 表单       | React Hook Form、Zod                                              |
| 动画/反馈  | motion、sonner、vaul、cmdk                                        |
| Markdown   | marked、shiki、katex、dompurify                                   |
| 拖拽       | @dnd-kit/core + sortable + utilities                              |
| 工具       | oxfmt、oxlint、knip、vitest                                       |

优先选用成熟、维护良好的开源库；仅在现有库无法满足或需特殊适配时自行实现，并评估可维护性与通用性。HugeIcons 仅用免费层（`@hugeicons/core-free-icons`），付费图标用 lucide 兜底。

---

## 二、目录

- [一、项目概览](#一项目概览)
- [二、目录](#二目录)
- [三、开发规范](#三开发规范)
  - [3.1 国际化](#31-国际化)
  - [3.2 代码风格与类型](#32-代码风格与类型)
  - [3.3 组件](#33-组件)
  - [3.4 性能](#34-性能)
  - [3.5 状态管理](#35-状态管理)
  - [3.6 API 请求](#36-api-请求)
  - [3.7 表单](#37-表单)
  - [3.8 路由](#38-路由)
  - [3.9 错误处理](#39-错误处理)
  - [3.10 样式](#310-样式)
  - [3.11 文件组织](#311-文件组织)
  - [3.12 可访问性](#312-可访问性)
  - [3.13 安全](#313-安全)
  - [3.14 测试](#314-测试)
  - [3.15 依赖管理](#315-依赖管理)
  - [3.16 构建与部署](#316-构建与部署)
- [四、协作与提交](#四协作与提交)
- [更新日志](#更新日志)

---

## 三、开发规范

### 3.1 国际化

- **页面文本**：所有面向用户的文案均需支持 i18n，使用 `useTranslation()` 的 `t()` 进行翻译。支持 7 语言（en/zhCN/fr/ru/ja/vi/zhTW），`scripts/sync-i18n.mjs` 同步补齐。
- **使用场景**
  - **React 组件**：必须使用 `const { t } = useTranslation()`，以保证语言切换时组件会重新渲染。
  - **非 React 环境**（工具函数、常量、类方法）：可使用 `import { t } from 'i18next'`；此类用法不会随语言切换自动更新，仅在不依赖响应式更新的场景使用。
  - 即使父组件已使用 `useTranslation()`，子组件仍应自行使用，以保证独立性。
- **专有名词**：品牌、产品、技术术语等可保留英文（如 API、React、TypeScript）；若有约定俗成的译法则使用翻译。
- **翻译键**：使用有层级、语义清晰的键名，如 `dashboard.overview.title`，并保持命名一致。

- **枚举与文案（常量中的 i18n）**
  各 feature 的 `constants.ts` 中常出现「枚举/状态 + 展示文案」或「成功/错误消息」，须统一约定以免遗漏 i18n、用法混乱：
  - **成功/错误/提示类消息**（如 `SUCCESS_MESSAGES`、`ERROR_MESSAGES`）：常量值仅表示 **i18n 键**（与英文 fallback 同字面量）。展示时**必须**通过 `t()` 使用，例如 `toast.success(t(SUCCESS_MESSAGES.ACCOUNT_CREATED))`、`toast.error(t(ERROR_MESSAGES.UNEXPECTED))`，**禁止**直接 `toast.success(SUCCESS_MESSAGES.xxx)` 当作最终文案。
  - **状态/选项的 label**：在常量中统一用 **labelKey**（字符串，即 i18n 键），组件中通过 `t(config.labelKey)` 渲染；同一 feature 内只采用一种方式，避免混用。
  - **新增此类常量时**：同步在 `src/i18n/static-keys.ts` 中登记对应 key，或确保文案以 `t('...')` 字面量形式出现以便扫描，避免遗漏翻译。

### 3.2 代码风格与类型

- **表达式**：禁止 2 层及以上嵌套三元表达式；改用 `if-else`、提前返回或抽取函数。单层三元可保留，但需简洁。
- **可读性**：控制函数圈复杂度，复杂逻辑拆成小函数；变量与函数命名需有意义，遵循驼峰等常规约定。
- **TypeScript**：避免 `any`，优先具体类型或 `unknown`；为参数与返回值显式标注类型；仅类型用途的导入使用 `import type { X } from '...'`。
- **类型检查**：每次改动 TypeScript 或 TSX 代码后都要执行 `bun run typecheck`（tsgo）；若出现类型错误，须修复至无错误为止，不得遗留。
- **Lint 检查**：每次完成代码改动前，必须对所涉及文件执行 lint 检查（`bun run lint`），并修复这些文件中的所有 lint error；不得遗留 error。warning 可按变更范围与风险评估处理。
- **解构**：对象非必要不要进行解构，特别是组件的 props；直接使用 `props.xxx` 更清晰，避免不必要的解构增加代码复杂度。

### 3.3 组件

- 使用函数式组件与 Hooks，单一职责；组件 props 须有明确类型（接口或类型别名）。
- **Props 使用**：组件 props 非必要不要解构，直接使用 `props.xxx` 访问属性，保持代码清晰（详见 [3.2 代码风格与类型](#32-代码风格与类型)）。
- 单文件超过约 200 行时考虑拆分子组件或将逻辑抽到自定义 Hooks；类型定义可与组件同文件或放在同模块的 `types` 中。

### 3.4 性能

- **React**：合理使用 `useMemo`、`useCallback` 减少无效重渲染；避免在渲染路径中创建新对象/数组；必要时使用 `React.memo`。
- **代码分割**：使用 `React.lazy` 与动态 `import` 做按需加载，控制首屏与路由体积；TanStack Router 在生产模式自动按路由 code splitting（见 `rsbuild.config.ts`）。
- **资源**：图片选用合适格式与尺寸，大列表考虑虚拟滚动（如 @tanstack/react-virtual），大量图片考虑懒加载。

### 3.5 状态管理

- 使用 Zustand 的 `create` 定义 store，并为 state 与 actions 定义清晰类型。Store 按功能放在 `src/stores/`（`auth-store` / `notification-store` / `system-config-store`）。
- 组件内优先用选择器订阅，避免整 store 订阅导致多余渲染，例如：`const user = useAuthStore((s) => s.auth.user)`。
- 需持久化的状态在 store 内读写，主题持久化用 cookie（`vite-ui-theme`，1 年），与 `next-themes` cookieStorage 一致；其余偏好可放 localStorage。

### 3.6 API 请求

- **后端契约**：metapi-go 后端默认 `http://localhost:4000`（`PORT` env 可覆盖）。admin REST 在 `/api`（`handler/admin` ~144 endpoints），OpenAI 兼容代理在 `/v1`（`handler/proxy` ~30 routes）。JSON 字段一律 camelCase（API 兼容 Golden Rule），env var 名与 TS 版一致无前缀。
- **Axios**：使用项目统一的 `api` 实例（`src/lib/http-client.ts`），`withCredentials: true`；GET 默认请求去重，特殊请求可通过配置关闭。认证与通用错误在拦截器中处理（401 静默刷新）。
- **React Query**：数据获取用 `useQuery`，变更用 `useMutation`；为每个查询配置唯一 `queryKey`（数组形式、层级一致）；在 `onSuccess` 中对相关 query 做 `invalidateQueries`，可配合乐观更新。服务端错误统一通过 `handleServerError` 处理（详见 [3.9 错误处理](#39-错误处理)）。
- **Dev proxy**：`rsbuild.config.ts` 将 `/api`、`/v1` 代理到后端；`DEV_PROXY_TARGET` / `VITE_DEV_PROXY_TARGET` / `PORT` / `VITE_BACKEND_PORT` 可覆盖默认 4000。

### 3.7 表单

- 使用 React Hook Form + Zod：在 feature 的 `lib/` 下定义 schema，并用 `z.infer` 导出表单类型；`useForm` 配合 `@hookform/resolvers/zod` 做校验。
- 提交逻辑放在 `onSubmit`，展示加载与错误状态；成功后视场景重置表单或关闭弹窗。服务端校验错误映射到对应字段并展示（字段级错误展示方式见 [3.9 错误处理](#39-错误处理)）。

### 3.8 路由

- 使用 TanStack Router，路由文件位于 `src/routes/`，通过 `createFileRoute` 定义；搜索参数用 Zod schema + `validateSearch` 校验。
- 在 `beforeLoad` 中做认证与重定向，避免不必要的请求；嵌套结构用布局路由与 `_authenticated` 等前缀，子路由通过 `<Outlet />` 渲染。
- 导航使用 `useNavigate` 或 `Link`，保持类型安全，避免直接操作 `window.location`。

### 3.9 错误处理

- **服务端错误**：统一使用 `handleServerError`（`src/lib/server-error.ts`），在 React Query 全局配置与拦截器中接入；按 HTTP 状态码给出合适提示，文案使用 i18n。
- **展示**：使用 `toast.error` 等统一方式；路由级错误由 `errorComponent` 承接，提供友好错误页并记录便于排查的信息。
- **表单**：校验与服务端错误映射到字段后，在字段下方展示；使用 `form.setError` 等与表单库一致的方式。

### 3.10 样式

- 以 Tailwind 工具类为主，动态类名用 `cn()` 合并；非动态场景避免内联样式。
- 响应式采用移动优先与 Tailwind 断点（`sm:`、`md:`、`lg:` 等）；主题与暗色用 CSS 变量与 `dark:`，自定义样式集中在 `src/styles/`（`theme.css` OKLCH 语义 token + `theme-presets.css` 10 套预设 + `index.css` Tailwind 入口），组件内尽量少写自定义 CSS。
- 暗色模式 class-based（`<html class="dark">`），`@custom-variant dark (&:is(.dark *))`；3 轴主题（preset/radius/scale）经 `<body data-theme-*>` 切换。

### 3.11 文件组织

- **功能模块**：置于 `src/features/<feature>/`，内含 `components/`、`lib/`、`hooks/`，以及按需的 `api.ts`、`types.ts`、`constants.ts`、入口组件。Feature 划分：
  `auth`（Login + oauth）、`dashboard`（仪表盘 + chart）、`sites`（站点管理）、`accounts`（连接管理 + Tokens）、`token-routes`（路由 + dnd-kit 拖拽）、`oauth`（OAuth 管理）、`checkin`（签到记录）、`proxy-logs`（使用日志）、`monitors`（可用性监控）、`settings`（含 downstream-keys/events/import-export/notify 子区，drill-in 工作区）、`models`（模型广场）、`model-tester`（操练场）、`site-announcements`（站点公告）、`about`（关于）。
- **通用**：通用组件放 `src/components/`（`ui/` shadcn 57 组件 + `data-table/` 四层架构 + `layout/`），通用工具与类型放 `src/lib/`；组件文件 PascalCase，工具/类型文件 kebab-case 或 `types.ts`，类型使用 PascalCase 命名并 `export type`。

### 3.12 可访问性

- 使用语义化 HTML（如 `header`、`nav`、`main`、`footer`），表单用 `label` 关联输入。
- 保证键盘可操作与焦点顺序合理；必要时使用 ARIA（如 `aria-label`、`aria-expanded`、`aria-hidden`）；装饰性图标加 `aria-hidden="true"`，重要信息提供文本等价。Base UI 组件自带 focus trap/Escape/ARIA，优先用 shadcn 封装。
- 对比度满足 WCAG 2.1 AA（正文至少 4.5:1）。

### 3.13 安全

- 认证与权限在路由（`beforeLoad`）与接口层校验；敏感操作增加二次确认等。
- 前后端均做数据校验（如 Zod），不信任仅前端校验；敏感信息不落前端存储，配置用环境变量，禁止硬编码密钥。
- 依赖 React 默认转义，慎用 `dangerouslySetInnerHTML`（公告/markdown 渲染须经 `dompurify`）；跨域与 Cookie 使用 `withCredentials` 并按后端要求处理 CSRF。

### 3.14 测试

- 工具函数与纯逻辑优先单元测试（Vitest + `happy-dom`），测试文件 `*.test.ts`；组件用 React Testing Library 测交互与行为，避免测实现细节。
- 功能模块或组件模块的测试必须放在该模块专属的 `__tests__/` 目录中，例如 `src/features/token-routes/components/__tests__/layout.test.ts`；禁止将新增测试文件与正式代码文件平铺在同一目录。
- 测试文件按被测职责命名（`layout.test.ts`、`validation.test.ts`）；一个测试文件只覆盖一个明确模块或职责。每用例只保护一个可描述的行为，名称含触发条件与预期结果，优先 Arrange/Act/Assert。
- 必须覆盖主要成功路径以及关键边界和失败路径（空数据、单项/多项、超长文本、无效输入、禁用状态、异步失败与降级）；不得为数量机械枚举不相关输入。
- 布局测试断言明确稳定的行为契约（固定尺寸、排列方向、溢出策略、降级路径），不要仅断言组件能渲染，也不依赖浏览器像素误差或脆弱 class 快照。
- 组件交互测试从用户视角查询元素并执行点击/输入/键盘/焦点操作，断言可见结果或可访问状态；禁止直接断言组件内部 state、私有函数调用次数或无用户意义的 DOM 层级。
- 异步测试必须等待明确的界面状态或 Promise 结果，不得使用固定 `sleep`；定时器、网络请求、浏览器 API 仅在必要边界可控 mock，并在每用例后恢复。
- 优先测试真实代码路径；只有外部网络、时间、随机数、存储或浏览器 API 等不可控边界可以 mock。禁止 mock 被测模块自身。
- 提交前必须至少运行受影响测试文件，并执行 `bun run typecheck` 和涉及文件的 lint；不得在未看到最新通过结果的情况下声明测试完成。

### 3.15 依赖管理

- 使用 **Bun**：`bun install`、`bun add <pkg>`、`bun add -d <pkg>`、`bun remove <pkg>`、`bun pm ls`、`bun update` 等。
- 新增依赖前评估维护情况、体积与许可；生产与开发依赖区分清楚，版本用 `^`/`~` 控制，定期更新以获取安全修复。`overrides` 字段的安全/版本钉桩不得擅自移除。

### 3.16 构建与部署

- 使用 Rsbuild，配置见 `rsbuild.config.ts`；脚本以 `package.json` 为准（`bun run dev`、`bun run build`、`bun run build:check`、`bun run typecheck`、`bun run lint`、`bun run format`、`bun run i18n:sync`、`bun run knip`）。
- **单二进制嵌入**：构建产物落入 `web/dist/`，经 `web/embed.go` 的 `go:embed dist` 打包进 Go 二进制；生产镜像不含 node/bun。`desktop:icons` 用 `node`（sharp native addon 需独立 node 运行），`build:web = desktop:icons && rsbuild build`，`build = build:web`。
- 代码分割与懒加载策略见 [3.4 性能](#34-性能)；环境变量用 `.env` 且以 `VITE_` 前缀，不在代码中硬编码。
- **发布前**：执行 `bun run build:check`（tsgo + 完整构建）、`bun run lint`、`bun run format:check`，检查产物体积与环境变量配置。

---

## 四、协作与提交

- 提交信息清晰、符合项目约定（`docs:`/`state:`/`ops:`/`chore:` 前缀），描述变更内容与原因，中英文统一即可。
- 变更需经过代码审查，符合本文档规范，并关注质量、性能、安全。
- 重大功能或规范变更时更新相关文档与本文件。本文件行数预算 ≤300 行（metapi-go AGENTS.md 既有约束），超限先外置到 `src/docs/`。

---

## 更新日志

- **2026-08-11**：初始版本——100% 对齐 newapi 栈重写（国际化、代码、组件、类型、状态、API、表单、路由、错误、样式、文件组织、可访问性、安全、测试、依赖、构建部署 16 节）。
