import { Search as SearchIcon, type LucideIcon } from 'lucide-react'
import type { ElementType } from 'react'
import { useTranslation } from 'react-i18next'

import { CommandGroup, CommandItem, CommandList } from '@/components/ui/command'
import { Spinner } from '@/components/ui/spinner'

import type { SearchActionId } from './lib/search-actions'

export const MAX_ITEMS_PER_GROUP = 8
export type SearchTarget = { to: string; search?: Record<string, unknown> }
export type SearchItem = {
  key: string
  label: string
  description: string | null
  icon?: ElementType
} & ({ target: SearchTarget } | { action: SearchActionId })

export type SearchGroup = {
  key: string
  heading: string
  icon: LucideIcon
  items: SearchItem[]
}

export function SearchResultList({
  query,
  groups,
  isSearching,
  onNavigate,
  onAction,
}: {
  query: string
  groups: SearchGroup[]
  isSearching: boolean
  onNavigate: (target: SearchTarget) => void
  onAction: (id: SearchActionId) => void
}) {
  const { t } = useTranslation()
  const showEmpty = query.length > 0 && !isSearching && groups.length === 0
  return (
    <CommandList className='max-h-[min(60svh,28rem)]'>
      {isSearching && (
        <div className='text-muted-foreground flex items-center justify-center gap-2 px-2 py-4 text-sm'>
          <Spinner />
          <span>{t('search.searching')}</span>
        </div>
      )}
      {showEmpty && (
        <div className='flex flex-col items-center gap-1 px-4 py-10 text-center'>
          <SearchIcon className='text-muted-foreground/60 size-5' />
          <p className='text-sm font-medium'>{t('search.noResults.title')}</p>
          <p className='text-muted-foreground text-xs'>
            {t('search.noResults.description', { query })}
          </p>
        </div>
      )}
      {groups.map((group) => (
        <CommandGroup key={group.key} heading={group.heading}>
          {group.items.map((item) => {
            const ItemIcon = item.icon ?? group.icon
            return (
              <CommandItem
                key={item.key}
                value={item.key}
                onSelect={() =>
                  'action' in item
                    ? onAction(item.action)
                    : onNavigate(item.target)
                }
              >
                <ItemIcon className='text-muted-foreground size-4 shrink-0' />
                <span className='flex min-w-0 flex-1 flex-col'>
                  <span className='truncate'>{item.label}</span>
                  {item.description ? (
                    <span className='text-muted-foreground truncate text-xs'>
                      {item.description}
                    </span>
                  ) : null}
                </span>
              </CommandItem>
            )
          })}
        </CommandGroup>
      ))}
    </CommandList>
  )
}
