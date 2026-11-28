import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import { Input } from '@/components/ui/input'

type Filters = Record<string, never>

function readSearch(search: string): UrlTableState<Filters> {
  const params = new URLSearchParams(search)
  const page = Number(params.get('page') ?? 1)
  return {
    q: (params.get('q') ?? '').slice(0, 200),
    pageIndex:
      Number.isSafeInteger(page) && page > 0 && page <= 1_000_000
        ? page - 1
        : 0,
    pageSize: 20,
    sorting: [],
    filters: {},
  }
}
function buildHref(
  next: UrlTableStateUpdate<Filters>,
  search = window.location.search
) {
  const state = { ...readSearch(search), ...next }
  const params = new URLSearchParams(search)
  params.set('view', 'models')
  params.set('page', String(state.pageIndex + 1))
  if (state.q) params.set('q', state.q)
  else params.delete('q')
  return '/user?' + params.toString()
}
export function useModelSearch() {
  return useUrlTableState<Filters>({
    basePath: '/user',
    read: readSearch,
    buildHref,
    resetPageIndexOnFilterChange: true,
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
  })
}

export function ModelSearchToolbar({
  search,
  count,
  onSearch,
}: {
  search: string
  count: number
  onSearch: (value: string) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap items-center gap-3'>
      <div className='relative min-w-0 flex-1'>
        <Search
          className='text-muted-foreground pointer-events-none absolute start-3 top-2.5 size-4'
          aria-hidden='true'
        />
        <Input
          aria-label={t('userPortal.catalog.search')}
          placeholder={t('userPortal.catalog.search')}
          value={search}
          maxLength={200}
          onChange={(e) => onSearch(e.target.value)}
          className='ps-9'
        />
      </div>
      <span className='text-muted-foreground text-sm tabular-nums'>
        {t('userPortal.catalog.count', { count })}
      </span>
    </div>
  )
}
