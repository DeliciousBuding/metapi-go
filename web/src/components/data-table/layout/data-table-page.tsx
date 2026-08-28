// metapi-go/data-table — ported from newapi
import type {
  ColumnDef,
  Row,
  Table as TanstackTable,
} from '@tanstack/react-table'
import * as React from 'react'

import { useMediaQuery } from '@/hooks/use-media-query'
import { TABLE_MOBILE_MEDIA_QUERY } from '@/lib/breakpoints'
import { cn } from '@/lib/utils'

import {
  DataTableView,
  type DataTableColumnClassName,
  type DataTablePinnedColumn,
  type DataTableRenderRowHelpers,
} from '../core/data-table-view'
import { DataTablePagination } from '../core/pagination'
import { DataTableToolbar } from '../toolbar/toolbar'
import { MobileCardList } from './mobile-card-list'

/**
 * Pass-through configuration for the default {@link DataTableToolbar}.
 * Pass `toolbar` (ReactNode) instead to fully replace the default toolbar.
 */
type DataTablePageToolbarProps<TData> = Omit<
  React.ComponentProps<typeof DataTableToolbar<TData>>,
  'table'
>

export type DataTablePageProps<TData> = {
  /**
   * TanStack Table instance returned from `useReactTable`.
   */
  table: TanstackTable<TData>

  /**
   * Column definitions. Used for skeleton column count and empty-state colSpan.
   */
  columns: ColumnDef<TData, unknown>[]

  /**
   * Initial loading state — renders {@link TableSkeleton} or mobile skeleton.
   */
  isLoading?: boolean

  /**
   * Refetch / background loading — dims the table without removing rows.
   */
  isFetching?: boolean

  /**
   * Empty-state title (used for both desktop {@link TableEmpty} and mobile fallback).
   */
  emptyTitle?: string

  /**
   * Empty-state description.
   */
  emptyDescription?: string

  /**
   * Empty-state icon override (desktop only; mobile uses default Database icon).
   */
  emptyIcon?: React.ReactNode

  /**
   * Empty-state extra content — e.g. a "Create" button below the message.
   */
  emptyAction?: React.ReactNode

  /**
   * Empty-state title shown when rows are empty due to active filters.
   */
  filteredEmptyTitle?: string

  /**
   * Empty-state description shown when rows are empty due to active filters.
   */
  filteredEmptyDescription?: string

  /**
   * Custom toolbar node — fully replaces the default {@link DataTableToolbar}.
   * Useful for layouts like "primary buttons + toolbar" or feature-specific filter cards.
   * If provided, `toolbarProps` is ignored.
   */
  toolbar?: React.ReactNode

  /**
   * Pass-through props for the default {@link DataTableToolbar}.
   * Ignored if `toolbar` is provided. Pass `null` to omit the toolbar entirely.
   */
  toolbarProps?: DataTablePageToolbarProps<TData> | null

  /**
   * Bulk action bar — typically a wrapped {@link DataTableBulkActions} component.
   * Rendered only on desktop (mobile selection is uncommon).
   */
  bulkActions?: React.ReactNode

  /**
   * Custom mobile list node — fully replaces the default {@link MobileCardList}.
   */
  mobile?: React.ReactNode

  /**
   * Pass-through props for the default {@link MobileCardList}.
   * Ignored if `mobile` is provided.
   */
  mobileProps?: {
    getRowKey?: (row: Row<TData>) => string | number
    getRowClassName?: (row: Row<TData>) => string | undefined
  }

  /**
   * Disable the mobile-specific layout entirely — always renders desktop table.
   * Useful for pages where the table is read-only and short.
   */
  hideMobile?: boolean

  /**
   * Row className resolver — applied to both desktop `TableRow` and mobile card.
   * Composes with the default `data-state="selected"` styling on desktop.
   * The `ctx.isMobile` flag is provided so consumers can return the
   * appropriate variant (e.g. `DISABLED_ROW_DESKTOP` vs `DISABLED_ROW_MOBILE`)
   * without having to re-call `useMediaQuery` themselves.
   */
  getRowClassName?: (
    row: Row<TData>,
    ctx: { isMobile: boolean }
  ) => string | undefined

  /**
   * Custom desktop row renderer — replaces the default `<TableRow>`/`<TableCell>` mapping.
   * Use for expanded rows, aggregate rows, click-on-row navigation, etc.
   */
  renderRow?: (
    row: Row<TData>,
    helpers: DataTableRenderRowHelpers
  ) => React.ReactNode

  /**
   * Desktop column className resolver. Use for semantic alignment/spacing only;
   * fixed-column behavior should be configured with `pinnedColumns`.
   */
  getColumnClassName?: DataTableColumnClassName

  /**
   * Fixed desktop columns. The shared table component owns sticky position,
   * layering, shadows, and row-state backgrounds.
   */
  pinnedColumns?: DataTablePinnedColumn[]

  /**
   * Apply explicit column widths from `header.getSize()` to `<TableHead>`.
   * Enable this when your column definitions include `size` and you want it honored.
   * Off by default (TanStack Table assigns a default size of 150 to all columns
   * which would unintentionally constrain layouts that don't define sizes).
   */
  applyHeaderSize?: boolean

  /**
   * Optional skeleton key prefix for stable React keys across re-renders.
   */
  skeletonKeyPrefix?: string

  /**
   * Whether to render pagination. Defaults to `true`.
   */
  showPagination?: boolean

  /**
   * Render pagination via `PageFooterPortal` (sticks to page footer).
   * Defaults to `true`. Set `false` to render inline below the table.
   */
  paginationInFooter?: boolean

  /**
   * Extra content rendered between the table/mobile list and the pagination.
   * E.g. summary stats, helper text.
   */
  afterTable?: React.ReactNode

  /**
   * Outer wrapper className (applied to the toolbar+table column).
   */
  className?: string

  /**
   * Make the desktop table consume the available page height and scroll inside
   * the table body while keeping the header fixed. Defaults to `true`.
   */
  fixedHeight?: boolean

  /**
   * Desktop table container className (the bordered scroll wrapper).
   */
  tableClassName?: string

  /**
   * Desktop `<TableHeader>` className override.
   * Use for header color/spacing overrides. Fixed-height pages keep the header
   * outside the scrollable body automatically.
   */
  tableHeaderClassName?: string
}

/**
 * Unified table page wrapper. Encapsulates the canonical structure used across
 * all list pages: toolbar → desktop table / mobile list → pagination, plus
 * loading/empty states and an opt-in bulk action bar.
 *
 * Most pages should be expressible as:
 * ```tsx
 * <DataTablePage
 *   table={table}
 *   columns={columns}
 *   isLoading={isLoading}
 *   isFetching={isFetching}
 *   emptyTitle={t('No X Found')}
 *   toolbarProps={{ searchPlaceholder: t('Filter...'), filters }}
 *   bulkActions={<MyBulkActions table={table} />}
 * />
 * ```
 *
 * For complex layouts (custom mobile, expanded rows, custom toolbar), use the
 * `toolbar` / `mobile` / `renderRow` slots instead of the `*Props` variants.
 */
export function DataTablePage<TData>(props: DataTablePageProps<TData>) {
  // Shared constant (lib/breakpoints): ≤640px renders the MobileCardList;
  // above it the desktop table keeps horizontal scrolling. The 641–767px
  // band intentionally still uses the mobile drawer navigation (768px
  // threshold in useIsMobile) — see lib/breakpoints for the rationale.
  const isMobile = useMediaQuery(TABLE_MOBILE_MEDIA_QUERY)
  const showMobile = isMobile && !props.hideMobile

  const toolbarNode = renderToolbar(props)
  const mobile_node = renderMobile(props, showMobile)
  const desktopNode = renderDesktop(props, showMobile)
  const paginationNode = renderPagination(props)

  return (
    <>
      <div
        className={cn(
          props.fixedHeight !== false
            ? 'flex h-full min-h-0 flex-col gap-2.5 sm:gap-3'
            : 'space-y-2.5 sm:space-y-3',
          props.className
        )}
      >
        {toolbarNode}
        {mobile_node}
        {desktopNode}
        {props.afterTable}
      </div>

      {/* Bulk actions are typically a fixed-position toolbar; let the consumer
          handle its own visibility. Rendered on mobile too — MobileCardList
          surfaces the row-selection checkbox when the table has a `select`
          column, so bulk enable/disable/delete are reachable on touch. */}
      {props.bulkActions}

      {paginationNode}
    </>
  )
}

function renderToolbar<TData>(
  props: DataTablePageProps<TData>
): React.ReactNode {
  if (props.toolbar !== undefined) {
    // Fully custom toolbar: the consumer owns layout, including any toggle.
    return props.toolbar
  }
  if (props.toolbarProps === null) {
    return null
  }
  if (props.toolbarProps) {
    return (
      <DataTableToolbar
        table={props.table}
        {...props.toolbarProps}
        viewToggle={props.toolbarProps.viewToggle}
      />
    )
  }
  return null
}

function renderPagination<TData>(
  props: DataTablePageProps<TData>
): React.ReactNode {
  if (props.showPagination === false) {
    return null
  }

  const pagination = <DataTablePagination table={props.table} />

  return props.paginationInFooter !== false ? (
    <PageFooterPortal>{pagination}</PageFooterPortal>
  ) : (
    <div className='pt-2'>{pagination}</div>
  )
}

function renderMobile<TData>(
  props: DataTablePageProps<TData>,
  showMobile: boolean
): React.ReactNode {
  if (!showMobile) {
    return null
  }

  const ownGetRowClassName = props.getRowClassName
  const mobileGetRowClassName =
    props.mobileProps?.getRowClassName ??
    (ownGetRowClassName
      ? (row: Row<TData>) => ownGetRowClassName(row, { isMobile: true })
      : undefined)

  let mobileContent = props.mobile
  if (mobileContent === undefined) {
    mobileContent = (
      <MobileCardList
        table={props.table}
        isLoading={props.isLoading}
        emptyTitle={props.emptyTitle}
        emptyDescription={props.emptyDescription}
        emptyAction={props.emptyAction}
        getRowKey={props.mobileProps?.getRowKey}
        getRowClassName={mobileGetRowClassName}
      />
    )
  }

  // Bottom-safe scroll edge: the mask fades the final stretch of the mobile
  // list instead of hard-cutting rows at the container boundary (previously
  // a 2px sliver of the last row's badge rendered at the edge as a stray
  // colored bar). The gradient is an ALPHA ramp only — it carries no color,
  // so the OKLCH token system stays the only source of color (see the
  // no-gradients test allowlist note).
  return (
    <div className='min-h-0 flex-1 overflow-y-auto [mask-image:linear-gradient(to_bottom,black_calc(100%_-_2.5rem),transparent)] pb-10'>
      {mobileContent}
    </div>
  )
}

function renderDesktop<TData>(
  props: DataTablePageProps<TData>,
  showMobile: boolean
): React.ReactNode {
  if (showMobile) {
    return null
  }

  const isFetchingOnly = props.isFetching && !props.isLoading
  const fixedHeight = props.fixedHeight !== false

  return (
    <DataTableView
      table={props.table}
      isLoading={props.isLoading}
      emptyTitle={props.emptyTitle}
      emptyDescription={props.emptyDescription}
      emptyIcon={props.emptyIcon}
      emptyAction={props.emptyAction}
      filteredEmptyTitle={props.filteredEmptyTitle}
      filteredEmptyDescription={props.filteredEmptyDescription}
      skeletonKeyPrefix={props.skeletonKeyPrefix}
      renderRow={props.renderRow}
      applyHeaderSize={props.applyHeaderSize}
      splitHeader={fixedHeight}
      tableContainerClassName={fixedHeight ? 'h-full min-h-0' : undefined}
      tableHeaderClassName={cn(
        fixedHeight && '[background-color:var(--table-header)]',
        props.tableHeaderClassName
      )}
      getColumnClassName={props.getColumnClassName}
      pinnedColumns={props.pinnedColumns}
      containerClassName={cn(
        fixedHeight && 'min-h-0 flex-1',
        'transition-opacity duration-150',
        // Subtle dim only while background-refetching; never block pointer
        // events — rows stay rendered (placeholderData) and interactive.
        isFetchingOnly && 'opacity-80',
        props.tableClassName
      )}
      getRowClassName={(row) =>
        props.getRowClassName?.(row, { isMobile: false })
      }
    />
  )
}

// Local fallback for PageFooterPortal — @/components/layout/components/page-footer
// is not yet present in metapi-go. Renders pagination inline below the table.
// When the shared portal lands, replace this with the real import so pagination
// can be portalled into a sticky page-footer slot.
function PageFooterPortal({ children }: { children: React.ReactNode }) {
  return <div className='pt-2'>{children}</div>
}
