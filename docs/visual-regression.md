# UI screenshot review and golden comparison

**Last updated**: 2026-10-06

截图扫描和 golden 对照是 UI 改动及发版候选的按需人工复核工具，
不在每次 PR/master 的必过 CI 中运行。CI 保留前端测试/构建及真实浏览器
`a11y`（axe + 路由/移动端 smoke）；它们防止功能与严重可访问性回归，
但不能替代有数据、亮暗主题、移动端和高分辨率截图的人工审美判断。
运行截图工具时，Go server 须嵌入新构建的 `web/dist`，使用一次性 sqlite
数据目录，登录后等 `/ready` 再执行浏览器步骤。

> **本地先决条件（`web/dist` 不入 git，踩坑点在此）**：Go server 用 `go:embed`
> 固化**启动时磁盘上的 dist**。`web/dist` 是构建产物（gitignore），
> 本地直接 `go run` 时若 dist 陈旧，
> 服务的是旧版 SPA（如 #1034 会话模型之前的认证守卫），登录页截图会
> "成功"产出 112 张而实际全是登录页。本地 SOP：
>
> ```bash
> cd web && bun run build:web   # 或 bun run build:check
> # 重启 Go server（go run ./cmd/server 或运行既有二进制），务必让新 dist 被重新嵌入
> ```
>
> `screenshot-scan.mjs` 内置 **auth preflight**（登录后导航一次 `/sites`，
> 断言未被弹回 `/sign-in`）：命中陈旧 dist 会立即报错并给出上述修复指引，
> 而不是静默产出全登登录页证据。

## 1. 有数据截图检查

`web/scripts/screenshot-scan.mjs` 用真 Chromium 对打包后 SPA 做全路由采集：

- 40 条 desktop + 14 条 mobile 路由 × light/dark × DPR 2 全页 PNG
  （另含每主题 1 张 /sign-in 无鉴权页），实测 ~6 分钟（110 张）。
- 输出目录含 `MANIFEST.md`（路由/主题/尺寸/体积清单）；本地保留待复核截图，
  不把原始运行截图自动作为公开 CI artifact。
- 任意路由采集失败 → 脚本非零退出；采集成功只证明有截图，仍须逐张检查。
- 需要有数据的页面时，用 `scripts/e2e/seed-demo-data.py` 向一次性运行时库注入确定性
  演示数据（24 个站点、30 个账号，另含路由/渠道/密钥/代理日志/签到/用量聚合），并以
  `EXPECTED_DATA_PROFILE=seeded` 核对注入生效——证据截图展示的是「活的」
  UI 而不是整页空态。注意这与 §2 golden 基线**故意相反**：golden 仍用空库
  （日期无关的布局契约）。
- 同一批演示数据可由 `bun run ui:mobile-list` 在 375px 真浏览器中验证
  站点/账号第 20 张卡与分页可滚达、跨页内容变化、卡面空白不触发选择而复选框
  可选择；页脚截图写入指定的 `MOBILE_LIST_SHOTS_DIR`，用于人工复核。

可裁剪 knob（默认全量，按改动范围缩小抽样）：

| 环境变量 | 默认 | 说明 |
| --- | --- | --- |
| `THEMES` | `light,dark` | 主题子集 |
| `VIEWPORTS` | `desktop,mobile` | 视口子集（预算不够时裁 desktop） |
| `MOBILE_SAMPLE` | 全部 mobile 路由 | mobile 抽查子集（逗号分隔） |
| `DPR` | `2` | 设备像素比 |
| `OUT_DIR` | OS 临时目录 | 输出目录 |
| `EXPECTED_DATA_PROFILE` | 空 | 可选 `empty` / `seeded`；截图前核对站点与账号数据，命名与运行态不符即失败 |

本地运行（**先 build:web 并重启 server**，见上方先决条件）：

```bash
cd web
bun run build:web
# 重启 Go server 使新 dist 被嵌入，然后：
EXPECTED_DATA_PROFILE=empty node scripts/screenshot-scan.mjs   # 默认连 BASE_URL=http://127.0.0.1:4099
```

### 1.1 交互弹层扫描（`web/scripts/shot-interactions.mjs`，本地工具）

静态路由扫描到不了的区域——对话框、Sheet、Popover、表单校验态、顶栏
chrome（cmdk / 待关注告警 / 外观定制 / 用户菜单）——用交互扫描补齐：

```bash
# 同样的先决条件：build:web + 重启 server；另需 seed 站点/账号让列表有行
OUT_DIR=<输出目录> node scripts/shot-interactions.mjs
```

26 个场景（桌面+移动双视口），逐场景失败收集不中断。选择器教训已写进
脚本头注释（告警铃 aria-label 是动态「待关注告警」、Select trigger 是
combobox role 且在 FormControl 内会丢 data-slot、mobile 列表无 tbody）。
此脚本仍是本地探索工具，不能替代按需运行的 `ui:mobile-list` 交互断言。

## 2. 按需 golden 对照

10 个关键页 golden 基线回归，用 Playwright `expect(page).toHaveScreenshot()`：

- 页面：`/`（dashboard）、`/token-routes`、`/accounts`、`/sites`、`/models`、
  `/channels`、`/oauth`、`/model-tester`、`/settings`（overview）、
  `/settings/basic/site`。全部空库、布局稳定、日期无关（契约同前 4 页）。
- 维度仅 desktop + light（低维度避免每月全量重生成基线；dark/mobile 由证据
  管道覆盖）。
- spec：`web/scripts/visual-regression.spec.mjs`；
  配置：`web/playwright.visual.config.mjs`；
  基线：`web/visual-baselines/*.png`（入库提交）。
- 配置使用 `updateSnapshots: none`，本地对照时基线缺失或漂移会报错，
  差异图片保留在 `web/test-results/` 供复核。它只覆盖空库亮色桌面，
  不作为每次 PR 的合并或镜像硬门禁；UI 变更/候选发布仍须结合实际有数据截图。

### 本地运行与基线更新

```bash
cd web
bun run visual:regression                       # 对照入库基线
UPDATE_SNAPSHOTS=all bun run visual:regression  # 重生成基线（有意 UI 改动后）
git add visual-baselines/*.png && git commit
```

注意事项：

- 基线必须在装满 `fonts-noto-cjk` 的 Linux 上生成（CI 已装；Windows 上
  生成的基线字型渲染与 CI 不一致，勿用）。#1260 之后正文中文走内置
  Noto Sans SC Variable（与平台无关），但这条前提仍然成立：mono 上下文里的
  CJK 仍走系统字体，删掉 `fonts-noto-cjk` 会让基线出豆腐块。
- 服务器须跑 fresh sqlite DATA_DIR（空库 → 页面无时间敏感内容，基线跨天稳定）。
- `maxDiffPixelRatio: 0.01` 只容忍抗锯齿亚像素抖动；布局漂移会超量级。

## 3. README 店头截图（`docs/assets/screenshots/*.webp`）

README gallery 的 8 张 webp 是人工挑选/裁剪后的产物：用
`screenshot-scan.mjs`（走 dev server + dev seed）在 desktop/light 下
扫出 PNG，选中页面后转 webp（保留 30-50KB 量级）再更新 README 引用。
CI 不产 webp——它是发布物料，不是回归证据。
