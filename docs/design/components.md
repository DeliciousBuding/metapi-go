# UI component ownership map

**Last verified**: 2026-08-16

The implementation is the primitive API source of truth. The pre-rewrite `ds-*` library and its gallery no longer exist; use Git history when that implementation history is needed.

| Concern                           | Owner                                                                     |
| :-------------------------------- | :------------------------------------------------------------------------ |
| Base UI / shadcn primitives       | `web/src/components/ui/**`                                                |
| Data-table composition            | `web/src/components/data-table/**`                                        |
| Application shell                 | `web/src/components/layout/**`                                            |
| Semantic tokens                   | `web/src/styles/theme.css` and `theme-presets.css`                        |
| Tailwind entry and global recipes | `web/src/styles/index.css`                                                |
| Visual and accessibility rules    | [`DESIGN.md`](./DESIGN.md) and [`a11y-checklist.md`](./a11y-checklist.md) |

## Usage rules

1. Start with an existing primitive and import it through `@/components/ui/*`; feature-specific composition stays in the owning feature.
2. Use semantic OKLCH tokens and Tailwind utilities. Do not revive `ds-*`, copy a primitive into a feature, or hard-code a parallel token set.
3. Keep accessibility behavior in the primitive owner: names, labels, focus rings, invalid state, keyboard interaction, and reduced-motion/transparency behavior.
4. Confirm light/dark rendering, keyboard behavior, focused Vitest coverage, typecheck, lint, and production build for changed primitives.
