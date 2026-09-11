// metapi-go/data-table — DataTableBulkActions: the floating bar that appears when
// rows are selected.
//
// It replaces the toolbar rather than sitting under it, because a selection
// changes what every action on the page means: "delete" now deletes *these*
// rows. Keeping both on screen invites acting on the wrong scope.
//
// Three accessibility contracts, each with a reason:
//   - it is a `role="toolbar"` with arrow / Home / End roving focus, because a
//     row of buttons reached by Tab one at a time is a dozen stops per table;
//   - Escape clears the selection, unless the keystroke belongs to an open
//     dropdown inside the bar — that menu closes first and the selection survives;
//   - selection changes are announced through a polite live region, since the bar
//     appearing is a visual event a screen reader would otherwise miss.

import type { Table } from '@tanstack/react-table'
import { X } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

/** How long an announcement stays in the live region before it is cleared. */
const ANNOUNCEMENT_MS = 3000

/**
 * Elements inside the bar that swallow Escape themselves. Matched on the
 * data-slot the dropdown primitives stamp, because by the time this handler runs
 * the menu has already closed and its state cannot be asked.
 */
const DROPDOWN_SELECTOR =
  '[data-slot="dropdown-menu-trigger"], [data-slot="dropdown-menu-content"]'

type DataTableBulkActionsProps<TData> = {
  table: Table<TData>
  /** Singular entity name; the plural is built by the i18n plural key. */
  entityName: string
  /** The action buttons this bar exists to hold. */
  children: React.ReactNode
}

export function DataTableBulkActions<TData>({
  table,
  entityName,
  children,
}: DataTableBulkActionsProps<TData>) {
  const { t } = useTranslation()
  const selectedCount = table.getFilteredSelectedRowModel().rows.length
  const toolbarRef = useRef<HTMLDivElement>(null)
  const descriptionId = useId()
  const [announcement, setAnnouncement] = useState('')

  const entityPlural = t('dataTable.bulkActions.entityPlural', {
    count: selectedCount,
    entityName,
  })

  // Announce selection changes, then clear: a live region that keeps its text
  // will not re-announce the same count, so the next selection of the same size
  // would go unspoken. The state write is the point of the effect (it owns the
  // timer that undoes it), not a render value that should have been derived.
  useEffect(() => {
    if (selectedCount === 0) return

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setAnnouncement(
      t('dataTable.bulkActions.announcement', {
        count: selectedCount,
        entityName: entityPlural,
      })
    )

    const timer = setTimeout(() => setAnnouncement(''), ANNOUNCEMENT_MS)
    return () => clearTimeout(timer)
  }, [entityPlural, selectedCount, t])

  if (selectedCount === 0) {
    return null
  }

  const clearSelection = () => table.resetRowSelection()

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'Escape') {
      // A keystroke from inside an open dropdown belongs to the dropdown: that
      // menu closes first and the selection survives to be acted on.
      const target = event.target instanceof Element ? event.target : null
      const insideDropdown =
        (target?.closest(DROPDOWN_SELECTOR) ?? null) !== null ||
        (document.activeElement?.closest(DROPDOWN_SELECTOR) ?? null) !== null
      if (insideDropdown) return

      event.preventDefault()
      clearSelection()
      return
    }

    // Queried on the keystroke rather than kept in a ref: the buttons are the
    // caller's children, so any ref snapshot would have to be refreshed after
    // every render to stay true.
    const buttons = [
      ...(toolbarRef.current?.querySelectorAll<HTMLButtonElement>('button') ??
        []),
    ]
    if (buttons.length === 0) return

    // -1 when focus is on the toolbar itself rather than on a button, which the
    // roving convention reads as "before the first": right enters at the start,
    // left wraps to the end.
    const current = buttons.indexOf(document.activeElement as HTMLButtonElement)

    switch (event.key) {
      case 'ArrowRight':
        event.preventDefault()
        buttons[(current + 1) % buttons.length]?.focus()
        break
      case 'ArrowLeft':
        event.preventDefault()
        buttons[current <= 0 ? buttons.length - 1 : current - 1]?.focus()
        break
      case 'Home':
        event.preventDefault()
        buttons[0]?.focus()
        break
      case 'End':
        event.preventDefault()
        buttons.at(-1)?.focus()
        break
    }
  }

  return (
    <>
      <div
        aria-live='polite'
        aria-atomic='true'
        className='sr-only'
        role='status'
      >
        {announcement}
      </div>

      <div
        ref={toolbarRef}
        role='toolbar'
        aria-label={t('dataTable.bulkActions.toolbarAria', {
          count: selectedCount,
          entityName: entityPlural,
        })}
        aria-describedby={descriptionId}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        className='focus-visible:ring-focus-ring fixed bottom-6 left-1/2 z-50 max-w-[calc(100vw_-_0.5rem)] -translate-x-1/2 rounded-xl transition-transform delay-100 duration-300 ease-out hover:scale-105 focus-visible:ring-2 focus-visible:outline-none'
      >
        <div className='bg-background/95 supports-[backdrop-filter]:bg-background/60 flex max-w-full flex-wrap items-center gap-x-1.5 gap-y-1 rounded-xl border p-1.5 shadow-xl backdrop-blur-lg sm:gap-x-2 sm:p-2'>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='outline'
                  size='icon'
                  onClick={clearSelection}
                  className='size-6'
                  aria-label={t('Clear selection')}
                  title={t('Clear selection (Escape)')}
                />
              }
            >
              <X />
              <span className='sr-only'>{t('Clear selection')}</span>
            </TooltipTrigger>
            <TooltipContent>
              <p>{t('Clear selection (Escape)')}</p>
            </TooltipContent>
          </Tooltip>

          <Separator
            className='h-5 max-sm:hidden'
            orientation='vertical'
            aria-hidden='true'
          />

          <div className='flex items-center gap-x-1 text-sm' id={descriptionId}>
            <Badge
              variant='default'
              className='min-w-8'
              aria-label={t('dataTable.bulkActions.selectedAria', {
                count: selectedCount,
              })}
            >
              {selectedCount}
            </Badge>{' '}
            <span className='hidden sm:inline'>
              {entityPlural} {t('dataTable.bulkActions.selected')}
            </span>{' '}
          </div>

          <Separator
            className='h-5 max-sm:hidden'
            orientation='vertical'
            aria-hidden='true'
          />

          {children}
        </div>
      </div>
    </>
  )
}
