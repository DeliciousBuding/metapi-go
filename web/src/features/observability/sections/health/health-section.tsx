import { Server, ShieldCheck, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useCooldownClock } from '@/hooks/use-cooldown-clock'
import { toBcp47 } from '@/i18n/languages'

import { useMonitorHealth } from '../../api'
import { ObservabilityErrorBanner } from '../../components/observability-error-banner'
import { HealthBreakerTable } from './health-breaker-table'
import { HealthCoolingChannels } from './health-cooling-channels'
import { RuntimeStats, InventoryCard } from './health-metrics'

export function HealthSection() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const health = useMonitorHealth()

  const data = health.data
  const runtime = data?.runtimeHealth
  const cooldown = data?.cooldown
  const deadlines = useMemo(
    () => [
      ...(runtime?.openBreakers ?? []).map((b) => b.breakerUntilMs),
      ...(cooldown?.cooling ?? []).map((c) => c.cooldownUntil),
    ],
    [runtime?.openBreakers, cooldown?.cooling]
  )
  const now = useCooldownClock(deadlines)

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
          <CardTitle className='flex items-center gap-2'>
            <ShieldCheck className='size-4' />
            {t('observability.health.runtime.title')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('observability.health.runtime.description')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <RuntimeStats loading={health.isLoading} runtime={runtime} />
        </CardContent>
      </Card>

      <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
        <Card>
          <CardHeader>
            <CardTitle className='flex items-center gap-2'>
              <TriangleAlert className='size-4' />
              {t('observability.health.breakers.title')}
            </CardTitle>
            <CardDescription className='text-xs'>
              {t('observability.health.breakers.description')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <HealthBreakerTable
              loading={health.isLoading}
              breakers={runtime?.openBreakers ?? []}
              locale={locale}
              now={now}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className='flex items-center gap-2'>
              <Server className='size-4' />
              {t('observability.health.cooldown.title')}
            </CardTitle>
            <CardDescription className='text-xs'>
              {t('observability.health.cooldown.description')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <HealthCoolingChannels
              loading={health.isLoading}
              cooldown={cooldown}
              locale={locale}
              now={now}
            />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
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
              />
              <InventoryCard
                title={t('observability.health.inventory.accounts')}
                data={data?.accounts}
              />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
