// metapi-go/features/observability/components — load-failure state for
// observability sections. Rendered instead of (or within) a card when a
// /api/monitor/health or /api/stats query fails, mirroring the settings
// `SettingsSectionError` pattern: the user sees an explicit error + a Retry
// button that calls `refetch()` instead of dashes and empty tables that look
// like "no data" rather than "request failed".

import { RefreshCw, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type ObservabilityErrorBannerProps = {
  /** i18n key (or literal) for the headline message. */
  messageKey?: string
  /** When true, the retry button shows a spinner + is disabled. */
  isRetrying?: boolean
  onRetry: () => void
  className?: string
}

export function ObservabilityErrorBanner({
  messageKey,
  isRetrying,
  onRetry,
  className,
}: ObservabilityErrorBannerProps) {
  const { t } = useTranslation()
  const message = messageKey
    ? t(messageKey)
    : t('observability.error.loadFailed')
  return (
    <div
      className={cn(
        'border-destructive/40 bg-destructive/5 flex min-h-32 flex-col items-center justify-center gap-3 rounded-lg border border-dashed py-8 text-center',
        className
      )}
      role='alert'
    >
      <TriangleAlert className='text-destructive/80 size-5' />
      <p className='text-destructive text-sm'>{message}</p>
      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={onRetry}
        disabled={isRetrying}
      >
        <RefreshCw className={cn('size-3.5', isRetrying && 'animate-spin')} />
        {t('observability.error.retry')}
      </Button>
    </div>
  )
}
