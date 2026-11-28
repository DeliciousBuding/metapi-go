import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DateRangePresets } from '@/components/common/date-range-presets'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { gatewayRequestStatusSchema } from '@/lib/api/gateway-requests'

import type {
  RequestFilters,
  UpdateGatewaySearch,
} from './gateway-request-search'

export function useGatewayRequestToolbar(
  kind: 'usage' | 'access',
  filters: RequestFilters,
  updateUrlState: UpdateGatewaySearch
) {
  const { t } = useTranslation()
  const reset = () =>
    updateUrlState({
      q: '',
      pageIndex: 0,
      filters: { status: 'all', downstreamKeyId: '', from: '', to: '' },
    })
  return {
    searchPlaceholder: t(
      kind === 'access'
        ? 'auditLogs.searchPlaceholder'
        : 'requestLogs.searchPlaceholder'
    ),
    searchDebounceMs: 400,
    additionalSearch: (
      <>
        <Select
          value={filters.status}
          onValueChange={(value) =>
            updateUrlState({
              pageIndex: 0,
              filters: { status: value ?? 'all' },
            })
          }
        >
          <SelectTrigger
            size='sm'
            className='w-44'
            aria-label={t('requestLogs.filterStatus')}
          >
            <SelectValue>
              {(value) =>
                t(
                  value === 'all'
                    ? 'requestLogs.all'
                    : `requestLogs.status.${value}`
                )
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {['all', ...gatewayRequestStatusSchema.options].map((status) => (
              <SelectItem value={status} key={status}>
                {t(
                  status === 'all'
                    ? 'requestLogs.all'
                    : `requestLogs.status.${status}`
                )}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {filters.downstreamKeyId && (
          <Button
            variant='secondary'
            size='sm'
            aria-label={t('proxyLogs.page.filterKeyClear')}
            onClick={() =>
              updateUrlState({
                pageIndex: 0,
                filters: { downstreamKeyId: '' },
              })
            }
          >
            {t('requestLogs.key')} #{filters.downstreamKeyId}
            <X className='size-3.5' />
          </Button>
        )}
      </>
    ),
    expandable: (
      <div className='flex w-full flex-wrap items-center gap-2'>
        <Input
          type='datetime-local'
          className='w-50 max-sm:w-full'
          aria-label={t('proxyLogs.page.startTime')}
          value={filters.from}
          onChange={(event) =>
            updateUrlState({
              pageIndex: 0,
              filters: { from: event.target.value },
            })
          }
        />
        <Input
          type='datetime-local'
          className='w-50 max-sm:w-full'
          aria-label={t('proxyLogs.page.endTime')}
          value={filters.to}
          onChange={(event) =>
            updateUrlState({
              pageIndex: 0,
              filters: { to: event.target.value },
            })
          }
        />
        <DateRangePresets
          onApply={(from, to) =>
            updateUrlState({ pageIndex: 0, filters: { from, to } })
          }
        />
      </div>
    ),
    hasExpandedActiveFilters: !!filters.from || !!filters.to,
    hasAdditionalFilters:
      filters.status !== 'all' ||
      !!filters.downstreamKeyId ||
      !!filters.from ||
      !!filters.to,
    onReset: reset,
  }
}
