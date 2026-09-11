// metapi-go/data-table — DataTableFacetedFilter: one column's filter popover.
//
// A checkbox list in a popover rather than a native multi-select, because the
// trigger has to say what is currently filtered: up to two selected values it
// names them, above that it collapses to a count. A native control can do one or
// the other, not both, and a filter whose state is invisible until you open it
// is how a table ends up silently showing a subset.
//
// `singleSelect` turns the same UI into a one-of picker (re-selecting the current
// value clears it) for columns where two values at once mean nothing.
//
// The count beside an option comes from the caller when the caller knows it
// (`option.count`) and from TanStack's faceting otherwise. When neither knows,
// nothing renders: "0 matching rows" and "not counted" are different claims.

import type { Column } from '@tanstack/react-table'
import { Check as CheckIcon, PlusCircle as PlusCircledIcon } from 'lucide-react'
import type { ComponentType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

type FacetOption = {
  label: string
  value: string
  /** Icon component to render before the label. */
  icon?: ComponentType<{ className?: string }>
  /** Pre-rendered icon; wins over `icon` when both are given. */
  iconNode?: ReactNode
  /** Caller-supplied count; preferred over the column's facet count. */
  count?: number
}

type DataTableFacetedFilterProps<TData, TValue> = {
  column?: Column<TData, TValue>
  title?: string
  options: FacetOption[]
  /** Allow at most one selected value instead of a set. */
  singleSelect?: boolean
}

/** Above this many selections the trigger shows a count instead of the labels. */
const NAMED_SELECTION_LIMIT = 2

const CHECK_BOX_CLASS =
  'border-primary flex size-4 items-center justify-center rounded-sm border'

export function DataTableFacetedFilter<TData, TValue>({
  column,
  title,
  options,
  singleSelect = false,
}: DataTableFacetedFilterProps<TData, TValue>) {
  const { t } = useTranslation()
  const facets = column?.getFacetedUniqueValues() ?? new Map<unknown, number>()
  const selectedValues = new Set(
    column?.getFilterValue() as string[] | undefined
  )

  const toggle = (value: string) => {
    const next = nextSelection(selectedValues, value, singleSelect)
    column?.setFilterValue(next.length > 0 ? next : undefined)
  }

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button variant='outline' size='sm' className='h-8 border-dashed' />
        }
      >
        <PlusCircledIcon className='size-4' />
        {title}
        {selectedValues.size > 0 && (
          <>
            <Separator orientation='vertical' className='mx-2 h-4' />
            {/* The count badge is the narrow-viewport form of the same statement. */}
            <Badge
              variant='secondary'
              className='rounded-sm px-1 font-normal lg:hidden'
            >
              {selectedValues.size}
            </Badge>
            <div className='hidden space-x-1 lg:flex'>
              {selectedValues.size > NAMED_SELECTION_LIMIT ? (
                <Badge
                  variant='secondary'
                  className='rounded-sm px-1 font-normal'
                >
                  {selectedValues.size} {t('selected')}
                </Badge>
              ) : (
                options
                  .filter((option) => selectedValues.has(option.value))
                  .map((option) => (
                    <Badge
                      variant='secondary'
                      key={option.value}
                      className='rounded-sm px-1 font-normal'
                    >
                      {option.label}
                    </Badge>
                  ))
              )}
            </div>
          </>
        )}
      </PopoverTrigger>
      <PopoverContent className='max-w-[360px] min-w-[200px] p-0' align='start'>
        <Command>
          <CommandInput placeholder={title} />
          <CommandList>
            <CommandEmpty>{t('No results found.')}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => {
                const isSelected = selectedValues.has(option.value)

                return (
                  <CommandItem
                    key={option.value}
                    onSelect={() => toggle(option.value)}
                  >
                    <div
                      className={cn(
                        CHECK_BOX_CLASS,
                        isSelected
                          ? 'bg-primary text-primary-foreground'
                          : 'opacity-50 [&_svg]:invisible'
                      )}
                    >
                      <CheckIcon className='text-background h-4 w-4' />
                    </div>
                    <OptionIcon option={option} />
                    <span
                      className='min-w-0 flex-1 truncate'
                      title={option.label}
                    >
                      {option.label}
                    </span>
                    <OptionCount
                      count={option.count}
                      facet={facets.get(option.value)}
                    />
                  </CommandItem>
                )
              })}
            </CommandGroup>
            {selectedValues.size > 0 && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem
                    onSelect={() => column?.setFilterValue(undefined)}
                    className='justify-center text-center'
                  >
                    {t('Clear filters')}
                  </CommandItem>
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

/** The option's own glyph. A pre-rendered node wins over an icon component. */
function OptionIcon({ option }: { option: FacetOption }) {
  if (option.iconNode) {
    return (
      <span className='text-muted-foreground flex size-4 items-center justify-center'>
        {option.iconNode}
      </span>
    )
  }

  const Icon = option.icon
  return Icon ? <Icon className='text-muted-foreground size-4' /> : null
}

/** Rows matching the option, right-aligned in a fixed-width gutter. */
function OptionCount({ count, facet }: { count?: number; facet?: number }) {
  if (typeof count === 'number') {
    return (
      <span className='text-muted-foreground ms-auto flex h-4 min-w-4 items-center justify-center font-mono text-xs'>
        {count}
      </span>
    )
  }
  // A facet count of 0 is not rendered: an option nothing matches is still
  // selectable, and a printed 0 reads as "this filter is broken".
  if (!facet) return null

  return (
    <span className='ms-auto flex h-4 w-4 items-center justify-center font-mono text-xs'>
      {facet}
    </span>
  )
}

/**
 * The selection after `value` is picked. Single-select toggles between that one
 * value and nothing; multi-select adds or removes it from the set.
 */
function nextSelection(
  selected: Set<string>,
  value: string,
  singleSelect: boolean
): string[] {
  if (singleSelect) {
    return selected.has(value) ? [] : [value]
  }

  const next = new Set(selected)
  if (next.has(value)) next.delete(value)
  else next.add(value)

  return [...next]
}
