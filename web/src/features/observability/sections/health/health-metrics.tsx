import { useTranslation } from 'react-i18next'

import { KpiValue } from '@/components/ui/kpi-value'
import { Skeleton } from '@/components/ui/skeleton'
import { formatInt } from '@/lib/format'

import type { MonitorHealthResponse } from '../../types'
function resolveToneClass(tone: 'neutral' | 'warning' | 'success'): string {
  if (tone === 'warning') return 'text-warning-soft-fg'
  if (tone === 'success') return 'text-success-soft-fg'
  return 'text-foreground'
}

export function StatCell({
  label,
  value,
  tone = 'neutral',
}: {
  label: string
  value: string
  tone?: 'neutral' | 'warning' | 'success'
}) {
  return (
    <div className='bg-card rounded-lg border p-3'>
      <p className='text-muted-foreground text-xs'>{label}</p>
      <KpiValue size='lg' className={resolveToneClass(tone)}>
        {value}
      </KpiValue>
    </div>
  )
}

export function RuntimeStats({
  loading,
  runtime,
}: {
  loading: boolean
  runtime?: MonitorHealthResponse['runtimeHealth']
}) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className='h-20 rounded-lg' />
        ))}
      </div>
    )
  }
  return (
    <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
      <StatCell
        label={t('observability.health.runtime.sitesTracked')}
        value={formatInt(runtime?.sitesTracked)}
      />
      <StatCell
        label={t('observability.health.runtime.sitesBreakerOpen')}
        value={formatInt(runtime?.sitesBreakerOpen)}
        tone={runtime?.sitesBreakerOpen ? 'warning' : 'success'}
      />
      <StatCell
        label={t('observability.health.runtime.modelsTracked')}
        value={formatInt(runtime?.modelsTracked)}
      />
      <StatCell
        label={t('observability.health.runtime.modelsBreakerOpen')}
        value={formatInt(runtime?.modelsBreakerOpen)}
        tone={runtime?.modelsBreakerOpen ? 'warning' : 'success'}
      />
    </div>
  )
}

export function InventoryCard({
  title,
  data,
}: {
  title: string
  data?: MonitorHealthResponse['sites']
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-card rounded-lg border p-3'>
      <p className='text-muted-foreground mb-2 text-xs font-medium'>{title}</p>
      <div className='grid grid-cols-4 gap-2 text-center'>
        <InventoryMetric
          label={t('observability.health.inventory.total')}
          value={formatInt(data?.total)}
          className='text-foreground'
        />
        <InventoryMetric
          label={t('observability.health.inventory.active')}
          value={formatInt(data?.active)}
          className='text-success'
        />
        <InventoryMetric
          label={t('observability.health.inventory.disabled')}
          value={formatInt(data?.disabled)}
          className='text-muted-foreground'
        />
        <InventoryMetric
          label={t('observability.health.inventory.other')}
          value={formatInt(data?.other)}
          className='text-foreground'
        />
      </div>
    </div>
  )
}

function InventoryMetric({
  label,
  value,
  className,
}: {
  label: string
  value: string
  className: string
}) {
  return (
    <div>
      <p className='text-muted-foreground text-xs'>{label}</p>
      <KpiValue size='sm' className={className}>
        {value}
      </KpiValue>
    </div>
  )
}
