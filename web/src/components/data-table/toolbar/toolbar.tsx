// metapi-go/data-table — DataTableToolbar: the filter row above a list page.
//
// One `flex-wrap` row, no panel chrome: the search input, any page-supplied
// controls and the column filter chips flow left, the action cluster hugs the
// right edge via `ms-auto` and wraps to its own line when the filters fill the
// row. Visual hierarchy comes from whitespace and the adjacent table border.
//
// The toolbar owns the search *draft*, not the search value: commits are
// debounced, so between keystrokes the input shows what the user typed while
// the table still holds the last settled value.
import type { Table } from '@tanstack/react-table'
import { ChevronDown, X as Cross2Icon } from 'lucide-react'
import * as React from 'react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import { useDebounce } from '../hooks/use-debounce'
import { DataTableFacetedFilter } from './faceted-filter'
import { DataTableViewOptions } from './view-options'

type FilterDef = {
  columnId: string
  title: string
  options: {
    label: string
    value: string
    icon?: React.ComponentType<{ className?: string }>
    iconNode?: React.ReactNode
    count?: number
  }[]
  singleSelect?: boolean
}

/**
 * What the user has typed but not yet committed. `baseValue` is the committed
 * value it was typed against, which is how a draft tells "still mine" apart
 * from "the table moved on without me" (Reset, a URL sync, another filter).
 */
type SearchDraft = {
  baseValue: string
  value: string
}

const DEFAULT_SEARCH_DEBOUNCE_MS = 300

export type DataTableToolbarProps<TData> = {
  table: Table<TData>
  /**
   * Placeholder for the search input. Defaults to `t('Filter...')`.
   */
  searchPlaceholder?: string
  /**
   * Delay before a keystroke reaches the table's global filter. Defaults to
   * 300ms: filtering re-runs over the whole page of rows, so a page that
   * filters as you type should not pay for that per keystroke.
   */
  searchDebounceMs?: number
  /**
   * Column filter chips (faceted multi-select / single-select). A chip whose
   * `columnId` does not resolve to a column is skipped rather than rendered
   * empty.
   */
  filters?: FilterDef[]
  /**
   * Extra controls in the filter flow, after the search input — e.g. the
   * status `<Select>` and date range the proxy-log page drives from the URL.
   */
  additionalSearch?: ReactNode
  /**
   * Whether filters this toolbar does not own (`additionalSearch`,
   * `expandable`, or anything the page keeps in the URL) are active. Decides
   * Reset visibility once the table itself reports no filter.
   */
  hasAdditionalFilters?: boolean
  /**
   * Called after Reset has cleared the table's own filters, so the page can
   * clear the state it owns.
   */
  onReset?: () => void
  /**
   * Extra filter inputs behind the Expand/Collapse toggle. They join the same
   * wrapping flow when expanded.
   */
  expandable?: ReactNode
  /**
   * Highlights the collapsed toggle when an `expandable` input holds a value,
   * so a hidden filter is still visible as active.
   */
  hasExpandedActiveFilters?: boolean
  /**
   * Action buttons rendered before Reset in the right-hand cluster — the
   * page's primary entry points (Add site, Import accounts, …).
   */
  preActions?: ReactNode
  /**
   * Extra control rendered after Reset, before the column-visibility menu.
   */
  viewToggle?: ReactNode
}

export function DataTableToolbar<TData>(props: DataTableToolbarProps<TData>) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const [draft, setDraft] = useState<SearchDraft | null>(null)
  // True between compositionstart and compositionend. An IME builds a string
  // across several keystrokes; committing (or debouncing) the intermediate
  // text would filter on half-typed pinyin.
  const [composing, setComposing] = useState(false)

  const committed =
    (props.table.getState().globalFilter as string | undefined) ?? ''
  const liveDraft =
    draft && (composing || draft.baseValue === committed) ? draft : null
  const searchValue = liveDraft?.value ?? committed
  const debouncedSearchValue = useDebounce(
    searchValue,
    props.searchDebounceMs ?? DEFAULT_SEARCH_DEBOUNCE_MS
  )

  const isFiltered =
    (props.table.getState().columnFilters ?? []).length > 0 ||
    committed !== '' ||
    props.hasAdditionalFilters === true

  const commitSearchValue = React.useCallback(
    (value: string) => {
      if (value !== committed) {
        props.table.setGlobalFilter(value)
      }
    },
    [committed, props.table]
  )

  // The debounce has settled — the debounced value caught up with what the
  // input shows — and the table has not been told yet: publish it.
  React.useEffect(() => {
    if (composing || debouncedSearchValue !== searchValue) {
      return
    }
    commitSearchValue(debouncedSearchValue)
  }, [commitSearchValue, composing, debouncedSearchValue, searchValue])

  const handleSearchChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setDraft({ baseValue: committed, value: event.target.value })
  }

  const handleCompositionEnd = (
    event: React.CompositionEvent<HTMLInputElement>
  ) => {
    setComposing(false)
    setDraft({ baseValue: committed, value: event.currentTarget.value })
  }

  // Enter commits the draft instead of waiting out the debounce, so
  // "type → Enter" behaves like an explicit search.
  const handleSearchKeyDown = (
    event: React.KeyboardEvent<HTMLInputElement>
  ) => {
    if (event.key !== 'Enter') return
    event.preventDefault()
    setComposing(false)
    commitSearchValue(searchValue)
  }

  const clearSearchValue = () => {
    setComposing(false)
    setDraft(null)
    commitSearchValue('')
  }

  const handleReset = () => {
    clearSearchValue()
    props.table.resetColumnFilters()
    props.onReset?.()
  }

  const placeholder = props.searchPlaceholder ?? t('Filter...')
  const hasExpandable = props.expandable != null

  return (
    <div className='flex flex-wrap items-center gap-2 sm:gap-3'>
      <div className='relative w-full sm:w-[200px] lg:w-[240px]'>
        <Input
          aria-label={placeholder}
          placeholder={placeholder}
          value={searchValue}
          onChange={handleSearchChange}
          onKeyDown={handleSearchKeyDown}
          onCompositionStart={() => setComposing(true)}
          onCompositionEnd={handleCompositionEnd}
          className={cn('w-full', searchValue !== '' && 'pe-8')}
        />
        {searchValue !== '' && (
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            aria-label={t('Clear search')}
            onClick={clearSearchValue}
            className='text-muted-foreground hover:text-foreground absolute top-1/2 right-1 -translate-y-1/2'
          >
            <Cross2Icon className='size-3.5' />
          </Button>
        )}
      </div>

      {props.additionalSearch}

      {(props.filters ?? []).map((filter) => {
        const column = props.table.getColumn(filter.columnId)
        if (!column) return null
        return (
          <DataTableFacetedFilter
            key={filter.columnId}
            column={column}
            title={filter.title}
            options={filter.options}
            singleSelect={filter.singleSelect}
          />
        )
      })}

      {expanded && props.expandable}

      {/* `shrink-0` is deliberately absent from the cluster: an over-long
          `viewToggle` (the routes page's "show zero-channel models" switch)
          must not push the View Options button past the viewport. With
          `min-w-0` its label truncates and the button wraps to its own tight
          line instead of being clipped by the page's overflow-x hidden. */}
      <div className='ms-auto flex min-w-0 flex-wrap items-center justify-end gap-1.5 sm:gap-2'>
        {props.preActions}
        {isFiltered && (
          <Button
            variant='ghost'
            onClick={handleReset}
            className='text-muted-foreground hover:text-foreground gap-1 px-2'
          >
            {t('Reset')}
            <Cross2Icon />
          </Button>
        )}
        {props.viewToggle}
        <DataTableViewOptions table={props.table} />
        {hasExpandable && (
          <Button
            variant='ghost'
            onClick={() => setExpanded((previous) => !previous)}
            aria-expanded={expanded}
            className={cn(
              'text-muted-foreground hover:text-foreground gap-1 px-2',
              props.hasExpandedActiveFilters &&
                !expanded &&
                'text-primary hover:text-primary'
            )}
          >
            {expanded ? t('Collapse') : t('Expand')}
            <ChevronDown
              className={cn(
                'size-3.5 transition-transform duration-200',
                expanded && 'rotate-180'
              )}
            />
          </Button>
        )}
      </div>
    </div>
  )
}
