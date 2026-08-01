# New API 新 UI 设计参考（2026-08-02 研究）

**Source**: `D:\Code\Temp\newapi-ui-study`（Calcium-Ion/new-api `web/`，2026-08-02 clone，研究用）
**Scope**: 只提炼设计哲学/细节/token，不照搬功能；落地前按 MetAPI DESIGN.md 语言评估。

---

## 1. 技术栈与设计系统形态

| 维度 | New API 新 UI | MetAPI 现状 | 评估 |
|:-----|:--------------|:------------|:-----|
| 样式体系 | Tailwind CSS v4 + shadcn 语义 token | 手写 tokens.css + `ds-*` primitives | 各成体系，不必迁移 |
| 色彩空间 | **oklch()** 感知均匀色彩 | hex/rgba | 技术升级，用户不可见，暂缓 |
| 字体 | Public Sans（sans）+ Lora（serif 编辑轴，CJK 回退链完整） | 系统字体栈（DESIGN.md 决策：无 CDN） | **保持 MetAPI 决策**（避免字体加载成本） |
| 图表色板 | `--chart-1..5` token 面，随主题预设联动 | `--color-chart-1..8` token 已有，**系列色此前未接线 canvas** | ✅ **已修复**（见 §3） |

## 2. 可借鉴的设计哲学/细节（差距矩阵）

| # | 哲学/细节 | New API 实现 | MetAPI 现状 | 建议 |
|--:|:----------|:-------------|:------------|:-----|
| 1 | **主题预设** | `data-theme-preset` 多套 curated 色板（underground/rose-garden/ocean-breeze…），每套声明 light+dark 双面 + chart 色 + 默认半径 | 仅 light/dark 双主题 | **候选**：MetAPI 已有主题菜单 + 密度开关，可扩展 1-2 套预设（如「终端绿」「海洋蓝」） |
| 2 | **独立密度轴** | `data-theme-scale` 独立于颜色 | 已有 `data-density="compact"` 开关（CHANGELOG DENSE-1） | ✅ 已具备 |
| 3 | **可调半径** | `data-theme-radius` 用户可覆盖预设默认半径 | `--radius-*` tokens 固定 | 候选（低优先：视觉收益小） |
| 4 | **chart token 面** | 图表色是设计系统一部分，随主题联动 | token 已定义，canvas 接线缺失 | ✅ **已修复**（§3） |
| 5 | **sidebar 专属 token 面** | `--sidebar-*` 独立面 | 无独立 sidebar 面 | 候选（当前 glass 侧栏已自洽） |
| 6 | 状态色语义化 | success/warning/danger soft+ink 全套 | 已有 `--color-*-soft/-ink` | ✅ 已具备 |

## 3. 本次落地：图表系列色 token 接线（chart-colors 第三波）

**问题**: VChart canvas 不解析 CSS `var()`。此前仅轴/图例色经 `useChartColors()` 解析（2026-08-01 两波），**系列色**（27 处 `var(--color-chart-N)`，7 个图表文件）全部静默回退 VChart 默认色板——Dashboard 图表颜色与设计系统无关、不随主题。

**修复**: `useChartColors()` 扩展解析 `--color-chart-1..8` + `-soft/-faint` 衍生 + `--color-on-primary`；所有系列色数组/gradient stops/point stroke/pie 数组改用解析值；DOM 图例色块保留 var()（合法）。

**门禁**: chart-colors.gate 新规则——系列色 raw var() 数组/字面量直接报错；注入验证会响。

**验证**: 601 vitest（gate + hook 断言扩展）+ typecheck + build 全绿；提交 `3442b6e`。

## 4. 未采纳（明确理由）

- **字体轴（Public Sans/Lora）**: MetAPI DESIGN.md 决策「system stack only, no Google Fonts CDN」；运维控制台不追求编辑性排版。
- **Tailwind/shadcn 迁移**: 手写 token + ds-* 已与项目自洽，迁移成本远大于收益。
- **oklch 全量迁移**: 纯技术债置换，视觉无差；hex 已双主题覆盖。
- **主题预设（暂缓）**: 工作量中等（多套双面色板 + 切换 UI + 持久化 + 验收），属产品方向取舍——留待拍板后作为独立波。
