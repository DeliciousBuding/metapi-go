import { useTranslation } from 'react-i18next'

import { useRelativeFreshnessNow } from '@/hooks/use-quiet-refresh'
import { cn } from '@/lib/utils'

export function QuietRefreshStatus({
  updatedAt,
  hasError = false,
  className,
}: {
  updatedAt: number
  hasError?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const now = useRelativeFreshnessNow(updatedAt, hasError)

  if (now === null || hasError || !updatedAt || updatedAt <= 0) {
    return null
  }

  const elapsedSeconds = Math.max(0, Math.floor((now - updatedAt) / 1000))
  let label: string
  if (elapsedSeconds < 10) {
    label = t('common.freshness.justNow')
  } else if (elapsedSeconds < 60) {
    label = t('common.freshness.secondsAgo', { count: elapsedSeconds })
  } else if (elapsedSeconds < 3600) {
    label = t('common.freshness.minutesAgo', {
      count: Math.floor(elapsedSeconds / 60),
    })
  } else {
    label = t('common.freshness.hoursAgo', {
      count: Math.floor(elapsedSeconds / 3600),
    })
  }

  return (
    <span
      data-testid='quiet-refresh-status'
      className={cn(
        'text-muted-foreground text-xs whitespace-nowrap tabular-nums',
        className
      )}
    >
      {label}
    </span>
  )
}
