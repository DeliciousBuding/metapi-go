# Data Table

The list-page framework every metapi-go table is built on: TanStack Table state,
a responsive page composition, and the column conventions features declare.

## Public API

`index.ts` is the whole public surface — feature code imports from
`@/components/data-table` and never reaches into a subdirectory. Everything
exported there is frozen against feature call sites; everything not exported
there is free to be reorganised.

## Layout

| Directory  | Contents                                                                                                                                                                                                                                                                                                                                                             |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `core/`    | Rendering primitives: `data-table-view` (the `<table>`), header, row, pagination, empty state, loading skeleton. Plus the column model — `table-sizing` (the width budget shared by table / colgroup / header), `column-pinning` (sticky edges), `column-label` (a column's human name), `types` (the `ColumnMeta` augmentation) — and the shared cells `badge-list-cell`, `truncated-cell`, `status-badge`. |
| `layout/`  | `DataTablePage`, the composition features actually render: toolbar + desktop table + mobile card list + pagination, with the table↔card switch at `TABLE_MOBILE_MAX_WIDTH`. `card-cell-utils` / `card-row-content` turn the same column definitions into card fields.                                                                                                |
| `toolbar/` | Search input, faceted filters, view options, and the bulk-action bar that replaces the toolbar while rows are selected.                                                                                                                                                                                                                                              |
| `hooks/`   | `useDataTable` (the controlled-state layer), `useUrlTableState` / `encodeSorting` (URL ⇄ table state), and the package-private `useDebounce`.                                                                                                                                                                                                                        |

## Conventions

**Column meta drives the responsive layout.** A column declares `mobileTitle` /
`mobileBadge` / `mobileHidden` / `mobileOrder` once and both the mobile card
list and the desktop card grid render it; `label` supplies a card field name
when `header` is a sort/filter component with no string to reuse. See
`core/types.ts` for the augmentation and its semantics.

**URL state is three-stage.** Route `validateSearch` owns parsing, the feature's
`useSearch` owns the current value, `useDataTable` owns the controlled table
state. Keeping the stages separate is what lets a table's sort/filter/page
survive a reload and a back-navigation without the table owning routing.

**Feature-specific columns, actions and dialogs stay in the feature folder.**
Code belongs here only once a second feature needs it.
