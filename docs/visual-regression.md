# UI screenshot evidence + golden visual regression

**Last updated**: 2026-08-23

Metapi 有两个协作的截图管道，都跑在 CI 的 `frontend` job 产物（`web-dist`
artifact）之上，模式与 a11y job 一致：Go server 嵌入 dist、fresh sqlite
runtime DB、`AUTH_TOKEN=dev-admin-token-123` 经 localStorage 注入、启动后
等 `/ready` 再执行浏览器步骤。

## 1. 截图证据管道（job: `ui-screenshots`）

`web/scripts/screenshot-scan.mjs` 用真 Chromium 对打包后 SPA 做全路由采集：

- 40 条 desktop + 14 条 mobile 路由 × light/dark × DPR 2 全页 PNG
  （另含每主题 1 张 /sign-in 无鉴权页），实测 ~6 分钟（110 张）。
- 输出目录含 `MANIFEST.md`（路由/主题/尺寸/体积清单），随 artifact
  `ui-screenshots` 上传（保留 7 天）。
- 任意路由采集失败 → 脚本非零退出 → job 红（证据管道关门）。

可裁剪 knob（默认全量，CI 不传即为全量）：

| 环境变量 | 默认 | 说明 |
| --- | --- | --- |
| `THEMES` | `light,dark` | 主题子集 |
| `VIEWPORTS` | `desktop,mobile` | 视口子集（预算不够时裁 desktop） |
| `MOBILE_SAMPLE` | 全部 mobile 路由 | mobile 抽查子集（逗号分隔） |
| `DPR` | `2` | 设备像素比 |
| `OUT_DIR` | OS 临时目录 | 输出目录 |

本地运行（起服务见 `web/scripts/a11y-scan.mjs` 头部或 CI a11y job 步骤）：

```bash
cd web
node scripts/screenshot-scan.mjs   # 默认连 BASE_URL=http://127.0.0.1:4099
```

## 2. 黄金基线回归（job: `visual-regression`）

4 个关键页 golden 基线回归，用 Playwright `expect(page).toHaveScreenshot()`：

- 页面：`/`（dashboard）、`/token-routes`、`/accounts`、`/sites`。
- 维度仅 desktop + light（低维度避免每月全量重生成基线；dark/mobile 由证据
  管道覆盖）。
- spec：`web/scripts/visual-regression.spec.mjs`；
  配置：`web/playwright.visual.config.mjs`；
  基线：`web/visual-baselines/*.png`（入库提交）。
- CI 里 `updateSnapshots: none`——基线缺失或漂移即红；失败时
  `visual-regression-diffs` artifact 上传 diff（actual/baseline/diff 三件套）。

### 本地运行与基线更新

```bash
cd web
bun run visual:regression                       # 对照入库基线
UPDATE_SNAPSHOTS=all bun run visual:regression  # 重生成基线（有意 UI 改动后）
git add visual-baselines/*.png && git commit
```

注意事项：

- 基线必须在装满 `fonts-noto-cjk` 的 Linux 上生成（CI 已装；Windows 上
  生成的基线字型渲染与 CI 不一致，勿用）。
- 服务器须跑 fresh sqlite DATA_DIR（空库 → 页面无时间敏感内容，基线跨天稳定）。
- `maxDiffPixelRatio: 0.01` 只容忍抗锯齿亚像素抖动；布局漂移会超量级。

## 3. README 店头截图（`docs/assets/screenshots/*.webp`）

README gallery 的 8 张 webp 是人工挑选/裁剪后的产物：用
`screenshot-scan.mjs`（走 dev server + dev seed）在 desktop/light 下
扫出 PNG，选中页面后转 webp（保留 30-50KB 量级）再更新 README 引用。
CI 不产 webp——它是发布物料，不是回归证据。
