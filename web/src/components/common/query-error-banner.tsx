// Shared load-error banner with an optional Retry button. Replaces the
// per-page `{error && <div className='border-destructive/40 …'>}` pattern
// (banner-only) and the sites/routes Pattern A (banner + retry button).
// Pass `onRetry` to render the Retry button; omit it for a banner-only
// inline warning above the table.

import { RefreshCw, TriangleAlert } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

type QueryErrorBannerProps = {
  /** The error object; renders nothing when null. */
  error: Error | null
  /** i18n key whose template interpolates `{{message}}` (e.g. `accounts.page.loadError`). */
  messageKey: string
  /** i18n key for the Retry button label; defaults to `common.retry`. */
  retryKey?: string
  /** When provided, a Retry button renders and re-fetches the query. */
  onRetry?: () => void
  /** True while the retry request is in flight (disables the button + shows a spinner). */
  isRetrying?: boolean
  /** Extra children rendered after the banner (e.g. a secondary action). */
  children?: ReactNode
  className?: string
}

export function QueryErrorBanner({
  error,
  messageKey,
  retryKey = 'common.retry',
  onRetry,
  isRetrying = false,
  children,
  className,
}: QueryErrorBannerProps) {
  const { t } = useTranslation()
  if (!error) return null

  return (
    <div className={`flex flex-col gap-3 ${className ?? ''}`}>
      <Notice tone='destructive' role='alert'>
        <TriangleAlert className='mt-0.5 size-4 shrink-0' />
        <span>{t(messageKey, { message: error.message })}</span>
      </Notice>
      {(onRetry || children) && (
        <div className='flex items-center gap-2'>
          {onRetry && (
            <Button
              variant='secondary'
              size='sm'
              onClick={onRetry}
              disabled={isRetrying}
            >
              {isRetrying ? <Spinner /> : <RefreshCw className='size-3.5' />}
              {t(retryKey)}
            </Button>
          )}
          {children}
        </div>
      )}
    </div>
  )
}
