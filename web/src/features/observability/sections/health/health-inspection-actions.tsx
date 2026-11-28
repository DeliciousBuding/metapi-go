import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'

export function FailedAttemptsLink({
  filters,
  model,
}: {
  filters: { siteId: number } | { channelId: number }
  model?: string
}) {
  const { t } = useTranslation()
  return (
    <Link
      to='/proxy-logs'
      search={{
        view: 'attempts',
        ...filters,
        status: 'failed',
        ...(model ? { q: model } : {}),
      }}
      className={buttonVariants({ size: 'xs', variant: 'outline' })}
    >
      {t('observability.health.breakers.inspectLogs')}
    </Link>
  )
}

export function BreakerActions({
  siteId,
  model,
}: {
  siteId: number
  model: string
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap justify-end gap-1'>
      <FailedAttemptsLink filters={{ siteId }} model={model} />
      {model && (
        <Link
          to='/token-routes'
          search={{ workspaceModel: model }}
          className={buttonVariants({ size: 'xs', variant: 'ghost' })}
        >
          {t('observability.health.breakers.inspectRoute')}
        </Link>
      )}
    </div>
  )
}
