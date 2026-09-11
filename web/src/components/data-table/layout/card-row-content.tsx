// metapi-go/data-table — CardRowContent: one row rendered as a card.
//
// The mobile list and the desktop card grid differ only in their container (a
// single bordered list versus a responsive grid), so the per-row content lives
// here and both render the same thing. That is the whole reason this module
// exists: a card that looks different on a phone and on a narrow desktop window
// is a bug nobody would think to file.
//
// Two layouts, chosen per table by `tableHasCompactMeta`:
//
//   compact    a column declared `mobileTitle` / `mobileBadge`, so the card gets
//              a header row — title left, badge right — and the rest of the
//              fields wrap into two columns beneath it, with the action menu on
//              its own bottom-right row.
//   condensed  no column declared either, so every field renders as a
//              label/value line. Used by tables that were never given a card
//              design and still have to be readable on a phone.
//
// `mobileHidden` is honoured by both, and `mobileOrder` decides field order.
// Badges render through `StatusBadgeTypeContext` as `text` rather than as chips:
// a chip's background and border cost more than a card row has room for.

import type { Cell, Row } from '@tanstack/react-table'

import { StatusBadgeTypeContext } from '../core/status-badge'
import { getCellLabel, renderCellContent } from './card-cell-utils'

type CardCells<TData> = {
  actions: Cell<TData, unknown> | undefined
  badge: Cell<TData, unknown> | undefined
  fields: Cell<TData, unknown>[]
  title: Cell<TData, unknown> | undefined
}

/**
 * Splits a row's visible cells into the roles the card layout needs.
 *
 * The selection column is dropped (the card is not selectable in place) and the
 * action menu is pulled out of the field flow so it can sit bottom-right. Title
 * and badge are only looked for in the compact layout: in the condensed one they
 * are ordinary fields, which is what makes the two layouts share this function.
 */
function cardCells<TData>(row: Row<TData>, compact: boolean): CardCells<TData> {
  const cells = row
    .getVisibleCells()
    .filter((cell) => cell.column.id !== 'select')
  const actions = cells.find((cell) => cell.column.id === 'actions')
  const title = compact
    ? cells.find((cell) => cell.column.columnDef.meta?.mobileTitle)
    : undefined
  const badge = compact
    ? cells.find((cell) => cell.column.columnDef.meta?.mobileBadge)
    : undefined

  const fields = inMobileOrder(
    cells.filter(
      (cell) =>
        cell !== title &&
        cell !== badge &&
        cell !== actions &&
        !cell.column.columnDef.meta?.mobileHidden
    )
  )

  return { actions, badge, fields, title }
}

/**
 * Fields by `mobileOrder`, unordered ones last. `Array.prototype.sort` is
 * stable, so "no order declared" keeps the table's own column order rather than
// reshuffling it.
 */
function inMobileOrder<TData>(cells: Cell<TData, unknown>[]) {
  return [...cells].sort((a, b) => {
    const left = a.column.columnDef.meta?.mobileOrder
    const right = b.column.columnDef.meta?.mobileOrder

    if (left == null && right == null) return 0
    if (left == null) return 1
    if (right == null) return -1
    return left - right
  })
}

/**
 * Header row plus a two-column field grid. The grid is what keeps a card with
 * six fields short: fields wrap into columns instead of squeezing onto one line.
 */
function CompactContent<TData>({ row }: { row: Row<TData> }) {
  const { actions, badge, fields, title } = cardCells(row, true)

  return (
    <>
      <div className='flex items-center justify-between gap-2'>
        {title && (
          <div className='min-w-0 flex-1 text-sm font-medium [&_[data-slot=status-badge]]:max-w-full [&_[data-slot=status-badge]]:whitespace-normal'>
            {renderCellContent(title)}
          </div>
        )}
        {badge && (
          <div className='flex-none [&_[data-slot=status-badge]]:max-w-none'>
            {renderCellContent(badge)}
          </div>
        )}
      </div>

      {fields.length > 0 && (
        <div className='mt-1.5 grid grid-cols-2 gap-x-3 gap-y-1.5'>
          {fields.map((cell) => {
            const label = getCellLabel(cell)

            return (
              <div key={cell.id} className='min-w-0 flex-1 overflow-hidden'>
                {label && (
                  <div className='text-muted-foreground mb-0.5 text-[10px] leading-none select-none'>
                    {label}
                  </div>
                )}
                <div className='min-w-0 overflow-hidden text-xs [&_:is([data-slot=badge-cell],[data-slot=provider-badge],[data-slot=status-badge])]:ml-0'>
                  {renderCellContent(cell) ?? '-'}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {actions && (
        <div className='mt-1 -mb-0.5 flex justify-end'>
          {renderCellContent(actions)}
        </div>
      )}
    </>
  )
}

/**
 * One label/value line per field. A field with no label takes the whole width
 * and right-aligns, which is how a lone status or icon column stays readable
 * without an empty label gutter beside it.
 */
function CondensedContent<TData>({ row }: { row: Row<TData> }) {
  const { actions, fields } = cardCells(row, false)

  return (
    <>
      {fields.map((cell) => {
        const label = getCellLabel(cell)

        if (!label) {
          return (
            <div
              key={cell.id}
              className='flex justify-end overflow-hidden [&_:is([data-slot=badge-cell],[data-slot=provider-badge],[data-slot=status-badge])]:ml-0'
            >
              {renderCellContent(cell)}
            </div>
          )
        }

        return (
          <div
            key={cell.id}
            className='flex items-start justify-between gap-2 overflow-hidden'
          >
            <span className='text-muted-foreground shrink-0 text-[10px] font-medium select-none'>
              {label}
            </span>
            <div className='flex min-w-0 flex-1 items-center justify-end overflow-hidden text-xs [&_:is([data-slot=badge-cell],[data-slot=provider-badge],[data-slot=status-badge])]:ml-0'>
              {renderCellContent(cell) ?? '-'}
            </div>
          </div>
        )
      })}

      {actions && (
        <div className='-mb-0.5 flex justify-end pt-0.5'>
          {renderCellContent(actions)}
        </div>
      )}
    </>
  )
}

/**
 * A single row's card content.
 *
 * `compact` is decided once per table by the caller (`tableHasCompactMeta`), not
 * per row: every card in a list has to have the same shape, and recomputing the
 * scan for each row would be pure waste.
 *
 * The badge-type provider wraps the whole card rather than each field, so both
 * layouts declare it once.
 */
export function CardRowContent<TData>({
  row,
  compact,
}: {
  row: Row<TData>
  compact: boolean
}) {
  return (
    <StatusBadgeTypeContext.Provider value='text'>
      {compact ? <CompactContent row={row} /> : <CondensedContent row={row} />}
    </StatusBadgeTypeContext.Provider>
  )
}
