# Data Table Components

最后更新：2026-08-11

This package keeps a stable public API through `index.ts`; feature code should
continue importing from `@/components/data-table`.

- `core/`: TanStack table rendering primitives, headers, rows, pagination,
  loading, empty states, and pinned-column behavior. Also contains a vendored
  `status-badge.tsx` (local copy of newapi's `@/components/status-badge`) so the
  package stays self-contained while the shared component is not yet ported.
- `layout/`: responsive page-level composition that combines toolbar, desktop
  table, mobile list, bulk actions, and pagination placement. The local
  `PageFooterPortal` fallback renders pagination inline until
  `@/components/layout/components/page-footer` lands.
- `toolbar/`: filter/search/view-option controls and selection action toolbar.
- `static/`: lightweight table rendering for local/static arrays that do not
  need TanStack state.
- `hooks/`: table state and filter hooks. Includes a vendored `use-debounce`
  (local copy of newapi's `@/hooks/use-debounce`) plus `useDataTable` which
  provides the controlled-state layer for the three-stage URL state sync pattern
  (route validateSearch → feature useSearch → useDataTable controlled state).

Keep feature-specific columns, actions, and dialogs inside their feature
folders. Shared table code belongs here only when it is reusable across more
than one feature.
