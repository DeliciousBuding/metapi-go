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
import type { api } from '@/lib/api'

import { PROXY_LOG_STATUS_FILTER_OPTIONS } from '../lib/proxy-logs-schema'
import { LatencyBadge } from './latency-badge'
import type { ProxyLogsUrlFilters } from './use-proxy-logs-url-state'

type UpdateFilters = (filters: Partial<ProxyLogsUrlFilters>) => void
type ProxyLogsMeta = Awaited<ReturnType<typeof api.getProxyLogsMeta>>

export function ProxyLogsQuickFilters({
  status,
  downstreamKeyLabel,
  onChange,
}: {
  status: ProxyLogsUrlFilters['status']
  downstreamKeyLabel: string | null
  onChange: UpdateFilters
}) {
  const { t } = useTranslation()
  return (
    <>
      <Select
        value={status}
        onValueChange={(value) => {
          onChange({ status: value ?? 'all' })
        }}
      >
        <SelectTrigger
          size='sm'
          aria-label={t('proxyLogs.page.filterStatusPlaceholder')}
          className='w-32'
        >
          <SelectValue>
            {(selected) => {
              const option = PROXY_LOG_STATUS_FILTER_OPTIONS.find(
                (item) => item.value === selected
              )
              if (option) return t(option.labelKey)
              return selected ? String(selected) : t('proxyLogs.filter.all')
            }}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {PROXY_LOG_STATUS_FILTER_OPTIONS.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {t(option.labelKey)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {/* Active downstream-key drill-down (entered via deep link from
      the keys page / connect dialog). A chip, not an input: key
      ids are not memorable, and the name is resolved from the
      loaded rows. */}
      {downstreamKeyLabel !== null && (
        <Button
          type='button'
          variant='secondary'
          size='sm'
          className='gap-1'
          aria-label={t('proxyLogs.page.filterKeyClear')}
          onClick={() => onChange({ downstreamKeyId: '' })}
        >
          {t('proxyLogs.page.filterKeyActive', {
            name: downstreamKeyLabel,
          })}
          <X className='size-3.5' />
        </Button>
      )}
    </>
  )
}

export function ProxyLogsAdvancedFilters({
  filters,
  meta,
  onChange,
}: {
  filters: ProxyLogsUrlFilters
  meta: ProxyLogsMeta | undefined
  onChange: UpdateFilters
}) {
  const { t } = useTranslation()
  const { siteId, channelId, client, from, to, latencyMin, latencyMax } =
    filters
  const clientOptions = meta?.clientOptions ?? []
  const siteOptions = meta?.sites ?? []
  const slowOnly = latencyMin === '2000' && !latencyMax
  const hasLatencyFilter = Boolean(latencyMin || latencyMax)
  return (
    <>
      {siteOptions.length > 0 && (
        <Select
          value={siteId || 'all'}
          onValueChange={(value) => {
            onChange({
              siteId: !value || value === 'all' ? '' : value,
            })
          }}
        >
          <SelectTrigger
            size='sm'
            aria-label={t('proxyLogs.page.filterSitePlaceholder')}
            className='w-40'
          >
            <SelectValue>
              {(selected) => {
                if (!selected || selected === 'all') {
                  return t('proxyLogs.page.filterAllSites')
                }
                const site = siteOptions.find(
                  (item) => String(item.id) === selected
                )
                return site ? site.name || `#${site.id}` : String(selected)
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('proxyLogs.page.filterAllSites')}
            </SelectItem>
            {siteOptions.map((site) => (
              <SelectItem key={site.id} value={String(site.id)}>
                {site.name || `#${site.id}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {clientOptions.length > 0 && (
        <Select
          value={client || 'all'}
          onValueChange={(value) => {
            onChange({
              client: !value || value === 'all' ? '' : value,
            })
          }}
        >
          <SelectTrigger
            size='sm'
            aria-label={t('proxyLogs.page.filterClientPlaceholder')}
            className='w-40'
          >
            <SelectValue>
              {(selected) => {
                if (!selected || selected === 'all') {
                  return t('proxyLogs.page.filterAllClients')
                }
                const option = clientOptions.find(
                  (item) => item.value === selected
                )
                return option ? option.label : String(selected)
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('proxyLogs.page.filterAllClients')}
            </SelectItem>
            {clientOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      <Input
        type='number'
        min={1}
        aria-label={t('proxyLogs.page.filterChannelPlaceholder')}
        placeholder={t('proxyLogs.page.filterChannelPlaceholder')}
        value={channelId}
        onChange={(event) => {
          onChange({ channelId: event.target.value })
        }}
        className='w-32'
      />
      {/* Native datetime controls render in the BROWSER's locale
      (not the page's). Layout is what we own: the pair stacks
      full-width on mobile so neither clips its time segment. */}
      <div className='flex items-center gap-2 max-sm:w-full max-sm:flex-col max-sm:items-stretch'>
        <Input
          type='datetime-local'
          aria-label={t('proxyLogs.page.startTime')}
          value={from}
          onChange={(event) => {
            onChange({ from: event.target.value })
          }}
          className='w-50 max-sm:w-full'
        />
        <Input
          type='datetime-local'
          aria-label={t('proxyLogs.page.endTime')}
          value={to}
          onChange={(event) => {
            onChange({ to: event.target.value })
          }}
          className='w-50 max-sm:w-full'
        />
      </div>
      <DateRangePresets
        onApply={(fromValue, toValue) => {
          onChange({ from: fromValue, to: toValue })
        }}
      />
      <div className='flex items-center gap-1.5'>
        <label className='text-muted-foreground text-xs'>
          {t('proxyLogs.page.latencyMin')}
        </label>
        <Input
          type='number'
          min={0}
          placeholder={t('proxyLogs.page.latencyUnit')}
          value={latencyMin}
          onChange={(event) => {
            const value = event.target.value
            onChange({ latencyMin: value })
          }}
          className='w-25'
        />
      </div>
      <div className='flex items-center gap-1.5'>
        <label className='text-muted-foreground text-xs'>
          {t('proxyLogs.page.latencyMax')}
        </label>
        <Input
          type='number'
          min={0}
          placeholder={t('proxyLogs.page.latencyUnit')}
          value={latencyMax}
          onChange={(event) => {
            const value = event.target.value
            onChange({ latencyMax: value })
          }}
          className='w-25'
        />
      </div>
      <div className='flex items-center gap-1.5'>
        <Button
          type='button'
          variant={slowOnly ? 'default' : 'outline'}
          size='sm'
          onClick={() => {
            onChange({ latencyMin: slowOnly ? '' : '2000', latencyMax: '' })
          }}
        >
          {t('proxyLogs.page.slowOnly')}
        </Button>
      </div>
      {hasLatencyFilter && (
        <div className='flex items-center gap-1.5 text-xs'>
          <span className='text-muted-foreground'>
            {t('proxyLogs.page.latencyExample')}
          </span>
          <LatencyBadge latencyMs={Number(latencyMin)} />
          <span className='text-muted-foreground'>~</span>
          <LatencyBadge latencyMs={Number(latencyMax)} />
        </div>
      )}
    </>
  )
}
