# UI component ownership map

**Last updated**: 2026-10-19

The implementation is the primitive API source of truth. The pre-rewrite `ds-*` library and its gallery no longer exist; use Git history when that implementation history is needed.

| Concern                           | Owner                                                                     |
| :-------------------------------- | :------------------------------------------------------------------------ |
| Base UI / shadcn primitives       | `web/src/components/ui/**`                                                |
| Cross-feature shared composition  | `web/src/components/common/**` (section card / error / skeleton, query-error banner, confirm dialog, master-token reauth host, HTTP status badge + the shared soft-tone badge recipe, credential export, safe external link) |
| Data-table composition            | `web/src/components/data-table/**` (public surface is its `index.ts`; see [`web-package-boundaries.md`](../web-package-boundaries.md) rule 6) |
| Form plumbing                     | `web/src/components/form/**` (dirty-close guard)                          |
| Application shell                 | `web/src/components/layout/**`                                            |
| Semantic tokens                   | `web/src/styles/theme.css` and `theme-presets.css`                        |
| Tailwind entry and global recipes | `web/src/styles/index.css`                                                |
| Visual and accessibility rules    | [`DESIGN.md`](./DESIGN.md) and [`a11y-checklist.md`](./a11y-checklist.md) |

## Usage rules

1. Start with an existing primitive and import it through `@/components/ui/*`; feature-specific composition stays in the owning feature. A composition moves up to `@/components/common/*` when a **second** feature needs it — not before, and the move lands together with that second consumer (each such move is recorded in the [`web-package-boundaries.md`](../web-package-boundaries.md) precedent log).
2. Use semantic OKLCH tokens and Tailwind utilities. Do not revive `ds-*`, copy a primitive into a feature, or hard-code a parallel token set.
3. Keep accessibility behavior in the primitive owner: names, labels, focus rings, invalid state, keyboard interaction, and reduced-motion/transparency behavior.
4. Confirm light/dark rendering, keyboard behavior, focused Vitest coverage, typecheck, lint, and production build for changed primitives.

## Operator workflows

Expandable list rows compose `components/common/row-disclosure.tsx`: a named
`RowDisclosureTrigger`, `rowDisclosureClick` for the non-interactive row surface,
and `RowDisclosureContent` for the indented region. Data tables and ordinary
tables share these controls; feature owners supply the detail content and state.

模型测试场使用开放对话画布，顶部仅保留模型选择和新会话操作。
消息与底部圆角输入框使用同一阅读宽度；空态提供现有模板入口。
输入框工具栏显示当前渠道和协议；渠道、协议、对比、模板、系统提示词与采样
参数集中在按需打开的设置侧栏，手机使用全宽面板。校验失败会打开对应设置。
消息组件采用 [Vercel AI Elements Message](https://elements.ai-sdk.dev/components/message)
的组合方式，并用 Streamdown 渲染 Markdown；控件继续复用现有 Base UI 原语。
共享 Markdown 渲染由 `components/common/markdown-content.tsx` 维护，供测试场
与公告正文复用；HTML 标签不参与页面渲染，远程图片仅显示说明文字。
每轮响应独立保留推理、用量与原始数据。当前测试接口完成同步请求后返回响应，
等待状态显示“等待响应”。对话只有一个滚动容器；阅读旧消息时保持位置，
回到最新后跟随新消息。会话内容仅在当前页面内存中保留。

List pages separate persistent create/import actions from filters. Site status
shortcuts reuse the URL-backed table filter and show counts from the complete
site snapshot; unavailable counts are not shown as zero. Site account counts
link to the existing account filter. Status capsules need one status glyph and
a label; unavailable secondary quota percentages should not fill every row
with a repeated placeholder.
