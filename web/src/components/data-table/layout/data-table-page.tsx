// metapi-go/data-table — DataTablePage: the list-page composition.
//
// The structure every list page in the app has, in one place: toolbar → desktop
// table *or* mobile card list → pagination, with the loading skeleton, the empty
// state, the error banner and the bulk-action bar all wired to the same table.
//
// A page hands over a TanStack table (from `useDataTable`) plus its copy, and
// this file owns the rest — including the responsive split, so no page has to
// call `useMediaQuery` itself. The two escape hatches are narrow on purpose:
// `toolbarProps` configures the standard toolbar, `renderRow` replaces a row.
// Between them they cover expanded rows and row navigation, which is everything
// the current pages need; a third hatch would only be a way to fork the layout.
import type { Row, Table as TanstackTable } from '@tanstack/react-table'
import * as React from 'react'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useMediaQuery } from '@/hooks/use-media-query'
import { TABLE_MOBILE_MEDIA_QUERY } from '@/lib/breakpoints'
import { cn } from '@/lib/utils'

import { DataTableView } from '../core/data-table-view'
import { DataTablePagination } from '../core/pagination'
import type { DataTableRenderRowHelpers } from '../core/types'
import { DataTableToolbar } from '../toolbar/toolbar'
import { MobileCardList } from './mobile-card-list'

/**
 * Pass-through configuration for the default {@link DataTableToolbar}.
 */
type DataTablePageToolbarProps<TData> = Omit<
  React.ComponentProps<typeof DataTableToolbar<TData>>,
  'table'
>

/**
 * The shared list-page error contract: hand the list query's error to the page
 * and the banner renders through {@link QueryErrorBanner}. The union makes
 * `errorMessageKey` a compile-time requirement next to `error`, so a page cannot
 * render the banner with no message to show.
 */
type DataTablePageErrorProps =
  | {
      error?: null
      errorMessageKey?: undefined
      onErrorRetry?: undefined
      isErrorRetrying?: undefined
      errorPlacement?: undefined
    }
  | {
      /**
       * List-query error. Non-null renders the shared QueryErrorBanner:
       * 'replace' (default) swaps the whole table region for the banner;
       * 'inline' keeps stale rows visible under the banner.
       */
      error: Error | null
      /**
       * i18n key whose template interpolates `{{message}}`
       * (e.g. `accounts.page.loadError`).
       */
      errorMessageKey: string
      /** Retry handler for the banner — typically `() => query.refetch()`. */
      onErrorRetry?: () => void
      /** True while the retry request is in flight. */
      isErrorRetrying?: boolean
      /**
       * 'replace' (default): banner replaces toolbar/table/pagination.
       * 'inline': banner renders above the table so placeholderData rows
       * stay visible (e.g. oauth, proxy-logs).
       */
      errorPlacement?: 'replace' | 'inline'
    }

export type DataTablePageProps<TData> = DataTablePageErrorProps & {
  /** TanStack table instance, normally the one `useDataTable` returned. */
  table: TanstackTable<TData>

  /** First load — renders the skeleton body / skeleton cards. */
  isLoading?: boolean

  /** Background refetch — dims the table without removing rows. */
  isFetching?: boolean

  /** Empty-state copy, used by both the desktop table and the mobile list. */
  emptyTitle?: string
  emptyDescription?: string

  /** Empty-state extra content — e.g. a "Create" button under the message. */
  emptyAction?: React.ReactNode

  /**
   * Configuration for the default {@link DataTableToolbar}. Omit it or pass
   * `null` for a page with no toolbar at all.
   */
  toolbarProps?: DataTablePageToolbarProps<TData> | null

  /**
   * Bulk-action bar, typically a wrapped {@link DataTableBulkActions}. It owns
   * its own visibility. Rendered on mobile too: the card list surfaces row
   * selection whenever the table has a `select` column, so bulk
   * enable/disable/delete stay reachable on touch.
   */
  bulkActions?: React.ReactNode

  /**
   * Replaces the default desktop row mapping — for expanded rows, aggregate
   * rows, or click-on-row navigation. The helper carries the pinned-column
   * class resolver, so a custom row still looks pinned where a default one
   * would. Mobile keeps rendering cards either way.
   */
  renderRow?: (
    row: Row<TData>,
    helpers: DataTableRenderRowHelpers
  ) => React.ReactNode

  /** React key prefix for skeleton rows; distinct per table when two share a page. */
  skeletonKeyPrefix?: string

  /** Class for the toolbar+table column. */
  className?: string

  /**
   * Let the desktop table take the available page height and scroll inside its
   * body while the header stays pinned. Defaults to `true`; a short embedded
   * list (the downstream keys section) turns it off and grows with its rows.
   */
  fixedHeight?: boolean
}

/**
 * ```tsx
 * <DataTablePage
 *   table={table}
 *   isLoading={query.isLoading}
 *   isFetching={query.isFetching}
 *   error={query.error as Error | null}
 *   errorMessageKey='x.page.loadError'
 *   onErrorRetry={() => query.refetch()}
 *   isErrorRetrying={query.isFetching}
 *   emptyTitle={t('No X Found')}
 *   toolbarProps={{ searchPlaceholder: t('Filter...'), filters }}
 *   bulkActions={<MyBulkActions table={table} />}
 * />
 * ```
 */
export function DataTablePage<TData>(props: DataTablePageProps<TData>) {
  // Shared constant (lib/breakpoints): ≤640px renders the MobileCardList;
  // above it the desktop table keeps horizontal scrolling. The 641–767px
  // band intentionally still uses the mobile drawer navigation (768px
  // threshold in useIsMobile) — see lib/breakpoints for the rationale.
  const showMobile = useMediaQuery(TABLE_MOBILE_MEDIA_QUERY)

  const errorBanner = props.error ? (
    <QueryErrorBanner
      error={props.error}
      messageKey={props.errorMessageKey}
      onRetry={props.onErrorRetry}
      isRetrying={props.isErrorRetrying}
    />
  ) : null

  // Replace placement (default): the failed load swaps the whole region —
  // toolbar, table, and pagination — so a stale cache can never read as
  // current data. Inline placement keeps the table under the banner.
  if (errorBanner && props.errorPlacement !== 'inline') {
    return errorBanner
  }

  const fixedHeight = props.fixedHeight !== false

  return (
    <>
      {errorBanner}
      <div
        className={cn(
          fixedHeight
            ? 'flex h-full min-h-0 flex-col gap-2.5 sm:gap-3'
            : 'space-y-2.5 sm:space-y-3',
          props.className
        )}
      >
        {props.toolbarProps && (
          <DataTableToolbar table={props.table} {...props.toolbarProps} />
        )}

        {showMobile ? (
          // Bottom-safe scroll edge: the mask fades the final stretch of the
          // list instead of hard-cutting rows at the container boundary (a 2px
          // sliver of the last row's badge used to read as a stray coloured
          // bar). The gradient is an ALPHA ramp only — it carries no colour, so
          // the OKLCH token system stays the only source of colour (see the
          // no-gradients test allowlist note).
          <div className='min-h-0 flex-1 overflow-y-auto [mask-image:linear-gradient(to_bottom,black_calc(100%_-_2.5rem),transparent)] pb-10'>
            <MobileCardList
              table={props.table}
              isLoading={props.isLoading}
              emptyTitle={props.emptyTitle}
              emptyDescription={props.emptyDescription}
              emptyAction={props.emptyAction}
            />
          </div>
        ) : (
          <DataTableView
            table={props.table}
            isLoading={props.isLoading}
            emptyTitle={props.emptyTitle}
            emptyDescription={props.emptyDescription}
            emptyAction={props.emptyAction}
            skeletonKeyPrefix={props.skeletonKeyPrefix}
            renderRow={props.renderRow}
            splitHeader={fixedHeight}
            containerClassName={cn(
              fixedHeight && 'min-h-0 flex-1',
              'transition-opacity duration-150',
              // Subtle dim only while background-refetching; never block pointer
              // events — rows stay rendered (placeholderData) and interactive.
              props.isFetching && !props.isLoading && 'opacity-80'
            )}
          />
        )}
      </div>

      {props.bulkActions}

      <div className='pt-2'>
        <DataTablePagination table={props.table} />
      </div>
    </>
  )
}
