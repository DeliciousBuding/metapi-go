// metapi-go/data-table — ported from newapi
export { DataTablePagination } from './core/pagination'
export { DataTableColumnHeader } from './core/column-header'
export { BadgeCell } from './core/badge-cell'
export { BadgeListCell } from './core/badge-list-cell'
export { TruncatedCell } from './core/truncated-cell'
export { DataTableViewOptions } from './toolbar/view-options'
export { DataTableToolbar } from './toolbar/toolbar'
export { DataTableBulkActions } from './toolbar/bulk-actions'
export {
  StaticDataTable,
  type StaticDataTableColumn,
} from './static/static-data-table'
export { StaticRowActions } from './static/static-row-actions'
export { staticDataTableClassNames } from './static/static-data-table-classnames'
export {
  DataTableRow,
  DataTableRowActionMenu,
  DataTableView,
  type DataTableColumnClassName,
  type DataTablePinnedColumn,
  type DataTableRenderRowHelpers,
} from './core/data-table-view'
export { MobileCardList } from './layout/mobile-card-list'
export {
  DataTableCardGrid,
  type DataTableCardGridProps,
  type DataTableCardHelpers,
} from './layout/card-grid'
export { CardRowContent } from './layout/card-row-content'
export { tableHasCompactMeta } from './layout/card-cell-utils'
export {
  DataTablePage,
  type DataTablePageProps,
} from './layout/data-table-page'
export {
  DataTableViewModeToggle,
  type DataTableViewModeToggleProps,
} from './toolbar/view-mode-toggle'
export { useDataTable } from './hooks/use-data-table'
export {
  useDataTableViewMode,
  DATA_TABLE_VIEW_MODES,
  type DataTableViewMode,
} from './hooks/use-data-table-view-mode'
export { useDebouncedColumnFilter } from './hooks/use-debounced-column-filter'

export const DISABLED_ROW_DESKTOP =
  '[--data-table-card-bg:var(--table-disabled)] hover:[--data-table-card-bg:var(--table-disabled-hover)] data-[state=selected]:![--data-table-card-bg:var(--table-disabled)] data-[state=selected]:hover:![--data-table-card-bg:var(--table-disabled-hover)] [background-color:var(--table-disabled)] hover:[background-color:var(--table-disabled-hover)] [&>td:first-child]:[border-left-color:var(--table-disabled-border)] [&>td:first-child]:border-l-4 [&>td:first-child]:pl-1'

export const DISABLED_ROW_MOBILE =
  '[--data-table-card-bg:var(--table-disabled)] data-[state=selected]:![--data-table-card-bg:var(--table-disabled)] [background-color:var(--table-disabled)]'
