// metapi-go/features/observability/sections/health — site/account health +
// routing breaker/cooldown aggregation. Reads the read-only
// /api/monitor/health projection; never mutates routing state.

import { AlertTriangle, CheckCircle2, Server, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { useMonitorHealth } from '../../api'
import type { CooldownChannel, RuntimeHealthBreaker } from '../../types'

type Translate = (key: string) => string

function formatCount(value: number | undefined): string {
  return typeof value === 'number' ? value.toLocaleString() : '—'
}

function formatClock(ms?: number | null): string {
  if (!ms) return '—'
  const date = new Date(ms)
  if (Number.isNaN(date.getTime())) return String(ms)
  return date.toLocaleString()
}

function resolveToneClass(tone: 'neutral' | 'warning' | 'success'): string {
  if (tone === 'warning') return 'text-warning'
  if (tone === 'success') return 'text-success'
  return 'text-foreground'
}

function StatCell({
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
      <p
        className={`text-2xl font-semibold tabular-nums ${resolveToneClass(tone)}`}
      >
        {value}
      </p>
    </div>
  )
}

export function HealthSection() {
  const { t } = useTranslation()
  const health = useMonitorHealth()

  const data = health.data
  const runtime = data?.runtimeHealth
  const cooldown = data?.cooldown

  return (
    <div className='flex flex-col gap-4'>
      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <ShieldCheck className='size-4' />
            {t('observability.health.runtime.title')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('observability.health.runtime.description')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {renderRuntimeStats(health.isLoading, runtime, t)}
        </CardContent>
      </Card>

      <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
        <Card>
          <CardHeader>
            <CardTitle className='flex items-center gap-2 text-sm font-medium'>
              <AlertTriangle className='size-4' />
              {t('observability.health.breakers.title')}
            </CardTitle>
            <CardDescription className='text-xs'>
              {t('observability.health.breakers.description')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {renderBreakersBody(
              health.isLoading,
              runtime?.openBreakers ?? [],
              t
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className='flex items-center gap-2 text-sm font-medium'>
              <Server className='size-4' />
              {t('observability.health.cooldown.title')}
            </CardTitle>
            <CardDescription className='text-xs'>
              {t('observability.health.cooldown.description')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {renderCooldownBody(health.isLoading, cooldown, t)}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <Server className='size-4' />
            {t('observability.health.inventory.title')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('observability.health.inventory.description')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {health.isLoading ? (
            <Skeleton className='h-16 w-full rounded-md' />
          ) : (
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
              <InventoryCard
                title={t('observability.health.inventory.sites')}
                data={data?.sites}
                t={t}
              />
              <InventoryCard
                title={t('observability.health.inventory.accounts')}
                data={data?.accounts}
                t={t}
              />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function renderRuntimeStats(
  loading: boolean,
  runtime:
    | {
        sitesTracked: number
        sitesBreakerOpen: number
        modelsTracked: number
        modelsBreakerOpen: number
      }
    | undefined,
  t: Translate
) {
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
        value={formatCount(runtime?.sitesTracked)}
      />
      <StatCell
        label={t('observability.health.runtime.sitesBreakerOpen')}
        value={formatCount(runtime?.sitesBreakerOpen)}
        tone={runtime?.sitesBreakerOpen ? 'warning' : 'success'}
      />
      <StatCell
        label={t('observability.health.runtime.modelsTracked')}
        value={formatCount(runtime?.modelsTracked)}
      />
      <StatCell
        label={t('observability.health.runtime.modelsBreakerOpen')}
        value={formatCount(runtime?.modelsBreakerOpen)}
        tone={runtime?.modelsBreakerOpen ? 'warning' : 'success'}
      />
    </div>
  )
}

function renderBreakersBody(
  loading: boolean,
  breakers: RuntimeHealthBreaker[],
  t: Translate
) {
  if (loading) {
    return <Skeleton className='h-40 w-full rounded-md' />
  }
  if (breakers.length === 0) {
    return (
      <div className='flex min-h-24 items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
        <CheckCircle2 className='text-success size-4' />
        <p className='text-muted-foreground text-sm'>
          {t('observability.health.breakers.empty')}
        </p>
      </div>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('observability.health.breakers.colSite')}</TableHead>
          <TableHead>{t('observability.health.breakers.colModel')}</TableHead>
          <TableHead className='text-right'>
            {t('observability.health.breakers.colLevel')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.breakers.colUntil')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {breakers.map((breaker) => (
          <TableRow key={`${breaker.siteId}:${breaker.model}`}>
            <TableCell className='tabular-nums'>{breaker.siteId}</TableCell>
            <TableCell className='truncate'>{breaker.model || '—'}</TableCell>
            <TableCell className='text-right tabular-nums'>
              {breaker.breakerLevel}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {formatClock(breaker.breakerUntilMs)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function renderCooldownBody(
  loading: boolean,
  cooldown:
    | {
        channelsCooling: number
        channelsWithFailures: number
        channelsRecentlyFailed: number
        cooling: CooldownChannel[]
      }
    | undefined,
  t: Translate
) {
  if (loading) {
    return <Skeleton className='h-40 w-full rounded-md' />
  }
  return (
    <>
      <div className='mb-3 grid grid-cols-3 gap-2'>
        <StatCell
          label={t('observability.health.cooldown.channelsCooling')}
          value={formatCount(cooldown?.channelsCooling)}
          tone={cooldown?.channelsCooling ? 'warning' : 'success'}
        />
        <StatCell
          label={t('observability.health.cooldown.channelsWithFailures')}
          value={formatCount(cooldown?.channelsWithFailures)}
        />
        <StatCell
          label={t('observability.health.cooldown.channelsRecentlyFailed')}
          value={formatCount(cooldown?.channelsRecentlyFailed)}
        />
      </div>
      {renderCoolingTable(cooldown?.cooling ?? [], t)}
    </>
  )
}

function renderCoolingTable(channels: CooldownChannel[], t: Translate) {
  if (channels.length === 0) {
    return (
      <div className='flex min-h-16 items-center justify-center gap-2 rounded-lg border border-dashed py-6 text-center'>
        <p className='text-muted-foreground text-sm'>
          {t('observability.health.cooldown.empty')}
        </p>
      </div>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('observability.health.cooldown.colSite')}</TableHead>
          <TableHead className='text-right'>
            {t('observability.health.cooldown.colChannel')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.cooldown.colFailCount')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.cooldown.colUntil')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {channels.map((channel) => (
          <TableRow key={channel.channelId}>
            <TableCell className='truncate'>
              {channel.siteName || channel.siteId}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {channel.channelId}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {channel.failCount}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {channel.cooldownUntil || '—'}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function InventoryCard({
  title,
  data,
  t,
}: {
  title: string
  data?: { total: number; active: number; disabled: number; other: number }
  t: Translate
}) {
  return (
    <div className='bg-card rounded-lg border p-3'>
      <p className='text-muted-foreground mb-2 text-xs font-medium'>{title}</p>
      <div className='grid grid-cols-4 gap-2 text-center'>
        <InventoryMetric
          label={t('observability.health.inventory.total')}
          value={formatCount(data?.total)}
          className='text-foreground'
        />
        <InventoryMetric
          label={t('observability.health.inventory.active')}
          value={formatCount(data?.active)}
          className='text-success'
        />
        <InventoryMetric
          label={t('observability.health.inventory.disabled')}
          value={formatCount(data?.disabled)}
          className='text-muted-foreground'
        />
        <InventoryMetric
          label={t('observability.health.inventory.other')}
          value={formatCount(data?.other)}
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
      <p className={`text-lg font-semibold tabular-nums ${className}`}>
        {value}
      </p>
    </div>
  )
}
