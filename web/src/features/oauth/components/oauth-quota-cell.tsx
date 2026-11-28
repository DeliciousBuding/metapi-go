import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import {
  classifyOAuthQuotaSeverity,
  resolveOAuthQuotaUsageRatio,
} from '../lib/oauth-quota'
import type { OAuthClient } from '../types'

type OAuthQuotaWindow = NonNullable<OAuthClient['quota']>['windows']['fiveHour']

/** Compact `used/limit` pair for one quota window ("12/50"). */
function formatQuotaWindowUsage(window: OAuthQuotaWindow): string {
  if (window.used != null && window.limit != null) {
    return `${window.used}/${window.limit}`
  }
  if (window.used != null) return `${window.used}`
  if (window.limit != null) return `—/${window.limit}`
  return '—'
}

const QUOTA_BAR_TONE: Record<'ok' | 'high' | 'exhausted', string> = {
  ok: 'bg-success',
  high: 'bg-warning',
  exhausted: 'bg-destructive',
}

/**
 * Renders the provider quota windows (5-hour / 7-day) as compact
 * `used/limit` lines with an honest proportional progress bar when both
 * `used` and positive `limit` are reported. Only supported windows that carry
 * at least one number render; anything else (unsupported provider, sync error,
 * empty payload) degrades to an em dash instead of a fabricated "0/0".
 */
export function QuotaCell({ client }: { client: OAuthClient }) {
  const { t } = useTranslation()
  const quota = client.quota
  if (!quota || quota.status !== 'supported' || !quota.windows) {
    return <span className='text-muted-foreground text-sm'>—</span>
  }
  const supportedWindows = [
    { labelKey: 'oauth.columns.quotaFiveHour', window: quota.windows.fiveHour },
    { labelKey: 'oauth.columns.quotaSevenDay', window: quota.windows.sevenDay },
  ].filter(
    ({ window }) =>
      window.supported && (window.used != null || window.limit != null)
  )
  if (supportedWindows.length === 0) {
    return <span className='text-muted-foreground text-sm'>—</span>
  }
  return (
    <div className='flex min-w-22 flex-col gap-1 text-xs tabular-nums'>
      {supportedWindows.map(({ labelKey, window }) => {
        const ratio = resolveOAuthQuotaUsageRatio(window)
        const barTone =
          ratio !== null
            ? QUOTA_BAR_TONE[classifyOAuthQuotaSeverity(ratio)]
            : ''
        return (
          <div key={labelKey} className='space-y-0.5'>
            <div className='flex items-center justify-between gap-2 whitespace-nowrap'>
              <span className='text-muted-foreground'>{t(labelKey)}</span>
              <span className='font-medium'>
                {formatQuotaWindowUsage(window)}
              </span>
            </div>
            {ratio !== null && (
              <div
                className='bg-muted h-1 w-full overflow-hidden rounded-full'
                aria-hidden='true'
              >
                <div
                  className={cn('h-full rounded-full', barTone)}
                  style={{ width: `${Math.round(ratio * 100)}%` }}
                />
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
