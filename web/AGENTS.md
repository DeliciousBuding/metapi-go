最后更新：2026-08-18 14:00

# 前端开发规范

本文档定义 metapi-go 前端项目的开发规范与最佳实践，供开发与 AI 助手共同遵循。具体依赖与脚本以 `package.json` 为准。

metapi-go 是 Meta-layer management and unified proxy for AI API aggregation platforms 的 Go 重写，前端为 React SPA，预构建产物经 `web/embed.go` 的 `go:embed dist` 打包进 Go 单二进制（生产镜像不含 node/bun）。前端采用与 newapi 同类的 React 技术栈；迁移必须保留后端 API 契约（camelCase 字段、env var 名）与 DB（SQLite/PG dual dialect）。

---

## 一、项目概览

### 技术栈

| 类别       | 技术                                                                                                       |
| ---------- | ---------------------------------------------------------------------------------------------------------- |
| 包管理     | Bun                                                                                                        |
| 构建       | Rsbuild 2、tsgo（TS 原生编译器）                                                                           |
| 框架       | React 19、TypeScript                                                                                       |
| 路由       | @tanstack/react-router（文件路由 + 自动生成 + validateSearch + loader + 生产自动拆包）                     |
| 数据与请求 | @tanstack/react-query、axios、Zustand                                                                      |
| 表格与列表 | @tanstack/react-table                                                                                      |
| 国际化     | i18next、react-i18next、i18next-browser-languagedetector（key-based）                                      |
| UI 与样式  | Base UI、shadcn/ui base-nova、HugeIcons（免费层）+ lucide、Tailwind CSS 4、clsx / class-variance-authority |
| 字体       | @fontsource-variable/public-sans + lora（本地嵌入）                                                        |
| 图表       | Recharts + shadcn chart                                                                                    |
| 表单       | React Hook Form、Zod                                                                                       |
| 动画/反馈  | tw-animate-css、sonner、cmdk                                                                               |
| 工具       | oxfmt、oxlint、knip、vitest                                                                                |

优先选用成熟、维护良好的开源库；仅在现有库无法满足或需特殊适配时自行实现，并评估可维护性与通用性。HugeIcons 仅用免费层（`@hugeicons/core-free-icons`），付费图标用 lucide 兜底。

---

## 二、目录

- [一、项目概览](#一项目概览)
- [二、目录](#二目录)
- [三、目录结构](#三目录结构)
- [四、开发工作流与静态门禁](#四开发工作流与静态门禁)
- [五、代码约定](#五代码约定)
  - [5.1 国际化](#51-国际化)
  - [5.2 代码风格与类型](#52-代码风格与类型)
  - [5.3 组件](#53-组件)
  - [5.4 性能](#54-性能)
  - [5.5 状态管理](#55-状态管理)
  - [5.6 API 请求](#56-api-请求)
  - [5.7 表单](#57-表单)
  - [5.8 路由](#58-路由)
  - [5.9 错误处理](#59-错误处理)
  - [5.10 样式与设计系统](#510-样式与设计系统)
  - [5.11 文件组织](#511-文件组织)
  - [5.12 可访问性](#512-可访问性)
  - [5.13 安全](#513-安全)
  - [5.14 测试](#514-测试)
  - [5.15 依赖管理](#515-依赖管理)
  - [5.16 构建与部署](#516-构建与部署)
- [六、隐私与迁移约束](#六隐私与迁移约束)
- [七、协作与提交](#七协作与提交)

---

## 三、目录结构

```
src/
├── features/              # 按产品领域组织的功能模块
├── components/
│   ├── data-table/        # 四层架构表格（core/layout/toolbar/static/hooks）
│   ├── ui/                # shadcn/Base UI 设计系统原语
│   └── layout/            # 页面级布局（含 components/ config/ lib/ types.ts）
├── lib/                   # api / http-client / auth-session / cookies /
│                          # identity-branding / theme-customization / utils + helpers/
├── routes/                # TanStack Router 文件路由
├── styles/                # theme.css + theme-presets.css + index.css
├── i18n/                  # config.ts + languages.ts + locales/{en,zh-CN}.json
├── hooks/                 # use-media-query / use-mobile / use-sidebar-data / use-sidebar-view
├── context/               # theme / theme-customization / font / direction providers
├── stores/                # auth-store
├── config/                # fonts.ts
├── assets/                # 静态资源
└── main.tsx               # 入口
```

### features

| feature                                      | 职责                                          |
| :------------------------------------------- | :-------------------------------------------- |
| `auth` / `oauth`                             | 管理登录与上游 OAuth 连接                     |
| `dashboard` / `observability`                | 仪表盘、健康状态、RealtimeOps、代理日志工作台 |
| `sites` / `accounts` / `channels` / `import` | 上游站点、连接、通道与导入动线                |
| `token-routes`                               | 下游 token 路由与策略配置                     |
| `models` / `model-tester`                    | 模型、价格/修复候选与四协议流式测试台         |
| `checkin` / `proxy-logs`                     | 签到记录与使用日志                            |
| `settings` / `about`                         | 设置中心与项目信息                            |

### 分层边界

- `src/components/` 禁止 import `src/features/` 或 `src/routes/`；`src/lib/`
  禁止 import `src/features/` 或 `src/routes/`。分层规则、例外与退出条件见
  `docs/internal/web-package-boundaries.md`；`bun run lint` 会通过
  `scripts/check-boundaries.mjs` 自动执行该门禁。

### data-table 四层

- `core/`：TanStack table 渲染原语、header/row/pagination、loading/empty、pinned-column。
- `layout/`：响应式页面组合（toolbar + desktop table + mobile list + bulk actions）。
- `toolbar/`：filter/search/view-option/selection action。
- `static/`：本地/静态数组的轻量渲染（不依赖 TanStack state）。
- `hooks/`：`useDataTable`（受控状态层，三段式 URL 同步）+ `use-debounce`。

公共 API 经 `index.ts` 导出，feature 一律 `import from '@/components/data-table'`。feature 专属列/动作/对话框留在各 feature 目录。

### lib

- `api/`、`http-client.ts`（统一 axios 实例 + 拦截器）、`auth-session.ts`（token/过期处理）、`cookies.ts`、`identity-branding.ts`、`theme-customization.ts`、`utils.ts`（含 `cn()`）。
- `helpers/`：纯函数工具集，测试统一放 `helpers/__tests__/`（如 `searchParams`、`routeMissingTokenHints`、`zeroChannelRoutes`）。

### routes

TanStack Router 文件路由：`__root.tsx` + `sign-in.tsx` + `_authenticated/`（布局路由 + `beforeLoad` 认证）。子路由经 `<Outlet />` 渲染；`dashboard/$section.tsx`、`settings/$subarea.*.tsx` 为参数化嵌套路由。生产模式按路由 code splitting。

---

## 四、开发工作流与静态门禁

```bash
bun install            # 装依赖
bun run dev            # 本地开发（rsbuild dev，/api /v1 代理到后端）
bun run typecheck      # tsgo -b，TS 类型检查
bun run lint           # oxlint，lint 检查
bun run lint:fix       # oxlint --fix
bun run test           # 路由树校验 + vitest run（全量）
bun run test:watch     # vitest watch
bun run knip           # 未使用代码检测
bun run build          # = build:web = desktop:icons && rsbuild build
bun run build:check    # tsgo -b && build（发布前完整检查）
bun run format         # oxfmt（含保护头）
bun run format:check   # 格式检查
bun run desktop:icons  # 生成桌面图标（sharp native，需 node）
```

**静态门禁（发布前必须全绿）**：tsgo 0 error + oxlint 0 error + knip exit 0 + `bun run build` pass + vitest 全绿。测试数量不写入规则文件，以 `bun run test` 的最新输出为准。Dev proxy 默认指向 `http://localhost:4000`，可经 `DEV_PROXY_TARGET` / `VITE_DEV_PROXY_TARGET` / `PORT` / `VITE_BACKEND_PORT` 覆盖。

---

## 五、代码约定

### 5.1 国际化

- **页面文本**：所有面向用户文案需 i18n，使用 `useTranslation()` 的 `t()` 翻译。当前支持 `en` + `zh-CN`，key 双向一致性由 `src/i18n/__tests__/i18n-keys.test.ts` 校验；不在规则中固化易漂移计数。
- **语言切换**：顶栏 `LanguageSwitcher`（`components/layout/components/app-header.tsx`）提供 en/zh-CN 手动切换；`i18next-browser-languagedetector` 按 localStorage → navigator 顺序自动跟随浏览器语言；`languageChanged` 时经 `toBcp47()` 同步 `document.documentElement.lang`（zhCN → `zh-CN`）+ `dir`。
- **使用场景**
  - **React 组件**：必须 `const { t } = useTranslation()`，保证语言切换时重渲染。
  - **非 React 环境**（工具函数、常量、类方法）：可 `import { t } from 'i18next'`；不随语言切换自动更新，仅在不依赖响应式更新场景使用。
  - 即使父组件已用 `useTranslation()`，子组件仍应自行使用以保证独立性。
- **专有名词**：品牌、产品、技术术语可保留英文（API、React、TypeScript）；有约定俗成译法则用翻译。
- **翻译键**：用有层级、语义清晰的键名（如 `dashboard.overview.title`），保持命名一致。

- **枚举与文案（常量中的 i18n）**
  各 feature `constants.ts` 常出现「枚举/状态 + 展示文案」或「成功/错误消息」，须统一约定以免遗漏 i18n、用法混乱：
  - **成功/错误/提示类消息**（`SUCCESS_MESSAGES`、`ERROR_MESSAGES`）：常量值仅表示 **i18n 键**（与英文 fallback 同字面量）。展示时**必须**经 `t()` 使用，如 `toast.success(t(SUCCESS_MESSAGES.ACCOUNT_CREATED))`，**禁止**直接当最终文案。
  - **状态/选项 label**：常量中统一用 **labelKey**（字符串，即 i18n 键），组件 `t(config.labelKey)` 渲染；同一 feature 内只用一种方式，避免混用。
  - 新增此类常量时，确保文案以 `t('...')` 字面量形式出现以便扫描，避免遗漏翻译。

### 5.2 代码风格与类型

- **表达式**：禁止 2 层及以上嵌套三元；改用 `if-else`、提前返回或抽取函数。单层三元可保留但需简洁。
- **可读性**：控制函数圈复杂度，复杂逻辑拆成小函数；命名需有意义，遵循驼峰约定。
- **TypeScript**：避免 `any`，优先具体类型或 `unknown`；为参数与返回值显式标注类型；仅类型用途导入用 `import type { X }`。
- **类型检查**：每次改动 TS/TSX 后执行 `bun run typecheck`（tsgo）；类型错误须修复至无错误，不得遗留。
- **Lint**：完成代码改动前对涉及文件执行 `bun run lint`，修复所有 error；warning 按变更范围与风险评估处理。
- **解构**：对象非必要不解构，特别是组件 props；直接 `props.xxx` 更清晰。
- **简单优先**：优先在现有 owner 内直接实现；只有真实边界或多个现有消费者才新增抽象。不得为尚不存在的接口、角色或产品能力保留 scaffold、双轨或 fallback ladder。

### 5.3 组件

- 函数式组件 + Hooks，单一职责；props 须有明确类型（接口或类型别名）。
- **Props 使用**：非必要不解构，直接 `props.xxx` 访问（见 [5.2](#52-代码风格与类型)）。
- 单文件超 ~200 行考虑拆子组件或抽自定义 Hook；类型定义可同文件或同模块 `types` 中。

### 5.4 性能

- **React**：合理用 `useMemo`/`useCallback` 减少无效重渲染；避免渲染路径中创建新对象/数组；必要时 `React.memo`。
- **代码分割**：TanStack Router 生产模式自动按路由 code splitting（见 `rsbuild.config.ts`，`autoCodeSplitting: isProd`）；不要再叠一套手工路由拆包机制。
- **资源**：图片选合适格式与尺寸并按需懒加载；只有真实性能证据支持时才引入虚拟滚动等额外复杂度。

### 5.5 状态管理

- 服务端状态用 TanStack Query（`useQuery`/`useMutation`）；客户端状态用 Zustand。
- Zustand store 按 `src/stores/` 放置（当前仅 `auth-store`）。组件内优先用选择器订阅，避免整 store 订阅：`const user = useAuthStore((s) => s.auth.user)`。
- 持久化：主题用 cookie（`vite-ui-theme`，1 年，与 `next-themes` cookieStorage 一致）；其余偏好可放 localStorage。

### 5.6 API 请求

- **后端契约**：后端默认 `http://localhost:4000`（`PORT` env 可覆盖）。admin REST 在 `/api`（`handler/admin`，端点清单见 `docs/api.md`），OpenAI 兼容代理在 `/v1`（`handler/proxy`）。JSON 字段一律 camelCase，env var 名与 TS 版一致无前缀。
- **Axios**：用统一 `api` 实例（`src/lib/http-client.ts`），`withCredentials: true`；GET 默认请求去重，特殊请求可配置关闭。后端没有 refresh endpoint，401 按现有契约清理会话并回到登录页，不保留伪刷新流程。
- **React Query**：`useQuery` 取数、`useMutation` 变更；每个查询配唯一 `queryKey`（数组形式、层级一致）；`onSuccess` 对相关 query `invalidateQueries`。仅在竞态收益明确且可回滚时做乐观更新。
- **Dev proxy**：代理表在 `config/build-shared.ts`（`createDevProxy`，rsbuild dev 消费），将 `/api`、`/v1` 代理到后端；`DEV_PROXY_TARGET` / `VITE_DEV_PROXY_TARGET` / `PORT` / `VITE_BACKEND_PORT` 可覆盖默认 4000。

### 5.7 表单

- RHF + Zod：feature 的 `lib/` 下定义 schema，`z.infer` 导出表单类型；`useForm` 配 `@hookform/resolvers/zod` 校验。
- 表单一律经 `ui/form.tsx` 的 `<Form {...form}>` 包裹：内置 `FormValidationFocus` 在校验失败提交后自动滚动并聚焦首个 `aria-invalid` 字段（focus-first-invalid），不得绕过 `<Form>` 或逐表单手写聚焦逻辑；字段须经 `FormField`/`FormControl` 渲染以获得 `aria-invalid` 联动。
- 提交逻辑放 `onSubmit`，展示加载与错误状态；成功后视场景重置或关闭弹窗。服务端校验错误映射到对应字段（见 [5.9](#59-错误处理)）。

### 5.8 路由

- TanStack Router，路由文件在 `src/routes/`，`createFileRoute` 定义；搜索参数用 Zod schema + `validateSearch` 校验。
- `beforeLoad` 做认证与重定向，避免不必要请求；嵌套结构用布局路由与 `_authenticated` 前缀，子路由经 `<Outlet />` 渲染。
- 用 `loader` 做预取（prefetch）；生产拆包由 `rsbuild.config.ts` 的 `autoCodeSplitting` 统一负责。
- 导航用 `useNavigate` 或 `Link`，保持类型安全，避免直接操作 `window.location`。

### 5.9 错误处理

- **服务端错误**：Axios 拦截器处理通用 HTTP/业务错误；feature mutation 只处理需要领域上下文的提示，避免双 toast。
- **展示**：`toast.error` 等统一方式；路由级错误由 router `defaultErrorComponent` 承接。
- **表单**：校验与服务端错误映射到字段后字段下方展示，用 `form.setError` 等与表单库一致方式。

### 5.10 样式与设计系统

设计系统 SSOT 是 `docs/internal/design/DESIGN.md`（视觉语言、OKLCH token、5 轴主题 preset/font/radius/scale/content-layout、图表取色 `--chart-1…5`、玻璃材料、焦点环、间距/圆角/字型）；组件归属见 `docs/internal/design/components.md`；可访问性见 `docs/internal/design/a11y-checklist.md`。本节只列前端落地约定：

- Tailwind 工具类为主，动态类名用 `cn()` 合并；非动态场景避免内联样式。响应式用移动优先与 Tailwind 断点（`sm:`/`md:`/`lg:`）。
- 三层 CSS 在 `src/styles/`：`theme.css`（OKLCH 语义 token）/ `theme-presets.css`（10 套预设覆盖）/ `index.css`（Tailwind 4 入口 + `@theme inline` 桥接）。新增 token 改这里，不在页面硬编码 hex。
- 组件内尽量少写自定义 CSS；新增 UI 先从 shadcn Base UI 原语（`@/components/ui/*`）出发。
- 图标分层：`components/ui/*` 设计系统原语统一用 HugeIcons 免费层（`@hugeicons/core-free-icons` + `HugeiconsIcon`，`strokeWidth={2}`）；`features/`、`layout/`、`data-table/` 业务层沿用 lucide-react；免费层缺 glyph 时用 lucide 兜底；禁止新增付费 HugeIcons；同一文件不得混用两套图标来源。

### 5.11 文件组织

- **功能模块**：`src/features/<feature>/`，含 `components/`、`lib/`、`hooks/`，及按需 `api.ts`、`types.ts`、`constants.ts`、入口组件；按领域归属，不按页面数量制造模块。
- **通用**：通用组件放 `src/components/`（`ui/` 设计系统原语 + `data-table/` + `layout/`），通用工具与类型放 `src/lib/`。组件文件 PascalCase，工具/类型文件 kebab-case 或 `types.ts`，类型 PascalCase 并 `export type`。

### 5.12 可访问性

可访问性 SSOT 是 `docs/internal/design/a11y-checklist.md`（键盘焦点与陷阱、accessible name 清单、对比度实测表、响应式 375/768/1280、已知残差）。本节只列前端落地约定：

- 语义化 HTML（`header`/`nav`/`main`/`footer`），表单用 `label` 关联输入。
- icon-only 控件必 `aria-label`；装饰图标 `aria-hidden="true"`；状态不只靠颜色（配文字/徽章）。
- 优先用 shadcn 封装（自带 focus trap/Escape/ARIA），不重复造轮子。

### 5.13 安全

- 认证与权限在路由（`beforeLoad`）与接口层校验；敏感操作二次确认。
- 前后端均做数据校验（Zod），不信任仅前端校验；敏感信息不落前端存储，配置用环境变量，禁止硬编码密钥。
- 依赖 React 默认转义；业务数据不得进入 `dangerouslySetInnerHTML`。跨域与 Cookie 用 `withCredentials` 并按后端要求处理 CSRF。

### 5.14 测试

- **栈**：vitest + @testing-library/react + jsdom（环境见 `vitest.config.ts`）；是否全绿以本次运行输出为准。`bun run test` 先跑 `scripts/verify-route-tree.mjs` 路由树同步校验，再执行 vitest。
- **范围**：工具函数与纯逻辑优先单元测试（`*.test.ts`）；组件用 React Testing Library 测交互与行为，避免测实现细节。
- **位置**：测试必须放模块专属 `__tests__/`（如 `src/features/token-routes/components/__tests__/layout.test.ts`）；禁止与正式代码平铺。
- **命名与组织**：按被测职责命名（`layout.test.ts`、`validation.test.ts`）；一个文件只覆盖一个明确模块；每用例只保护一个可描述行为，名称含触发条件与预期结果，优先 Arrange/Act/Assert。
- **覆盖**：必覆盖主要成功路径 + 关键边界/失败路径（空数据、单项/多项、超长文本、无效输入、禁用状态、异步失败与降级）；不为数量机械枚举不相关输入。
- **断言**：布局测试断言稳定行为契约（固定尺寸、排列方向、溢出策略、降级路径），不依赖像素误差或脆弱 class 快照；交互测试从用户视角查询并操作，断言可见结果或可访问状态，禁止断言内部 state/私有函数调用次数/无用户意义 DOM 层级。
- **异步**：等待明确界面状态或 Promise 结果，禁止固定 `sleep`；定时器/网络请求/浏览器 API 仅在必要边界可控 mock，每用例后恢复。优先测真实代码路径，只 mock 不可控边界（网络/时间/随机数/存储/浏览器 API），禁止 mock 被测模块自身。
- 提交前至少运行受影响测试文件 + `bun run typecheck` + 涉及文件 lint；未看到最新通过结果不得声明测试完成。

### 5.15 依赖管理

- 用 **Bun**：`bun install`、`bun add <pkg>`、`bun add -d <pkg>`、`bun remove <pkg>`、`bun pm ls`、`bun update`。
- 新增依赖前评估维护情况、体积与许可；生产/开发依赖区分，版本用 `^`/`~` 控制，定期更新获安全修复。`overrides` 字段的安全/版本钉桩不得擅自移除。

### 5.16 构建与部署

- 用 Rsbuild，配置见 `rsbuild.config.ts`；脚本以 `package.json` 为准。构建期共享常量（dev proxy、`METAPI_WEB_VERSION` define、`@` alias）唯一收口在 `config/build-shared.ts`，由 `rsbuild.config.ts` 与 `vitest.config.ts` 共同消费；路由生成唯一入口是 rsbuild 配置里的 `@tanstack/router-plugin`，产物 `src/routeTree.gen.ts` 随仓库提交。
- **单二进制嵌入**：构建产物落入 `web/dist/`，经 `web/embed.go` 的 `go:embed dist` 打包进 Go 二进制；生产镜像不含 node/bun。`desktop:icons` 用 `node`（sharp native addon 需独立 node），`build:web = desktop:icons && rsbuild build`，`build = build:web`。
- 代码分割与懒加载见 [5.4](#54-性能)；环境变量用 `.env` 且以 `VITE_` 前缀，不在代码中硬编码。
- **发布前**：执行 `bun run build:check`（tsgo + 完整构建）、`bun run lint`、`bun run format:check`，检查产物体积与环境变量配置。

---

## 六、隐私与迁移约束

- metapi-go 是**公开仓**（MIT）。本文档与所有公开材料不写内部部署路径、主机名、密钥、内部文档引用；运维事实（主机/镜像 tag/端口）不属于本仓，公开部署见仓库根 `docs/deployment.md`。
- **无缝迁移**：从 TS 版 Metapi 迁移无损——后端 + DB（SQLite/PG dual dialect）+ API 契约（camelCase 字段、env var 名）100% 保留；`store.Open(dialect, dsn)` 不假设 SQLite-only 特性。
- 原 TS 参考检出（reference checkout）保持在仓外，不在本仓记录本地检出路径。

---

## 七、协作与提交

- 提交信息清晰、符合项目约定（`docs:`/`state:`/`ops:`/`chore:` 前缀），描述变更内容与原因，中英文统一即可。
- 变更需经代码审查，符合本文档规范，关注质量、性能、安全。
- 重大功能或规范变更时更新相关文档与本文件。本文件行数预算 ≤300 行，超限先外置到 `src/docs/`。
