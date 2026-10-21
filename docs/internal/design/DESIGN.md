# Metapi Design System

**Product**: Metapi admin
**Scope**: Enterprise ops control plane (sites, accounts, tokens, routes, monitors, logs)
**Visual language**: GCP cloud console density + frosted glass shell + Apple detail
**Source of truth**: this document + `web/src/styles/theme.css` + `web/src/styles/theme-presets.css` + `web/src/lib/theme-customization.ts` + `web/src/components/ui/**`
**Last updated**: 2026-10-19

---

## 1. Brand

| Attribute       | Decision                                                                                                                                                                                                                                                                                                              |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Product voice   | Professional, dense, high-signal ops UI                                                                                                                                                                                                                                                                               |
| Audience        | Operators managing multi-site API gateways, keys, and routing                                                                                                                                                                                                                                                         |
| Personality     | Calm GCP control room with Apple-grade materials — not consumer marketing                                                                                                                                                                                                                                             |
| Density default | Comfortable-dense (admin tables + KPI cards coexist); density axis `data-theme-scale`                                                                                                                                                                                                                                 |
| Brand color     | **Indigo family** — light primary `oklch(0.565 0.19 262)` (white text), dark primary `oklch(0.71 0.155 262)` (dark ink); every constrained lightness is solved against its WCAG floor (see §2.3; exact values live in `theme.css`, never hard-code hex)                                                                                                                                          |
| Logo mark       | Transparent solid-color badge `web/public/logo.svg` — rounded-square `#3b5bdb` field with white **π** glyph (real U+03C0, serif fallback, not hand-drawn strokes); `favicon.svg` = standalone solid blue π for small sizes; both served from the embedded SPA root (`router.go` root-file whitelist, `image/svg+xml`) |
| Fonts           | **Inter + native CJK**, optional **Lora** (locally embedded via `@fontsource-variable`) — no Google Fonts CDN                                                                                                                                                                                                                            |
| High-res        | Content layout axis `data-theme-content-layout` (`full`/`centered`); full remains fluid until 1920px, then caps the content scan width at 1920px; centered clamps to `--max-content-width` (1280px) at ≥1280px. The sidebar and header remain viewport-wide. Utilities `max-w-container` (1280px) / `max-w-container-lg` (1536px) serve individual pages. |

**Primary workspace**: desktop administration. Judge hierarchy, density and
multi-column composition at 1440–1920px first. Mobile adapts the same tasks
without letting narrow-screen compromises dictate the desktop layout.
Metric cards keep title, primary value, supporting text and detail cells in
separate visual tiers; long supporting text must not compete beside the number.
Desktop table rows start at 48px and grow for multiline content; narrow layouts
retain their existing touch spacing. Desktop lists with four or fewer loaded rows
hug their content rather than stretching a mostly empty bordered table; longer
lists retain the sticky header. The [operator overview](dashboard-operations.md)
prioritizes request outcomes, actionable risks and upstream/model usage; resource
counts and maintenance are secondary. Metric details use compact divided cells
rather than nested padded cards. Increase information density by removing repeated spacing,
not by shrinking body text.

Proxy logs show attempt timing, input/output token counts, estimated USD cost,
and stream mode in the default view. API keys are not labeled as token usage.
Account, client, key and retry columns remain optional; saved column preferences
are preserved. Missing usage, cost and mode stay unknown rather than becoming
zero or non-stream. Billing details present billable input, output, cache read
and cache creation with their individual costs; the original billing object is
available in a disclosure, including legacy objects without that breakdown.

The model marketplace shows supply separately from account counts. Its detail
sheet identifies site/OAuth accounts and direct grants, with connection names,
upstream model names, and exact public route bindings. Configuration switches,
manual selection, availability records, and cumulative request outcomes retain
their own labels. None is rendered as a general healthy/verified verdict.
Missing supply data from older servers stays unknown; an empty list is explicit.
See [the marketplace contract](../../api/models.md) for counting and scope.

The marketplace list is server-driven: search, filters, sort, totals, and
facet counts are evaluated over the whole marketplace, and the table renders
exactly the page it was given. Facet options and counts arrive with the page
response and are disjunctive per dimension, so selecting a filter value never
hides the alternatives. Brand stays a display-only column (the name-to-brand
mapping is a client-side visual identity); the search box covers that intent.
A `?model=<name>` deep link resolves through an exact-name detail lookup, so
the referenced model opens even when it is not on the loaded page; an
unresolvable name produces an explicit notice, never a silent drop.

Route details separate a saved configuration snapshot from an explicit current
check. The check asks for one concrete client model and client API/protocol,
then labels the result as an administrator preview. It names typed account,
direct-group, and OAuth-pool identities, shows candidate-filter blockers, and
lists unevaluated dimensions. It never presents a candidate as the selected
winner, infers an identity from a negative compatibility ID, or renders an
uncalculated probability as 0%. A refresh failure clears the prior result so a
stale answer cannot sit under new input. Saved snapshots keep their own saved
time and unknown context labels. The route workspace on /token-routes keeps one URL-owned diagnostic context for a concrete client model, protocol, and optional downstream key; it renders the server's key admission verdict and candidate eligibility side by side, then links the matched route and that key's proxy logs without pretending the preview selected a candidate. Admission blockers also expose a one-shot edit deep link to /downstream-keys; dimensions with an editable form surface focus that control, while IP and rate blockers open the key sheet without inventing an anchor.

Proxy-log details offer the reverse handoff: the structured `downstreamPath`, requested model, and optional downstream key populate a current-diagnosis deep link. Without a recorded key it is an administrator preview. Unknown or unsupported interfaces hide the shortcut; error text never supplies the protocol. The check describes current configuration, not the historical decision.

`/proxy-logs` 默认展示网关请求结果：时间、模型/接口、终态、总耗时和尝试数。
HTTP 状态与终态分开，200 后输出中断不能显示为完成；WS 记录以 turn 为单位，
不拿握手状态表示完成。一个 URL-owned 详情抽屉展示内部 ID、实际响应信息、
首输出与响应头 TTFB、按序尝试和当前路由诊断。零派发不构造尝试；缺失关联、
未知用量与费用保持未知，失败尝试也保留其已记录用量。

尝试记录保留独立入口及旧筛选深链，详情仍显示 `lastAttempt`，不读取旧
`terminal` 字段。未关联的历史记录不推断请求终态。概览使用独立网关请求
summary/points 展示已记录请求完成率及覆盖起点；旧服务器只提供末次尝试时，
保留明确的“末次尝试”标签，不拼接成请求结果趋势。完成只说明网关观察到
协议处理结束，不代表客户端确认收妥。新管理 API 的非法数据显式报错与重试。

日志页在标题下用图标分段导航并列展示请求结果与尝试记录，统一标题，不以
页头文字链接或常驻横幅代替切换。记录口径通过可点击、键盘可达的说明浮层
查看。跨视图仅保留下游 Key、时间范围及每页条数，清除视图专属筛选与详情；
浏览器返回恢复原视图。自动刷新用显示当前值的下拉菜单，页面仍持有轮询状态。

The root sidebar follows the operator's task order instead of the storage
model: Overview, Upstream, Models & Routes, Access, Requests & Diagnostics, and
Settings. Site, account, check-in, announcement, and OAuth management stay with
supply; channels join routes and models; downstream keys are the access surface;
logs and observability form the diagnosis surface. The grouping changes labels
and order only — every existing URL, deep link, and drill-in workspace remains
stable, so bookmarks and cross-page links keep working.

**Principles**

1. **Signal over decoration** — every color/weight change means status, severity, or hierarchy.
2. **Token-first** — no new hard-coded hex in pages; use OKLCH CSS custom properties.
3. **Dual theme parity** — light and dark share the same semantic token names (`.dark` class on `<html>`).
4. **Dense but breathing** — 14px body text by default, component-owned secondary text, clear table rhythm, and semibold page titles. The compact density axis scales text separately.
   Mobile card labels use `text-xs`, values `text-sm`; detail-sheet labels use `text-xs`. Keep list/header text compact, but show the complete value in the detail layer (wrapping URLs and diagnostic reasons rather than silently ellipsizing them). All sizes follow the density axis.
5. **One glass system** — shell/modal/dropdown only; never blur table rows.
6. **One primitive set** — UI starts from `web/src/components/ui/**` and nothing hand-rolls a parallel control. This used to be a migration in progress; it is now gated — `web/scripts/check-form-control.mjs` classifies every `<FormControl>` site and fails on a native control or on a shape it cannot read.
7. **Console, not marketing** — pill nav, tabular nums, restrained card hover (no lift).
8. **No gradients** — backgrounds, masks, charts, swatches, fallback avatars, and brand assets use solid colors only.

---

### Shared page and task composition

The shared primitives also own how a task is presented; pages should not
rebuild these arrangements with local spacing and raw data editors:

- `components/common/page-header.tsx`: standard list/settings/tool page title,
  description and wrapping actions. Overview pages retain their overview
  hierarchy. Page containers use `page-padding` (16px on small screens, 24px
  from `md`) and continue to own scrolling. Sheet headers reserve space for the
  close button; long detail titles wrap within that space. Model details focus
  the title on opening so the first action below the content does not scroll
  the header out of view.
- `components/data-table/toolbar/toolbar.tsx`: separate query and view/action
  groups, with expanded filters in their own region. The desktop and mobile
  empty states share `core/empty-state.tsx`; page-owned filters participate in
  the same reset action as table filters.
- `features/settings/components/settings-subsection.tsx`: a settings group
  with title, optional description/icon/actions, and flat or panel treatment.
  Notifications and backup tasks use this same composition. Save/reset status
  lives in `settings-form-actions.tsx`, including a polite live announcement
  and responsive action placement.
- `components/common/string-map-editor.tsx`: string mappings use labeled
  key/value rows by default. JSON is an explicit advanced view, retains the
  original text, and remains available for legacy object values that cannot
  be represented as rows. A mode switch must not discard data.

- Backup import starts with file selection, then a human-readable plan and
  confirmation. JSON paste is collapsed by default. Counts use labeled summary
  cells; compatibility failures precede details, and omitted/adapted data
  requires acknowledgement. Raw field names are not primary UI labels.

模型列表的接口与能力使用轻量文字，每列最多显示两项，其余内容可悬停或键盘
聚焦查看。徽章用于弃用等状态。手机卡片保留模型品牌图标，操作菜单位于标题
旁；描述使用普通字重，品牌文字保留在桌面列中。模型名称直接打开详情，支持
键盘和触摸操作，关闭详情后焦点返回原入口。详情顶部提供复制模型名与测试入口，
长说明在正文中按需展开。

概览时间选择与刷新共用一行，按钮直接显示当前选择。指标在手机上按两列排列，
末项费用占满一行；宽屏保留五列。统计口径与记录起点集中在指标下方说明，
趋势图副标题显示所选时段。视觉验收以中文桌面与窄屏为主，英文检查长文案布局。

共享分页按表格容器宽度切换：窄处显示当前页、总页数和前后翻页，宽处提供页码
直达与首尾页按钮。条数与导航分组允许换行，控件与焦点轮廓完整可见。

### Upstream connections and access

列表详情优先在当前行下展开，保留列表位置。主行的非交互区域可点击，标题旁
用带箭头的按钮提供键盘入口，按钮声明 `aria-expanded` 和 `aria-controls`。
复选框、开关、链接和操作菜单独立响应。详情以缩进和边线区分，可同时展开多行；
窄屏沿用同一内容并允许长名称换行。详情按需加载，加载失败显示重试，未知值
保持未知。编辑和深入诊断仍保留独立入口。共享组件见[组件地图](components.md)。

Independent upstreams use the same brand identities as site presets. The connect
action is available above both channel views. Connection cards expose labeled credential,
model and route counts as direct links to the corresponding detail tabs. Missing
configuration gets a concrete next step; enabled state is not a live health result.
Routing groups show the client model name followed by priority-ordered upstream
models, platform and credential identities, configured protocol choices, and
selection/cooldown states. Manual selection is labeled explicitly; configuration
eligibility is separate from actual request history. Status must remain readable
without color, and missing runtime data is unknown rather than healthy.
The overview's configuration checklist has three milestones: upstreams, public
model routes and downstream keys. Site accounts and OAuth accounts contribute to
the first milestone alongside independent upstream connections. Existing sites
can continue through account creation; direct-key users do not need a site or
account. Configured counts never imply that a request has been verified.

API-key editing keeps a dimension summary above the scrolling controls, with
direct navigation to each control.
Scope pickers show selected counts, search and a selected-only view; locating
selected entries clears the search and focuses a selected control. Filtering
does not mutate selections, including unavailable references. The list's scope
column describes additional restrictions rather than claiming the key can call
every model. Present-empty direct-channel policies, zero quotas, blocked sources
and unreadable policies remain distinct states.

Creation starts with New API, domestic platforms and Coding Plan presets. Each product
has one branded card; protocol variants share that card, while distinct products
such as Bailian and Bailian Coding Plan remain separate. The normal form asks for
a key and, for a self-hosted gateway, its address. Connection names, custom hosts
and proxy settings live in a collapsed advanced section. OAuth products use their
existing authorization flow. The detail sheet uses four short tabs:
connection, credentials, models and routes. Models expand on demand, and batch
entry uses one upstream model name per line. Quick connect discovers models and
initializes their access and routes; a preset or empty discovery result must not
be presented as verified model availability. Protocol selection is automatic.
Explicit access restrictions and endpoint editing remain available in advanced
management. Capabilities are grouped as conversation, retrieval, images, audio,
video and dedicated native APIs inside that advanced view. The five conversation
protocols (including Ollama) support native-first conversion ordering. Video
formats use a separate order; System One and Alpha Search never join conversation
conversion. Anonymous credentials show no secret input and can grant only
anonymous endpoints. Media permissions remain exact capabilities.
Refetch updates untouched fields while
preserving edits. Deleting an edited entity first confirms draft loss; leaf
deletion retains undo, and dependent deletion reviews the server's current impact.

## 2. Color tokens

All values live in `web/src/styles/theme.css` under `:root` (light) and `.dark` (dark); presets override them per `data-theme-preset` in `theme-presets.css`.

### 2.1 Theme architecture

| Layer | Mechanism |
| ----- | --------- |
| Theme mode   | `<html>` carries `class="dark"` or `class="light"` plus a `data-theme` attribute (compat), set by `ThemeProvider` and the FOUC bootstrap in `web/index.html`; persisted in the `vite-ui-theme` cookie (1y) and falling back to `prefers-color-scheme` |
| Preset       | `<body data-theme-preset>` — 10 shipped presets: default, graphite, cobalt, lagoon, kelp, moss, ochre, ember, berry, plum |
| Font axis    | `<body data-theme-font>` — `sans` or `serif` (the `default` setting resolves to `sans`, see `public/theme-init.js`); swaps `--font-body` |
| Radius axis  | `<body data-theme-radius>` — `default`, `none`, `sm`, `md`, `lg`, `xl`; overrides `--radius` |
| Density axis | `<body data-theme-scale>` — `default`, `sm`, `lg`, `xl`; rescales `--text-*` and `--spacing` |
| Content axis | `<body data-theme-content-layout>` — `full` or `centered` |

Constants (allowed values, defaults, cookies `theme_preset` / `theme_font` / `theme_radius` / `theme_scale` / `theme_content_layout`): `web/src/lib/theme-customization.ts`.

### 2.2 Core semantic surfaces

| Token                     | Light                                 | Dark                                    | Usage                                             |
| ------------------------- | ------------------------------------- | --------------------------------------- | ------------------------------------------------- |
| `--background`            | `oklch(1 0 0)`                        | `oklch(0.205 0.006 262)`                | App canvas                                        |
| `--foreground`            | `oklch(0.15 0 0)`                     | `oklch(0.955 0.004 262)`                | Primary text                                      |
| `--card`                  | `oklch(1 0 0)`                        | `oklch(0.245 0.007 262)`                | Cards, tables, drawers                            |
| `--popover`               | `oklch(1 0 0)`                        | `oklch(0.265 0.007 262)`                | Popovers, menus, dropdowns                        |
| `--secondary` / `--muted` | `oklch(0.955 0.004 262)` / `oklch(0.966 0.002 262)` | `oklch(0.3 0.008 262)` / `oklch(0.275 0.007 262)` | Nested wells, subdued fills                       |
| `--muted-foreground`      | `oklch(0.455 0.008 262)`              | `oklch(0.76 0.008 262)`                 | Meta/secondary text                               |
| `--border`                | `oklch(0.918 0.003 262)`              | `oklch(1 0 0 / 11%)`                    | Hairlines                                         |
| `--input`                 | `oklch(0.918 0.003 262)`              | `oklch(1 0 0 / 18%)`                    | Input borders                                     |
| `--ring`                  | `oklch(0.6 0.15 262)`                 | `oklch(0.64 0.13 262)`                  | Focus rings (`focus-visible:ring-3 ring-ring/50`) |
| `--overlay`               | `oklch(0 0 0 / 0.12)`                 | `oklch(0 0 0 / 0.34)`                   | Modal/sheet scrim                                 |

Each maps a Tailwind alias `--color-*` (e.g. `--color-background: var(--background)`) in the `@theme inline` block.

### 2.3 Brand / accent

| Token                                                  | Light                                                        | Dark                                                         | Usage                               |
| ------------------------------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------ | ----------------------------------- |
| `--primary`                                            | `oklch(0.565 0.19 262)`                                      | `oklch(0.71 0.155 262)`                                      | Primary actions, active nav         |
| `--primary-foreground`                                 | `oklch(0.985 0 0)`                                           | `oklch(0.21 0.03 262)`                                       | Text on primary                     |
| `--accent`                                             | `color-mix(in oklch, var(--primary) 12%, var(--background))` | `color-mix(in oklch, var(--primary) 20%, var(--background))` | Soft primary fill / active chip     |
| `--neutral`                                            | `oklch(0.556 0 0)`                                           | `oklch(0.7 0.004 262)`                                       | Cool gray secondary                 |
| `--sidebar` / `--sidebar-primary` / `--sidebar-accent` | derived from background/primary                              | derived from background/primary                              | Sidebar canvas, brand, active tones |

Presets replace `--primary` and the derived accent/ring/chart slots per `data-theme-preset` (e.g. ember `oklch(0.575 0.17 40)` with its split-complementary teal secondary); `graphite` carries a full achromatic palette.

### 2.4 Status semantics

| Token                                        | Light                                            | Dark                                             | Usage              |
| -------------------------------------------- | ------------------------------------------------ | ------------------------------------------------ | ------------------ |
| `--success` / `--success-foreground`         | `oklch(0.54 0.14 152)` / `oklch(0.985 0 0)`      | `oklch(0.71 0.14 152)` / `oklch(0.19 0.03 152)`  | Healthy / active   |
| `--warning` / `--warning-foreground`         | `oklch(0.68 0.15 82)` / `oklch(0.24 0.04 82)`    | `oklch(0.79 0.14 82)` / `oklch(0.2 0.035 82)`    | Degraded / pending |
| `--destructive` / `--destructive-foreground` | `oklch(0.58 0.21 24)` / `oklch(0.985 0 0)`       | `oklch(0.58 0.19 24)` / `oklch(0.985 0 0)`       | Errors, deletes    |
| `--info` / `--info-foreground`               | `oklch(0.54 0.12 222)` / `oklch(0.985 0 0)`      | `oklch(0.7 0.11 222)` / `oklch(0.19 0.025 222)`  | Informational      |

Badge pattern: solid on soft fill (e.g. `bg-success/10 text-success`); each status also maps a `--color-*` Tailwind alias.

### 2.5 Charts

`--chart-1…5` in both themes drive recharts series (SVG resolves the CSS `var()` palette directly); dark reuses the same five hues at lighter lightness. Non-color status encoding is required for availability buckets (see a11y checklist).

### 2.6 Glass material

No dedicated `--glass-*` variables. Glass is a Tailwind recipe:

| Surface                | Recipe                                                                          |
| ---------------------- | ------------------------------------------------------------------------------- |
| Topbar / sticky chrome | `bg-background/95 supports-[backdrop-filter]:bg-background/60 backdrop-blur-lg` |
| Modal / sheet scrim    | `bg-overlay supports-backdrop-filter:backdrop-blur-xs`                          |
| Floating toolbars      | `bg-background/95 supports-[backdrop-filter]:bg-background/60 backdrop-blur-lg` |

Fallback: `supports-[backdrop-filter]` gates translucency so browsers without `backdrop-filter` keep a solid surface; `prefers-reduced-transparency` reduces to solid elevated surface.

### 2.7 Focus

`--ring` (per theme) + shadcn recipe `focus-visible:ring-3 focus-visible:ring-ring/50`; global base `outline-ring/50` in `styles/index.css`. Never remove the outline without a visible replacement ring.

---

## 3. Spacing, radius, elevation, motion, type, layout

| Family           | Mechanism                                                                                                                                                                                                                                                                                                                                           | Notes                                                                                                                                                                                            |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Spacing          | Tailwind 4 scale — base `--spacing` (default 0.25rem; `data-theme-scale` overrides 0.225 / 0.28 / 0.3rem)                                                                                                                                                                                                                                           | `gap-*`, `p-*`, `m-*`, `space-y-*`; no custom `--space-*`                                                                                                                                        |
| Radius           | `--radius: 0.625rem` (10px default) with `--radius-sm/md/lg/xl/2xl/3xl/4xl` derived; `data-theme-radius` overrides `--radius` (none 0 · sm 0.3 · md 0.5 · lg 0.75 · xl 1rem)                                                                                                                                                                                 | Controls/buttons `rounded-lg`; cards/sheets `rounded-xl`+                                                                                                                                        |
| Shadow           | Tailwind default `shadow-*` + custom `--shadow-card-hover` (`0 4px 12px …`)                                                                                                                                                                                                                                                                         | Hover elevation on cards (`[data-card-hover]`); no lift on plain rows                                                                                                                            |
| Motion           | `tw-animate-css` utilities (`animate-in/out`, `fade-in-*`, `zoom-in-*`) + the keyframes actually defined in `styles/index.css` (`tableRowEnter`, `sidebarViewEnter`, `slideDown`/`slideUp`, skeleton `shimmer`)                                                                                                                                                                                               | Calm; every animation guarded by `prefers-reduced-motion`                                                                                                                                        |
| Type             | `--font-sans` Inter Variable (Latin) + native CJK fallbacks (PingFang / Microsoft YaHei UI / Noto Sans CJK) · `--font-serif` Lora Variable + CJK serif fallbacks (native CJK serif fallback) · `--font-mono` Cascadia/SFMono/Consolas · `--font-body` active face · `:root[lang\|='zh']` zeroes `--tracking-tight` and the editorial tracking tokens (negative tracking is a Latin display convention; ideographs keep their side bearing)                                                                                                                     | `data-theme-font` swaps the body face; density axis rescales `--text-2xs…3xl` (sub-xs caption tokens registered in theme.css so captions ride the axis instead of freezing at px literals); visible axis tick labels and body text minimum 10px — 9px only allowed for decorative labels with a `title`/`aria-label` fallback |
| Page title scale | Landing/hub pages: `page-title-overview` (30px default); data/list pages: `page-title` (24px default). Both use semibold, tight leading and balanced wrapping | Exactly one h1 per page; Chinese tracking stays neutral; serif axis retains medium weight; title line-height follows density scaling |
| Layout | `data-theme-content-layout` remains `full` or `centered`; full caps content at 1920px above ultrawide viewports, centered caps at 1280px from 1280px viewport width; utilities `max-w-container` 1280 / `max-w-container-lg` 1536 | Header and sidebar remain viewport-wide; only the inset content is constrained |

### 3.1 Reading hierarchy and controls

The base UI text is 14px. Page titles use 24px (overview display titles 30px), section/card titles 16px semibold, field labels and row bodies 14px, and form help and table headers 12px. Sortable headers keep the same size and weight as plain headers. KPI values use the body face with semibold tabular numerals at 20–30px; monospace is reserved for code, keys and technical identifiers. User-selected font and density settings still apply.

Table-toolbar search, filters, column settings and reset controls share the 32px compact size. Form inputs retain the 36px default size. Settings forms have an 896px maximum page width; rate tables and log readers opt into full width through section metadata. Creation feedback uses the next dialog when it opens immediately, leaving its heading unobscured.

The Connect dialog keeps the endpoint and key visible above two tabs: request verification and client setup. Client profiles show their preview only after the key is revealed; copy actions remain available while masked.

Sidebar section labels keep readable foreground colors. Active navigation forwards both `data-active` and `aria-current=page` through its Link bridge. Reading roles belong to the shared components.

Model names use the shared `ModelPill` identity: a vendored brand glyph beside the full model identifier, with truncation and a full-name tooltip. Use its pill variant in dense records and its inline variant in selectors, rankings and headings where another container already supplies the surface. Unknown models keep a deterministic lettermark using the secondary surface and foreground pair; never borrow an unrelated vendor logo. Decorative glyphs are hidden from assistive technology so a selectable model keeps its exact accessible name. Aggregates such as Other are not model identities. Status badges remain separate from brand identity.

Proxy-log failure reasons use the shared keyboard-operable two-line disclosure. On mobile, status and failure copy occupy a full-width labeled field below the timestamp. Row Enter/Space shortcuts ignore nested interactive controls. Timing is a two-row label/value display shared by the list and detail sheet: response-header TTFB and complete attempt duration remain distinct, with missing measurements shown as unknown. Account editing uses semibold field labels and 24px form spacing with the shared form-control primitives.

Tables inherit the 14px body scale and tabular numerals. Cells and descendants must not be force-sized by the table primitive: explicit metadata, badge and secondary-label sizes belong to their components. Default control height is 36px (small 32px, large 40px); the default radius token is 10px. User-selected density, radius and font axes remain independent.

Detail sheets keep 12px medium labels above 14px normal-leading values. Plain text longer than 96 characters or spanning more than two source lines spans the detail grid, shows two lines at rest and exposes a keyboard-focusable expand/collapse control; the full text remains in the DOM for selection and assistive technology. Rich values (such as safe links) may use the same disclosure while retaining their link semantics. Diagnostic copy uses the same disclosure, not a hover-only tooltip. Mobile card titles use 14px semibold; labels stay 12px medium and values 14px normal-leading, while tertiary metadata may use the density-scaled 2xs role. Long values wrap, and long titles may occupy up to two lines rather than being silently cut to one.

Route editing presents matching and account selection first; advanced display/routing fields stay in a disclosure that preserves drafts. Invalid advanced fields must reveal and use the shared form validation focus. Filtering accounts limits bulk selection to visible matches without clearing hidden selections. Rebuild success uses a compact summary with optional metrics; partial failures, observation failures and retry actions remain visible.

Route inline expansion is a semantic channel inventory, not a nested table. The section header uses compact labeled badges for enabled count, total weight, cooling and missing credentials. Each channel keeps one account identity block with visible site and credential context, then groups weight/share, success/failure and state as independently labeled signals. Compact prices read input → output per million and keep provenance in the title. Mobile wraps those signals below the identity instead of hiding site, price or status.

OAuth 路由诊断在候选下折叠展示成员，使用分隔线区分成员，姓名和阻断原因完整换行。
成员筛选状态与父路由状态分别标注；冷却时间旁提供单成员恢复，成功后刷新诊断。
诊断保留未评估状态，选中成员和实际转发模型由请求时的选路决定。

路由诊断候选沿用服务端顺序；优先级与权重明确标为配置，不冒充实时评分或
选中结果。候选清单提供筛选通过、受阻与未评估计数，按身份、模型与阻断原因
检索完整候选集，再按二十项逐批显示。检索和筛选属于当前结果内的临时阅读态，
不写入主列表 URL；当前检查与保存快照互不共用阅读状态。协议未评估时不把
候选标为通过。账号候选的通道禁用或来源模型不匹配提供实际命中路由的编辑入口；
直接上游授权问题不指向账号路由编辑。无命中路由、无候选与筛选无匹配分别呈现。
工作区使用同一输入再次检查时显式重新请求；检查失败隐藏上次候选与准入结论，
不将缓存当作当前检查结果。检查请求期间禁用重复提交，空模型不发起检查。

账号详情中的可用 API Key 提供路由配置动作，仅把账号、站点和 Key ID 传入 URL。新建路由以该 Key 初始化手工通道，行内显示 Key ID；取消后重选仍保留该 Key。编辑已有路由不自动增加绑定，切换到组合路由时只提交来源路由。Key 列表加载失败显示重试入口，旧缓存不提供新的绑定动作。

Channels keep account-derived channels as the default view. Imported upstreams have a separate list and a detail sheet with connection, credential and route tabs; an installation containing only imported upstreams opens that list directly. Provider identity uses a logo capsule, model identity uses `ModelPill`, and protocol names describe the connection rather than masquerading as providers. Tabs retain drafts and the sheet uses the shared dirty-close guard. Request settings load only after an explicit edit action and never enter Query caches; credential replacement clears inputs only after success. Eligibility, disabled dependencies and temporary cooldown are distinct states.

上游凭据采用紧凑分隔清单，默认显示前五项；展开后提供全列表搜索与逐批加载，
名称、ID 和凭据类型均可查找。展开不滚动抢位，焦点进入搜索框。编辑器首次
打开才挂载，打开过的草稿在折叠、筛选和列表收起后仍保留；删除与关闭详情
继续沿用现有确认与脏表单保护。搜索无匹配和空凭据列表分别显示，不混为一谈。

### 3.2 Font roles and rendering

Inter is locally bundled for Latin UI text. Chinese follows the platform UI face (PingFang on macOS, Microsoft YaHei UI on Windows, Noto Sans CJK on Linux). This intentionally prioritizes native CJK rendering over identical glyphs on every platform; screenshot baselines must run in a fixed font environment. Public Sans and the bundled Noto Sans SC slices are no longer shipped.

Model identifiers use the standard sans-serif UI stack at 14px medium weight, including inside capsules and selectors; do not style model names as code. The medium weight distinguishes identities from secondary counts without making long identifiers look bold. Provider headings in the picker use semibold with quiet separators; overview rows keep model names at 14px and counts at 12px. Actionable attention text is 14px, not caption-sized. KPI figures retain the shared monospace stack. Model identity rows align a 16px icon box with a 20px text line, use middle vertical alignment in surrounding inline text, and retain the complete identifier in a tooltip. Body, title, numeric and code roles remain distinct; do not stretch glyphs or compensate with transforms.

Validate actual rendered font names and model icon/text box centers in the browser, then inspect light/dark desktop screenshots. A font-family declaration alone does not prove which CJK face rendered.

---

## 4. Components

Primitive ownership map: [`components.md`](./components.md). Component props and variants are defined by the implementation in `web/src/components/ui/**`, not duplicated in documentation.

State-management rules for URL-synced tables and filters (single URL owner, stable callbacks, one-transaction updates): [`state-stability.md`](./state-stability.md).

New UI must start from the primitives assigned in that ownership map. Import
shared primitives through `@/components/ui/*`; the component map, not this
page, maintains the layer-to-directory list.

### 4.1 Destructive-action tiers（破坏性操作四档）

Every destructive action maps to exactly one tier; never hand-roll a variant:

| 档 | 形态 | 适用 | 实现 |
|---|---|---|---|
| 直达 | 无确认 | 可逆切换（启用/停用、置顶、刷新） | 直接 mutation |
| 删除+undo | 无弹窗；行即消失 + 6s 可撤销 toast | 叶子实体单行删除（redirects、catalog sources、routes、downstream keys、account tokens） | `useUndoableDelete`（`@/lib/undoable-delete`） |
| 批量确认 | 计数确认弹窗 | 批量操作、清空、跨页应用 | `ConfirmDialog`（含 count 文案） |
| typed-confirm | 倒计时 + 输入确认词 | 不可逆/级联重操作（factory reset、站点/账号级联删除） | danger-zone 模式（倒计时 + 确认词） |

例外必须注释标注（例：OAuth 连接删除用 ConfirmDialog + 既有跨页乐观回滚 hook，不移交 undo helper）。

### 4.2 Settings page skeleton（设置页骨架）

- 单 h1 在 `SettingsPage` 页头，文案来自 section meta 的 i18n 键；侧栏树是唯一导航面（无页内面包屑或二级侧栏）。
- `SectionCard`（`components/common/section-card.tsx`，settings 各节与 `/downstream-keys` 页共用）**无 `actions` 时不渲染 CardHeader**——同一组 title/description i18n 键已在页头渲染，卡头再渲染就是逐字重复。带 header actions（测试/保存/批量按钮）的卡保留卡头（按钮需要宿主），但**单卡 section 传 `hideHeaderCopy`**：页头同题文案已在页面顶层，h2 重复无辨识价值；h2 只保留给多内容页内需区分的卡片。
- 危险/前置警告信息放卡内正文顶部 banner（`border-warning/40 bg-warning/5` + `TriangleAlert`），不藏在 description 里（参考：数据迁移节的 wipe/重启警告）。

---

## 5. Visual acceptance

1. `cd web && bun run test` — vitest unit/component suites
2. `cd web && bun run typecheck` — TS gate
3. `cd web && bun run lint` — oxlint
4. `cd web && bun run build` — production bundle gate (`build:web`)
5. `cd web && bun run a11y:scan` — axe-core serious/critical gate (needs the dev server; see `web/scripts/a11y-scan.mjs`). Also enforced in CI: the `a11y` job serves the real embedded SPA via the Go server (fresh sqlite runtime DB) and scans it in both shipped locales (`BASE_URL`-driven; `.github/workflows/main.yml`). The route list is `DESKTOP_ROUTES` in `web/scripts/route-smoke.mjs`, which `a11y-scan.mjs` imports so the two gates cannot drift — this document deliberately does not restate the count.
6. `cd web && bun run ui:smoke` — real-Chromium route/crash/mobile smoke gate (`web/scripts/route-smoke.mjs`); also enforced in CI in the `a11y` job against the shipped bundle
7. Manual score rubric (target ≥ 4/5 each):
   - Material (glass/solid hierarchy)
   - Brand calm (GCP blue, no neon)
   - Spacing rhythm
   - Card elevation / radius
   - Motion restraint
   - Dark parity

---

## 6. a11y non-negotiables

- Focus-visible rings via the `--ring` recipe (`focus-visible:ring-3 ring-ring/50`)
- Contrast on soft badges in both themes
- `prefers-reduced-transparency` / `prefers-reduced-motion`
- FOUC: cookie-first bootstrap in `web/index.html`; no white flash in dark

Checklist: [`a11y-checklist.md`](./a11y-checklist.md).

---

## 7. History

Visual change history lives in root [`CHANGELOG.md`](../../../CHANGELOG.md); this document states only current truth.
