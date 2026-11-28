import { RefreshCw } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import { Button } from '@/components/ui/button'
import { useDelayedSpin } from '@/hooks/use-quiet-refresh'
import { toBcp47 } from '@/i18n/languages'
import { formatInt } from '@/lib/format'

import type { useOAuthConnections } from '../api'
import { resolveOAuthQuotaAvailability } from '../lib/oauth-quota'
import type { OAuthClient } from '../types'

export function OAuthPageHeader({
  connectionsQuery,
  connections,
  connectionsTotal,
}: {
  connectionsQuery: ReturnType<typeof useOAuthConnections>
  connections: OAuthClient[]
  connectionsTotal: number
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const showRefreshSpin = useDelayedSpin(connectionsQuery.isFetching)
  const pageStats = useMemo(() => {
    let healthy = 0
    let abnormal = 0
    let quotaTracked = 0
    for (const item of connections) {
      if (item.status === 'abnormal') abnormal += 1
      else healthy += 1
      if (resolveOAuthQuotaAvailability(item.quota) === 'reported') {
        quotaTracked += 1
      }
    }
    return { healthy, abnormal, quotaTracked }
  }, [connections])
  return (
    <>
      <PageHeader
        title={t('oauth.page.title')}
        description={t('oauth.page.description')}
        actions={
          <>
            <QuietRefreshStatus
              updatedAt={connectionsQuery.dataUpdatedAt ?? 0}
              hasError={Boolean(connectionsQuery.error)}
              className='hidden sm:inline'
            />
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={connectionsQuery.isFetching}
              onClick={() => void connectionsQuery.refetch?.()}
            >
              <RefreshCw
                aria-hidden='true'
                className={
                  showRefreshSpin ? 'size-3.5 animate-spin' : 'size-3.5'
                }
              />
              {t('oauth.toolbar.refresh')}
            </Button>
          </>
        }
      />

      {!connectionsQuery.error && connectionsTotal > 0 && (
        <div
          className='bg-muted/20 flex flex-wrap items-center gap-x-5 gap-y-1 rounded-lg border px-3 py-2'
          data-testid='oauth-summary-strip'
        >
          <span className='inline-flex items-baseline gap-2 whitespace-nowrap'>
            <span className='text-muted-foreground text-xs'>
              {t('oauth.summary.total')}
            </span>
            <span className='text-foreground text-sm font-semibold tabular-nums'>
              {formatInt(connectionsTotal, locale)}
            </span>
          </span>
          <span className='inline-flex items-baseline gap-2 whitespace-nowrap'>
            <span className='text-muted-foreground text-xs'>
              {t('oauth.summary.healthy')}
            </span>
            <span className='text-success text-sm font-semibold tabular-nums'>
              {formatInt(pageStats.healthy, locale)}
            </span>
          </span>
          {pageStats.abnormal > 0 && (
            <span className='inline-flex items-baseline gap-2 whitespace-nowrap'>
              <span className='text-muted-foreground text-xs'>
                {t('oauth.summary.abnormal')}
              </span>
              <span className='text-destructive text-sm font-semibold tabular-nums'>
                {formatInt(pageStats.abnormal, locale)}
              </span>
            </span>
          )}
          <span className='inline-flex items-baseline gap-2 whitespace-nowrap'>
            <span className='text-muted-foreground text-xs'>
              {t('oauth.summary.quotaTracked')}
            </span>
            <span className='text-foreground text-sm font-semibold tabular-nums'>
              {formatInt(pageStats.quotaTracked, locale)}
            </span>
          </span>
        </div>
      )}
    </>
  )
}
