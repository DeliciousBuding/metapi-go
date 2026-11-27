import { RotateCcw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'

import {
  type readRequestSearch,
  type requestSearchHref,
  requestStatuses,
  quotaStates,
  type RequestStatus,
  type QuotaState,
} from '../request-search'

export function RequestToolbar({
  q,
  filters,
  hasFilters,
  onSearchChange,
  updateUrlState,
}: {
  q: string
  filters: ReturnType<typeof readRequestSearch>['filters']
  hasFilters: boolean
  onSearchChange: (value: string) => void
  updateUrlState: (update: Parameters<typeof requestSearchHref>[0]) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap items-center gap-2'>
      <div className='relative min-w-0 flex-1 basis-60'>
        <Search
          aria-hidden='true'
          className='text-muted-foreground pointer-events-none absolute start-3 top-2 size-4'
        />
        <Input
          className='h-8 ps-9'
          aria-label={t('userPortal.requestFilters.search')}
          placeholder={t('userPortal.requestFilters.search')}
          maxLength={200}
          value={q}
          onChange={(event) => onSearchChange(event.target.value)}
        />
      </div>
      <Select
        value={filters.status ?? 'all'}
        onValueChange={(value) =>
          updateUrlState({
            filters: {
              status:
                value === 'all'
                  ? undefined
                  : (value as RequestStatus | undefined),
            },
            pageIndex: 0,
          })
        }
      >
        <SelectTrigger
          size='sm'
          aria-label={t('userPortal.requestFilters.status')}
        >
          <SelectValue>
            {filters.status
              ? t('userPortal.status.' + filters.status)
              : t('userPortal.requestFilters.allResults')}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='all'>
            {t('userPortal.requestFilters.allResults')}
          </SelectItem>
          {requestStatuses.map((status) => (
            <SelectItem key={status} value={status}>
              {t('userPortal.status.' + status)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={filters.quotaState ?? 'all'}
        onValueChange={(value) =>
          updateUrlState({
            filters: {
              quotaState:
                value === 'all' ? undefined : (value as QuotaState | undefined),
            },
            pageIndex: 0,
          })
        }
      >
        <SelectTrigger
          size='sm'
          aria-label={t('userPortal.requestFilters.quota')}
        >
          <SelectValue>
            {filters.quotaState
              ? t('userPortal.requestFilters.quotaStates.' + filters.quotaState)
              : t('userPortal.requestFilters.allQuota')}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='all'>
            {t('userPortal.requestFilters.allQuota')}
          </SelectItem>
          {quotaStates.map((status) => (
            <SelectItem key={status} value={status}>
              {t('userPortal.requestFilters.quotaStates.' + status)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {hasFilters && (
        <Button
          size='sm'
          variant='ghost'
          onClick={() =>
            updateUrlState({
              q: '',
              filters: { status: undefined, quotaState: undefined },
              pageIndex: 0,
            })
          }
        >
          <RotateCcw aria-hidden='true' />
          {t('userPortal.requestFilters.clear')}
        </Button>
      )}
    </div>
  )
}
