// metapi-go/features/observability/sections/health — site/account health +
// routing breaker/cooldown aggregation. Reads the read-only
// /api/monitor/health projection; never mutates routing state.

import { Link } from '@tanstack/react-router'
import { CheckCircle2, Server, ShieldCheck, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'
import { KpiValue } from '@/components/ui/kpi-value'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime, formatInt } from '@/lib/format'

import { useMonitorHealth } from '../../api'
import { ObservabilityErrorBanner } from '../../components/observability-error-banner'
import type { CooldownChannel, RuntimeHealthBreaker } from '../../types'

type Translate = (key: string) => string

function resolveToneClass(tone: 'neutral' | 'warning' | 'success'): string {
  if (tone === 'warning') return 'text-warning-soft-fg'
  if (tone === 'success') return 'text-success-soft-fg'
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
      <KpiValue size='lg' className={resolveToneClass(tone)}>
        {value}
      </KpiValue>
    </div>
  )
}

export function HealthSection() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const health = useMonitorHealth()

  const data = health.data
  const runtime = data?.runtimeHealth
  const cooldown = data?.cooldown

  // Surface a load failure explicitly instead of rendering dashes / empty
  // tables that read as "no open breakers" rather than "request failed".
  // A retry re-runs the monitor-health query (auto-refresh keeps ticking).
  if (health.isError && !health.isLoading) {
    return (
      <ObservabilityErrorBanner
        messageKey='observability.health.loadFailed'
        isRetrying={health.isFetching && !health.isLoading}
        onRetry={() => void health.refetch()}
      />
    )
  }

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
              <TriangleAlert className='size-4' />
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
              t,
              locale
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
            {renderCooldownBody(health.isLoading, cooldown, t, locale)}
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

function renderBreakersBody(
  loading: boolean,
  breakers: RuntimeHealthBreaker[],
  t: Translate,
  locale: string
) {
  if (loading) {
    return <Skeleton className='h-40 w-full rounded-md' />
  }
  if (breakers.length === 0) {
    return (
      <Empty className='min-h-24 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon' className='bg-success/10 text-success'>
            <CheckCircle2 />
          </EmptyMedia>
          <EmptyDescription>
            {t('observability.health.breakers.empty')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
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
            <TableCell className='tabular-nums'>
              {/* The breaker projection carries no site name, so the cell
                  keeps the raw id but deep-links into the sites page edit
                  dialog (one-shot `edit` param). */}
              <Link
                to='/sites'
                search={{ edit: breaker.siteId }}
                className='text-primary hover:underline'
              >
                {breaker.siteId}
              </Link>
            </TableCell>
            <TableCell className='truncate'>{breaker.model || '—'}</TableCell>
            <TableCell className='text-right tabular-nums'>
              {breaker.breakerLevel}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {formatDateTime(breaker.breakerUntilMs, locale)}
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
  t: Translate,
  locale: string
) {
  if (loading) {
    return <Skeleton className='h-40 w-full rounded-md' />
  }
  return (
    <>
      <div className='mb-3 grid grid-cols-3 gap-2'>
        <StatCell
          label={t('observability.health.cooldown.channelsCooling')}
          value={formatInt(cooldown?.channelsCooling)}
          tone={cooldown?.channelsCooling ? 'warning' : 'success'}
        />
        <StatCell
          label={t('observability.health.cooldown.channelsWithFailures')}
          value={formatInt(cooldown?.channelsWithFailures)}
        />
        <StatCell
          label={t('observability.health.cooldown.channelsRecentlyFailed')}
          value={formatInt(cooldown?.channelsRecentlyFailed)}
        />
      </div>
      {renderCoolingTable(cooldown?.cooling ?? [], t, locale)}
    </>
  )
}

function renderCoolingTable(
  channels: CooldownChannel[],
  t: Translate,
  locale: string
) {
  if (channels.length === 0) {
    return (
      <Empty className='min-h-16 border'>
        <EmptyHeader>
          <EmptyDescription>
            {t('observability.health.cooldown.empty')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
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
              <Link
                to='/channels'
                search={{ channelId: channel.channelId }}
                className='text-primary hover:underline'
              >
                {channel.channelId}
              </Link>
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {channel.failCount}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {formatDateTime(channel.cooldownUntil, locale)}
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
