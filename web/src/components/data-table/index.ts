// metapi-go/data-table — package barrel.
//
// The public API of this package and nothing else: feature code imports from
// `@/components/data-table` and never reaches into `core/`, `layout/`,
// `toolbar/` or `hooks/`. Anything exported here is therefore frozen against
// feature call sites; anything not exported here is free to be reorganised.

export { BadgeListCell } from './core/badge-list-cell'
export { DataTableColumnHeader } from './core/column-header'
export { DataTableRow } from './core/data-table-row'
export { TruncatedCell } from './core/truncated-cell'
export type { DataTableRenderRowHelpers } from './core/types'
export { useDataTable } from './hooks/use-data-table'
export { encodeSorting, useUrlTableState } from './hooks/use-url-table-state'
export type {
  UrlTableState,
  UrlTableStateUpdate,
} from './hooks/use-url-table-state'
export { DataTablePage } from './layout/data-table-page'
export { DataTableBulkActions } from './toolbar/bulk-actions'
