import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Notice } from '@/components/ui/notice'

export function WriteError({ error }: { error: Error | null }) {
  const { t } = useTranslation()
  return error ? (
    <Notice tone='destructive' role='alert'>
      {t('userAdmin.writeFailed')}
    </Notice>
  ) : null
}

export function Loading() {
  const { t } = useTranslation()
  return (
    <p className='text-muted-foreground py-4 text-sm' role='status'>
      {t('userAdmin.loading')}
    </p>
  )
}

export function LoadError({
  error,
  retry,
  pending,
}: {
  error: Error | null
  retry: () => void
  pending?: boolean
}) {
  return (
    <QueryErrorBanner
      error={error}
      messageKey='userAdmin.loadFailed'
      onRetry={retry}
      isRetrying={pending}
    />
  )
}
